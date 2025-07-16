package ewf

import (
	"context"
	"testing"
)

func TestHooks_DoubleInvocationDueToSharedSlice(t *testing.T) {
	var beforeWorkflowCalls, afterWorkflowCalls int
	var beforeStepCalls, afterStepCalls int

	beforeWorkflow := func(ctx context.Context, w *Workflow) { beforeWorkflowCalls++ }
	afterWorkflow := func(ctx context.Context, w *Workflow, err error) { afterWorkflowCalls++ }
	beforeStep := func(ctx context.Context, w *Workflow, step *Step) { beforeStepCalls++ }
	afterStep := func(ctx context.Context, w *Workflow, step *Step, err error) { afterStepCalls++ }

	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	}()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	tmpl := &WorkflowTemplate{
		Steps: []Step{{Name: "step1"}},
		BeforeWorkflowHooks: []BeforeWorkflowHook{beforeWorkflow},
		AfterWorkflowHooks:  []AfterWorkflowHook{afterWorkflow},
		BeforeStepHooks:     []BeforeStepHook{beforeStep},
		AfterStepHooks:      []AfterStepHook{afterStep},
	}
	engine.RegisterTemplate("test", tmpl)

	// Register dummy activity for step1
	engine.Register("step1", func(ctx context.Context, state State) error { return nil })

	wf, err := engine.NewWorkflow("test")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	if err := engine.RunSync(context.Background(), wf); err != nil {
		t.Fatalf("RunSync failed: %v", err)
	}
	if beforeWorkflowCalls != 1 {
		t.Errorf("beforeWorkflow hook called %d times, expected 1", beforeWorkflowCalls)
	}
	if afterWorkflowCalls != 1 {
		t.Errorf("afterWorkflow hook called %d times, expected 1", afterWorkflowCalls)
	}
	if beforeStepCalls != 1 {
		t.Errorf("beforeStep hook called %d times, expected 1", beforeStepCalls)
	}
	if afterStepCalls != 1 {
		t.Errorf("afterStep hook called %d times, expected 1", afterStepCalls)
	}

	// Reset workflow to rerun steps
	wf.Status = StatusPending
	wf.CurrentStep = 0

	// Run again: counters should increment by 1 each
	if err := engine.RunSync(context.Background(), wf); err != nil {
		t.Fatalf("RunSync (second) failed: %v", err)
	}
	if beforeWorkflowCalls != 2 {
		t.Errorf("beforeWorkflow hook called %d times after second run, expected 2", beforeWorkflowCalls)
	}
	if afterWorkflowCalls != 2 {
		t.Errorf("afterWorkflow hook called %d times after second run, expected 2", afterWorkflowCalls)
	}
	if beforeStepCalls != 2 {
		t.Errorf("beforeStep hook called %d times after second run, expected 2", beforeStepCalls)
	}
	if afterStepCalls != 2 {
		t.Errorf("afterStep hook called %d times after second run, expected 2", afterStepCalls)
	}
}
