package ewf

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type State map[string]any

type WorkflowStatus string

const (
	StatusPending   WorkflowStatus = "pending"
	StatusRunning   WorkflowStatus = "running"
	StatusCompleted WorkflowStatus = "completed"
	StatusFailed    WorkflowStatus = "failed"
)

type StepFn func(ctx context.Context, state State) error

type Step struct {
	Name        string
	RetryPolicy *RetryPolicy
}

type RetryPolicy struct {
	MaxAttempts uint          `json:"max_attempts"`
	Delay       time.Duration `json:"delay"`
}
type BeforeWorkflowHook func(ctx context.Context, w *Workflow)
type AfterWorkflowHook func(ctx context.Context, w *Workflow, err error)
type BeforeStepHook func(ctx context.Context, w *Workflow, step *Step)
type AfterStepHook func(ctx context.Context, w *Workflow, step *Step, err error)

type Store interface {
	Setup() error // could be a no-op, no problem.
	SaveWorkflow(ctx context.Context, workflow *Workflow) error
	LoadWorkflowByName(ctx context.Context, name string) (*Workflow, error)
	LoadWorkflowByUUID(ctx context.Context, uuid string) (*Workflow, error)
	ListWorkflowUUIDsByStatus(ctx context.Context, status WorkflowStatus) ([]string, error)
	Close() error // could be a no-op, no problem.
}
type Workflow struct {
	UUID        string         `json:"uuid"`
	Name        string         `json:"name"`
	Status      WorkflowStatus `json:"status"`
	State       State          `json:"state"`
	CurrentStep int            `json:"current_step"`
	CreatedAt   time.Time      `json:"created_at"`

	// non persisted fields
	Steps               []Step               `json:"-"`
	store               Store                `json:"-"`
	beforeWorkflowHooks []BeforeWorkflowHook `json:"-"`
	afterWorkflowHooks  []AfterWorkflowHook  `json:"-"`
	beforeStepHooks     []BeforeStepHook     `json:"-"`
	afterStepHooks      []AfterStepHook      `json:"-"`
}
type WorkflowTemplate struct {
	Steps               []Step
	BeforeWorkflowHooks []BeforeWorkflowHook
	AfterWorkflowHooks  []AfterWorkflowHook
	BeforeStepHooks     []BeforeStepHook
	AfterStepHooks      []AfterStepHook
}

type WorkflowOpt func(w *Workflow)

func WithStore(store Store) WorkflowOpt {
	return func(w *Workflow) {
		w.store = store
	}
}

func (w *Workflow) SetStore(store Store) {
	w.store = store
}

func NewWorkflow(name string, opts ...WorkflowOpt) *Workflow {
	w := &Workflow{
		UUID:      uuid.New().String(),
		Name:      name,
		Status:    StatusPending,
		Steps:     []Step{},
		State:     make(State),
		CreatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Workflow) run(ctx context.Context, activities map[string]StepFn) (err error) {
	// register all after workflow hooks
	defer func() {
		for _, hook := range w.afterWorkflowHooks {
			hook(ctx, w, err)
		}
	}()

	// execute all before workflow hooks
	for _, hook := range w.beforeWorkflowHooks {
		hook(ctx, w)
	}

	if w.Status == StatusCompleted {
		return nil // Already completed
	}
	w.Status = StatusRunning
	// move to the current step
	for i := w.CurrentStep; i < len(w.Steps); i++ {
		step := w.Steps[i]

		for _, stepHook := range w.beforeStepHooks {
			stepHook(ctx, w, &step)
		}

		// execute the step with its retry policy (or default if it's nil)
		// break if you reached the max attempts or the step was successful
		var attempts uint = 1
		var stepErr error
		for {
							activity, ok := activities[step.Name]
				if !ok {
										return fmt.Errorf("activity '%s' not registered", step.Name)
				}
				stepErr = activity(ctx, w.State)
			if stepErr == nil {
				break
			}
			if step.RetryPolicy == nil {
				break
			}
			if attempts >= step.RetryPolicy.MaxAttempts {
				break
			}
			// Wait for the retry delay, while respecting context cancellation.
			timer := time.NewTimer(step.RetryPolicy.Delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
				// Continue to the next attempt.
			}
			attempts++

		}
		// --- After Step Hook ---
		for _, hook := range w.afterStepHooks {
			hook(ctx, w, &step, stepErr)
		}

		if stepErr != nil {
			w.Status = StatusFailed
			if w.store != nil {
				_ = w.store.SaveWorkflow(ctx, w) // best effort
			}
			return fmt.Errorf("workflow failed: step %s failed: %w", step.Name, stepErr)
		}
		w.CurrentStep = i + 1
		if w.store != nil {
			if err := w.store.SaveWorkflow(ctx, w); err != nil {
				return fmt.Errorf("failed to save workflow state after step %d: %v", w.CurrentStep-1, err)
			}
		}
	}

	w.Status = StatusCompleted
	if w.store != nil {
		if err := w.store.SaveWorkflow(ctx, w); err != nil {
			return fmt.Errorf("failed to save final workflow state: %w", err)
		}
	}

	return nil
}
