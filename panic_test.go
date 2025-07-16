package ewf

import (
	"context"
	"strings"
	"testing"
)

// TestStepPanic verifies that panics in steps are caught and treated as errors
func TestStepPanic(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	}()

	engine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	var attempts int
	engine.Register("PanickyStep", func(ctx context.Context, state State) error {
		attempts++
		panic("something went wrong!")
	})

	tmpl := &WorkflowTemplate{
		Steps: []Step{{
			Name: "PanickyStep",
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 2,
				Delay:       0,
			},
		}},
	}
	engine.RegisterTemplate("panic_workflow", tmpl)

	wf, err := engine.NewWorkflow("panic_workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	err = engine.RunSync(context.Background(), wf)
	if err == nil {
		t.Fatalf("expected workflow to fail due to panic, but it succeeded")
	}
	if !strings.Contains(err.Error(), "panic in step") {
		t.Errorf("expected panic error, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts due to retry policy, got %d", attempts)
	}
}
