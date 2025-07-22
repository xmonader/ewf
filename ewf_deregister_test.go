package ewf

import (
	"context"
	"testing"
)

func TestEngine_DeregisterTemplate(t *testing.T) {
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

	step1Done := false
	step2Done := false
	engine.Register("step1", func(ctx context.Context, state State) error { step1Done = true; return nil })
	engine.Register("step2", func(ctx context.Context, state State) error { step2Done = true; return nil })

	engine.RegisterTemplate("deregister-workflow", &WorkflowTemplate{
		Steps: []Step{{Name: "step1"}, {Name: "step2"}},
	})

	// Ensure workflow can be created and run before deregister
	wf, err := engine.NewWorkflow("deregister-workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	if err := engine.RunSync(context.Background(), wf); err != nil {
		t.Fatalf("failed to run workflow: %v", err)
	}
	if !step1Done || !step2Done {
		t.Errorf("steps not executed before deregister")
	}

	// Deregister and ensure workflow cannot be created anymore
	engine.Deregister("deregister-workflow")
	_, err = engine.NewWorkflow("deregister-workflow")
	if err == nil {
		t.Errorf("expected error when creating workflow from deregistered template, got nil")
	}

	// Re-register and ensure it works again
	engine.RegisterTemplate("deregister-workflow", &WorkflowTemplate{
		Steps: []Step{{Name: "step1"}},
	})
	_, err = engine.NewWorkflow("deregister-workflow")
	if err != nil {
		t.Fatalf("failed to create workflow after re-register: %v", err)
	}
}
