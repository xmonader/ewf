package ewf

import (
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"time"
)

// ErrFailWorkflowNow is a special error that can be returned by a step to indicate that the workflow should be failed immediately.
var ErrFailWorkflowNow = errors.New("fail workflow now")

// State represents the workflow state as a generic key-value map.
type State map[string]any

// WorkflowStatus represents the status of a workflow.
type WorkflowStatus string

const (
	// StatusPending indicates the workflow is pending and has not started.
	StatusPending   WorkflowStatus = "pending"
	// StatusRunning indicates the workflow is currently running.
	StatusRunning   WorkflowStatus = "running"
	// StatusCompleted indicates the workflow has completed successfully.
	StatusCompleted WorkflowStatus = "completed"
	// StatusFailed indicates the workflow has failed.
	StatusFailed    WorkflowStatus = "failed"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

// Context keys used by the workflow engine
const (
	// StepNameContextKey is used to store the current step name in the context
	StepNameContextKey contextKey = "stepName"
)


// StepFn defines the function signature for a workflow step.
type StepFn func(ctx context.Context, state State) error

// Step represents a single step in a workflow.
type Step struct {
	Name        string
	RetryPolicy *RetryPolicy
}

// RetryPolicy defines the retry behavior for a step.
type RetryPolicy struct {
	MaxAttempts uint          `json:"max_attempts"`
	Delay       time.Duration `json:"delay"`
}
// BeforeWorkflowHook is a function run before a workflow starts.
type BeforeWorkflowHook func(ctx context.Context, w *Workflow)
// AfterWorkflowHook is a function run after a workflow finishes.
type AfterWorkflowHook func(ctx context.Context, w *Workflow, err error)
// BeforeStepHook is a function run before a step starts.
type BeforeStepHook func(ctx context.Context, w *Workflow, step *Step)
// AfterStepHook is a function run after a step finishes.
type AfterStepHook func(ctx context.Context, w *Workflow, step *Step, err error)

// Store defines the interface for workflow persistence.
type Store interface {
	Setup() error // could be a no-op, no problem.
	SaveWorkflow(ctx context.Context, workflow *Workflow) error
	LoadWorkflowByName(ctx context.Context, name string) (*Workflow, error)
	LoadWorkflowByUUID(ctx context.Context, uuid string) (*Workflow, error)
	ListWorkflowUUIDsByStatus(ctx context.Context, status WorkflowStatus) ([]string, error)
	Close() error // could be a no-op, no problem.
}
// Workflow represents a workflow instance.
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
// WorkflowTemplate defines the structure and hooks for a workflow definition.
type WorkflowTemplate struct {
	Steps               []Step
	BeforeWorkflowHooks []BeforeWorkflowHook
	AfterWorkflowHooks  []AfterWorkflowHook
	BeforeStepHooks     []BeforeStepHook
	AfterStepHooks      []AfterStepHook
}

// WorkflowOpt is a functional option for configuring a workflow.
type WorkflowOpt func(w *Workflow)

// WithStore sets the store for a workflow.
func WithStore(store Store) WorkflowOpt {
	return func(w *Workflow) {
		w.store = store
	}
}

// SetStore sets the store for the workflow.
func (w *Workflow) SetStore(store Store) {
	w.store = store
}

// NewWorkflow creates a new workflow instance with the given name and options.
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
			// Inject step name into context for idempotency key helpers
			ctxWithStep := context.WithValue(ctx, StepNameContextKey, step.Name)
			stepErr = activity(ctxWithStep, w.State)
			// Check for fail-fast error
			if stepErr == ErrFailWorkflowNow {
				break // break out to mark workflow as failed immediately
			}
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
