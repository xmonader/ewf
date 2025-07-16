package ewf

import (
	"context"
	"testing"
)

func TestFailFastErrorBypassesRetries(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	defer store.Close()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	engine.Register("failfast", func(ctx context.Context, state State) error {
		calls++
		return ErrFailWorkflowNow
	})
	engine.RegisterTemplate("failfast-test", &WorkflowTemplate{
		Steps: []Step{{Name: "failfast"}},
	})
	wf, err := engine.NewWorkflow("failfast-test")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	err = engine.RunSync(context.Background(), wf)
	if err == nil {
		t.Fatal("expected workflow to fail with ErrFailWorkflowNow, got nil")
	}
	if calls != 1 {
		t.Fatalf("expected step to be called once, got %d", calls)
	}
}

func TestNormalRetryPolicyStillWorks(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
	}
	defer store.Close()
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	engine.Register("fail", func(ctx context.Context, state State) error {
		calls++
		return context.DeadlineExceeded
	})
	engine.RegisterTemplate("retry-test", &WorkflowTemplate{
		Steps: []Step{{
			Name:        "fail",
			RetryPolicy: &RetryPolicy{MaxAttempts: 3, Delay: 0},
		}},
	})
	wf, err := engine.NewWorkflow("retry-test")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	err = engine.RunSync(context.Background(), wf)
	if err == nil {
		t.Fatal("expected workflow to fail, got nil")
	}
	if calls != 3 {
		t.Fatalf("expected step to be called 3 times (for retries), got %d", calls)
	}
}
