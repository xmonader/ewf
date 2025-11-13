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
	Queued      bool           `json:"queued"`

	// non persisted fields
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

// NewWorkflow creates a new workflow instance with the given name and options.
func NewWorkflow(name string) *Workflow {
	return &Workflow{
		UUID:      uuid.New().String(),
		Name:      name,
		Status:    StatusPending,
		Steps:     []Step{},
		State:     make(State),
		CreatedAt: time.Now(),
	}
}
