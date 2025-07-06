package ewf

import (
	"context"
	"fmt"
	"time"
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
	Fn          StepFn
	RetryPolicy *RetryPolicy
}

type RetryPolicy struct {
	MaxAttempts int           `json:"max_attempts"`
	Delay       time.Duration `json:"delay"`
}
type BeforeWorkflowHook func(ctx context.Context, w *Workflow)
type AfterWorkflowHook func(ctx context.Context, w *Workflow, err error)
type BeforeStepHook func(ctx context.Context, w *Workflow, step *Step)
type AfterStepHook func(ctx context.Context, w *Workflow, step *Step, err error)

type Store interface {
	SaveWorkflow(ctx context.Context, workflow *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	ListIDsByStatus(ctx context.Context, status WorkflowStatus) ([]string, error)
}
type Workflow struct {
	ID          string         `json:"id"`
	Status      WorkflowStatus `json:"status"`
	Steps       []Step         `json:"steps"`
	State       State          `json:"state"`
	CurrentStep int            `json:"current_step"`

	// non persisted fields
	store               Store                `json:"-"`
	beforeWorkflowHooks []BeforeWorkflowHook `json:"-"`
	afterWorkflowHooks  []AfterWorkflowHook  `json:"-"`
	beforeStepHooks     []BeforeStepHook     `json:"-"`
	afterStepHooks      []AfterStepHook      `json:"-"`
}
type WorkflowOpt func(w *Workflow)

func WithStore(store Store) WorkflowOpt {
	return func(w *Workflow) {
		w.store = store
	}
}
func WithSteps(steps ...Step) WorkflowOpt {
	return func(w *Workflow) {
		w.Steps = append(w.Steps, steps...)
	}
}

func NewWorkflow(id string, opts ...WorkflowOpt) *Workflow {
	w := &Workflow{
		ID:     id,
		Status: StatusPending,
		Steps:  []Step{},
		State:  make(State),
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func (w *Workflow) Run(ctx context.Context) (err error) {
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
		attempts := 1
		var stepErr error
		for {
			stepErr = step.Fn(ctx, w.State)
			if stepErr == nil {
				break
			}
			if step.RetryPolicy == nil {
				break
			}
			if attempts > step.RetryPolicy.MaxAttempts {
				break
			}
			// sleep for the delay
			time.Sleep(step.RetryPolicy.Delay * time.Second)
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
			if err = w.store.SaveWorkflow(ctx, w); err != nil {
				err = fmt.Errorf("failed to save state after step '%s': %w", step.Name, err)
				return
			}
		}
	}
	w.Status = StatusCompleted
	if w.store != nil {
		if err := w.store.SaveWorkflow(ctx, w); err != nil {
			err = fmt.Errorf("failed to save final state: %w", err)
			return err
		}
	}
	return nil
}
