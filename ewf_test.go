package ewf

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWorkflow_Run_Simple(t *testing.T) {
	engine, err := NewEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	var step1Done bool
	engine.Register("step1", func(ctx context.Context, state State) error {
		step1Done = true
		state["step1output"] = "output from step1"
		return nil
	})

	var step2Done bool
	engine.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		state["step2output"] = "output from step2"
		return nil
	})

	step1 := Step{Name: "step1"}
	step2 := Step{Name: "step2"}

	engine.RegisterTemplate("basic-workflow", &WorkflowTemplate{
		Steps: []Step{step1, step2},
	})

	wf, err := engine.NewWorkflow("basic-workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	err = engine.RunSync(context.Background(), wf)
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
	engine, err := NewEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	step1Done := false
	engine.Register("step1", func(ctx context.Context, state State) error {
		return fmt.Errorf("transient error")
	})

	var step2Done bool
	engine.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		state["step2output"] = "output from step2"
		return nil
	})

	step1 := Step{Name: "step1"}
	step2 := Step{Name: "step2"}

	engine.RegisterTemplate("basic-workflow-retry-success", &WorkflowTemplate{
		Steps: []Step{step1, step2},
	})

	wf, err := engine.NewWorkflow("basic-workflow-retry-success")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	err = engine.RunSync(context.Background(), wf)

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
	engine, err := NewEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	step1Attempts := 0
	step1Done := false
	engine.Register("step1", func(ctx context.Context, state State) error {
		step1Attempts++
		if step1Attempts < 3 {
			return fmt.Errorf("transient error")
		}
		step1Done = true
		state["final_attempts"] = step1Attempts
		return nil
	})

	var step2Done bool
	engine.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		state["step2output"] = "output from step2"
		return nil
	})

	step1 := Step{Name: "step1", RetryPolicy: &RetryPolicy{MaxAttempts: 3, Delay: 1 * time.Millisecond}}
	step2 := Step{Name: "step2"}

	engine.RegisterTemplate("basic-workflow-retry", &WorkflowTemplate{
		Steps: []Step{step1, step2},
	})

	wf, err := engine.NewWorkflow("basic-workflow-retry")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	err = engine.RunSync(context.Background(), wf)

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
	engine, err := NewEngine(nil)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	var step1Done bool
	engine.Register("step1", func(ctx context.Context, state State) error {
		return fmt.Errorf("step1err")
	})

	var step2Done bool
	engine.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		state["step2output"] = "output from step2"
		return nil
	})

	step1 := Step{Name: "step1"}
	step2 := Step{Name: "step2", RetryPolicy: &RetryPolicy{MaxAttempts: 3, Delay: 1 * time.Millisecond}}

	engine.RegisterTemplate("basic-workflow-retry-failure", &WorkflowTemplate{
		Steps: []Step{step1, step2},
	})

	wf, err := engine.NewWorkflow("basic-workflow-retry-failure")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	err = engine.RunSync(context.Background(), wf)

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
