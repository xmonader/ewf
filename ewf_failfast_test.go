package ewf

import (
	"context"
	"testing"
	"time"
	"errors"
	"fmt"
)

// TestFailFastErrorBypassesRetries tests that ErrFailWorkflowNow causes workflow to fail immediately.
func TestFailFastErrorBypassesRetries(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
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
	if !errors.Is(err, ErrFailWorkflowNow) {
		t.Fatalf("expected workflow to fail with ErrFailWorkflowNow, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected step to be called once, got %d", calls)
	}
}

// TestWrappedFailFastErrorBypassesRetries tests that a wrapped ErrFailWorkflowNow is still detected and fails the workflow immediately.
func TestWrappedFailFastErrorBypassesRetries(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
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
	engine.Register("failfast", func(ctx context.Context, state State) error {
		calls++
		return fmt.Errorf("wrapped: %w", ErrFailWorkflowNow)
	})
	engine.RegisterTemplate("failfast-test-wrap", &WorkflowTemplate{
		Steps: []Step{{Name: "failfast"}},
	})
	wf, err := engine.NewWorkflow("failfast-test-wrap")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	err = engine.RunSync(context.Background(), wf)
	if !errors.Is(err, ErrFailWorkflowNow) {
		t.Fatalf("expected workflow to fail with wrapped ErrFailWorkflowNow, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected step to be called once, got %d", calls)
	}
}

// TestNormalRetryPolicyStillWorks tests that normal retry policy works as expected.
func TestNormalRetryPolicyStillWorks(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
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
	engine.Register("fail", func(ctx context.Context, state State) error {
		calls++
		return context.DeadlineExceeded
	})
	engine.RegisterTemplate("retry-test", &WorkflowTemplate{
		Steps: []Step{{
			Name:        "fail",
			RetryPolicy: &RetryPolicy{MaxAttempts: 2, BackOff: ConstantBackoff(10 * time.Millisecond)},
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
	if calls != 2 {
		t.Fatalf("expected step to be called 2 times (for retries), got %d", calls)
	}
}
