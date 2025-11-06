package ewf

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ErrFailWorkflowNow is a special error that can be returned by a step to indicate that the workflow should be failed immediately.
var ErrFailWorkflowNow = errors.New("fail workflow now")

// State represents the workflow state as a generic key-value map.
type State map[string]any

// WorkflowStatus represents the status of a workflow.
type WorkflowStatus string

const (
	// StatusPending indicates the workflow is pending and has not started.
	StatusPending WorkflowStatus = "pending"
	// StatusRunning indicates the workflow is currently running.
	StatusRunning WorkflowStatus = "running"
	// StatusCompleted indicates the workflow has completed successfully.
	StatusCompleted WorkflowStatus = "completed"
	// StatusFailed indicates the workflow has failed.
	StatusFailed WorkflowStatus = "failed"
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
	Timeout     time.Duration // Maximum execution time for the step (including retries)
}

// RetryPolicy defines the retry behavior for a step.
type RetryPolicy struct {
	MaxAttempts uint
	BackOff     BackOff
}

// MarshalJSON implements custom JSON marshaling for RetryPolicy
func (rp *RetryPolicy) MarshalJSON() ([]byte, error) {
	if rp == nil {
		return json.Marshal(nil)
	}

	var backoffData []byte
	var err error
	if rp.BackOff != nil {
		backoffData, err = MarshalBackOff(rp.BackOff)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal BackOff: %w", err)
		}
	}

	// Create a temporary struct to avoid infinite recursion
	type RetryPolicyTemp struct {
		MaxAttempts uint            `json:"max_attempts"`
		BackOff     json.RawMessage `json:"backoff,omitempty"`
	}

	tmp := RetryPolicyTemp{
		MaxAttempts: rp.MaxAttempts,
		BackOff:     backoffData,
	}

	return json.Marshal(tmp)
}

// UnmarshalJSON implements custom JSON unmarshaling for RetryPolicy
func (rp *RetryPolicy) UnmarshalJSON(data []byte) error {
	if string(data) == "null" || len(data) == 0 {
		*rp = RetryPolicy{}
		return nil
	}

	// Create a temporary struct to avoid infinite recursion
	type RetryPolicyTemp struct {
		MaxAttempts uint            `json:"max_attempts"`
		BackOff     json.RawMessage `json:"backoff,omitempty"`
	}

	var tmp RetryPolicyTemp
	if err := json.Unmarshal(data, &tmp); err != nil {
		return err
	}

	rp.MaxAttempts = tmp.MaxAttempts

	if len(tmp.BackOff) > 0 {
		backoff, err := UnmarshalBackOff(tmp.BackOff)
		if err != nil {
			return fmt.Errorf("failed to unmarshal BackOff: %w", err)
		}
		rp.BackOff = backoff
	}

	return nil
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
	SaveWorkflowTemplate(ctx context.Context, name string, tmpl *WorkflowTemplate) error
	LoadWorkflowTemplate(ctx context.Context, name string) (*WorkflowTemplate, error)
	LoadAllWorkflowTemplates(ctx context.Context) (map[string]*WorkflowTemplate, error)
	SaveQueueMetadata(ctx context.Context, meta *QueueMetadata) error
	DeleteQueueMetadata(ctx context.Context, name string) error
	LoadAllQueueMetadata(ctx context.Context) ([]*QueueMetadata, error)
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
	Steps       []Step         `json:"steps"`

	// non persisted fields
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

	// Save workflow immediately when it starts running so it's visible in status checks
	if w.store != nil {
		if err := w.store.SaveWorkflow(ctx, w); err != nil {
			return fmt.Errorf("failed to save workflow state when starting: %w", err)
		}
	}

	// move to the current step
	for i := w.CurrentStep; i < len(w.Steps); i++ {
		step := w.Steps[i]

		for _, stepHook := range w.beforeStepHooks {
			stepHook(ctx, w, &step)
		}

		var attempts uint = 0
		var stepErr error
		activity, ok := activities[step.Name]
		if !ok {
			return fmt.Errorf("activity '%s' not registered", step.Name)
		}

		var bo BackOff
		var maxAttempts uint = 1
		if step.RetryPolicy != nil {
			if step.RetryPolicy.BackOff != nil {
				bo = WithContext(step.RetryPolicy.BackOff, ctx)
				if step.RetryPolicy.MaxAttempts > 0 {
					maxAttempts = step.RetryPolicy.MaxAttempts
				}
			}
		}
		if bo == nil {
			// No retry policy or BackOff: single attempt, no retry
			bo = WithContext(NewStopBackOff(), ctx)
		}

		attempts = 0
		operation := func() error {
			attempts++
			ctxWithStep := context.WithValue(ctx, StepNameContextKey, step.Name)
			if step.Timeout > 0 {
				var cancel context.CancelFunc
				ctxWithStep, cancel = context.WithTimeout(ctxWithStep, step.Timeout)
				defer cancel()
			}
			// Panic safety
			func() {
				defer func() {
					if r := recover(); r != nil {
						stepErr = fmt.Errorf("panic in step '%s': %v", step.Name, r)
					}
				}()
				stepErr = activity(ctxWithStep, w.State)
			}()
			if ctxWithStep.Err() == context.DeadlineExceeded {
				stepErr = fmt.Errorf("step '%s' timed out after %v: %w", step.Name, step.Timeout, ctxWithStep.Err())
			}
			if errors.Is(stepErr, ErrFailWorkflowNow) {
				return PermanentError(stepErr)
			}
			// Only return permanent error when we've exhausted retries or if the error is nil
			if stepErr != nil {
				if attempts >= maxAttempts {
					return PermanentError(stepErr) // Mark as permanent on last attempt
				}
				return stepErr // Regular error for retry
			}
			return nil // No error
		}

		if err := Retry(operation, bo); err != nil {
			stepErr = err
		}
		// After backoff.Retry, stepErr has the last error (or nil)

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
