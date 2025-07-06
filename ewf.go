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

type Workflow struct {
	ID          string         `json:"id"`
	Status      WorkflowStatus `json:"status"`
	Steps       []Step         `json:"steps"`
	State       State          `json:"state"`
	CurrentStep int            `json:"current_step"`
}

func NewWorkflow(id string) *Workflow {
	return &Workflow{
		ID:     id,
		Status: StatusPending,
		Steps:  []Step{},
		State:  make(State),
	}
}

func (w *Workflow) Run() (err error) {
	if w.Status == StatusCompleted {
		return nil // Already completed
	}
	w.Status = StatusRunning
	// move to the current step
	for i := w.CurrentStep; i < len(w.Steps); i++ {
		step := w.Steps[i]
		// execute the step with its retry policy (or default if it's nil)
		// break if you reached the max attempts or the step was successful
		attempts := 1
		var stepErr error
		for {
			stepErr = step.Fn(context.Background(), w.State)
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
		if stepErr != nil {
			w.Status = StatusFailed
			return fmt.Errorf("workflow failed: step %s failed: %w", step.Name, stepErr)
		}
	}
	w.Status = StatusCompleted
	return
}
