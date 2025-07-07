package ewf

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWorkflow_Run_Simple(t *testing.T) {
	var step1Done bool
	step1 := Step{
		Name: "Step 1",
		Fn: func(ctx context.Context, state State) error {
			step1Done = true
			state["step1output"] = "output from step1"
			return nil
		},
	}
	var step2Done bool
	step2 := Step{
		Name: "Step 2",
		Fn: func(ctx context.Context, state State) error {
			step2Done = true
			state["step2output"] = "output from step2"
			return nil
		},
	}

	wf := NewWorkflow("basic-workflow", WithSteps(step1, step2))
	err := wf.Run(context.Background())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !step1Done {
		t.Errorf("step1 not executed")
	}
	if !step2Done {
		t.Errorf("step2 not executed")
	}

	if wf.CurrentStep != 2 {
		t.Errorf("expected currentStep to be 2, got %d", wf.CurrentStep)
	}
	if _, ok := wf.State["step1output"]; !ok {
		t.Errorf("step1 output not found")
	}
	if _, ok := wf.State["step2output"]; !ok {
		t.Errorf("step2 output not found")
	}
}

func TestWorkflow_Run_Fail(t *testing.T) {
	step1Done := false
	step1 := Step{
		Name: "Step 1",
		Fn: func(ctx context.Context, state State) error {
			return fmt.Errorf("transient error")
		},
	}
	var step2Done bool
	step2 := Step{
		Name: "Step 2",
		Fn: func(ctx context.Context, state State) error {
			step2Done = true
			state["step2output"] = "output from step2"
			return nil
		},
	}

	wf := NewWorkflow("basic-workflow-retry-success", WithSteps(step1, step2))
	err := wf.Run(context.Background())

	if err == nil {
		t.Errorf("expected error but err is nil")
	}
	if step1Done {
		t.Errorf("step1 should not have been executed")
	}
	if step2Done {
		t.Errorf("step2 should not have been executed")
	}

	if wf.CurrentStep != 0 {
		t.Errorf("expected currentStep to be 0, got %d", wf.CurrentStep)
	}
}

func TestWorkflow_Run_Retry(t *testing.T) {
	step1Attempts := 0
	step1Done := false
	step1 := Step{
		Name: "Step 1",
		Fn: func(ctx context.Context, state State) error {
			step1Attempts++
			if step1Attempts < 3 {
				return fmt.Errorf("transient error")
			}
			step1Done = true
			state["final_attempts"] = step1Attempts
			return nil
		},
		RetryPolicy: &RetryPolicy{MaxAttempts: 3, Delay: 1 * time.Millisecond},
	}
	var step2Done bool
	step2 := Step{
		Name: "Step 2",
		Fn: func(ctx context.Context, state State) error {
			step2Done = true
			state["step2output"] = "output from step2"
			return nil
		},
	}

	wf := NewWorkflow("basic-workflow-retry", WithSteps(step1, step2))
	err := wf.Run(context.Background())

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !step1Done {
		t.Errorf("step1 not executed")
	}
	if !step2Done {
		t.Errorf("step2 not executed")
	}

	if wf.CurrentStep != 2 {
		t.Errorf("expected currentStep to be 2, got %d", wf.CurrentStep)
	}

	if _, ok := wf.State["step2output"]; !ok {
		t.Errorf("step2 output not found")
	}
	if wf.State["final_attempts"] != 3 {
		t.Errorf("expected final_attempts to be 3, got %d", wf.State["final_attempts"])
	}
}

func TestWorkflow_Run_Retry_Failure(t *testing.T) {
	var step1Done bool
	step1 := Step{
		Name: "Step 1",
		Fn: func(ctx context.Context, state State) error {
			return fmt.Errorf("step1err")
		},
	}
	var step2Done bool
	step2 := Step{
		Name: "Step 2",
		Fn: func(ctx context.Context, state State) error {
			step2Done = true
			state["step2output"] = "output from step2"
			return nil
		},
		RetryPolicy: &RetryPolicy{MaxAttempts: 3, Delay: 1 * time.Millisecond},
	}

	wf := NewWorkflow("basic-workflow-retry-failure", WithSteps(step1, step2))
	err := wf.Run(context.Background())

	if err == nil {
		t.Errorf("expected error but err is nil")
	}
	if step1Done {
		t.Errorf("step1 should not have been executed")
	}
	if step2Done {
		t.Errorf("step2 should not have been executed")
	}

	if wf.CurrentStep != 0 {
		t.Errorf("expected currentStep to be 0, got %d", wf.CurrentStep)
	}
	if _, ok := wf.State["step2output"]; ok {
		t.Errorf("step2 output should not have been found")
	}
}
