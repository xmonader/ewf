package ewf

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestStepTimeout verifies that steps respect their timeout setting
func TestStepTimeout(t *testing.T) {
	// Create a test store
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	}()

	// Create an engine with the store
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Track if the slow step was actually interrupted
	var slowStepInterrupted bool

	// Register a step that takes longer than its timeout
	engine.Register("SlowStep", func(ctx context.Context, state State) error {
		// Check if context is done frequently
		select {
		case <-time.After(500 * time.Millisecond):
			// This should never complete if timeout works correctly
			return nil
		case <-ctx.Done():
			// This should happen when timeout is triggered
			slowStepInterrupted = true
			return ctx.Err() // Return the context error (deadline exceeded)
		}
	})

	// Create a workflow template with the slow step and a short timeout
	tmpl := &WorkflowTemplate{
		Steps: []Step{
			{
				Name:    "SlowStep",
				Timeout: 100 * time.Millisecond, // Very short timeout
			},
		},
	}
	engine.RegisterTemplate("timeout_workflow", tmpl)

	// Create and run the workflow
	wf, err := engine.NewWorkflow("timeout_workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Run the workflow - it should fail with timeout
	err = engine.RunSync(context.Background(), wf)
	
	// Verify the workflow failed due to timeout
	if err == nil {
		t.Fatalf("expected workflow to fail with timeout error, but it succeeded")
	}
	
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	
	if !slowStepInterrupted {
		t.Fatalf("step was not interrupted by timeout")
	}
}

// TestStepTimeoutWithRetry verifies that step timeout applies to each retry attempt
func TestStepTimeoutWithRetry(t *testing.T) {
	// Create a test store
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	}()

	// Create an engine with the store
	engine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Track retry attempts
	var attempts int

	// Register a step that times out on each attempt
	engine.Register("RetryStep", func(ctx context.Context, state State) error {
		attempts++
		
		// Always time out
		select {
		case <-time.After(200 * time.Millisecond):
			return nil // Should never reach here
		case <-ctx.Done():
			return ctx.Err() // Return the context error (deadline exceeded)
		}
	})

	// Create a workflow template with retry policy and timeout
	tmpl := &WorkflowTemplate{
		Steps: []Step{
			{
				Name:    "RetryStep",
				Timeout: 50 * time.Millisecond, // Short timeout
				RetryPolicy: &RetryPolicy{
					MaxAttempts: 3,
					Delay:       10 * time.Millisecond,
				},
			},
		},
	}
	engine.RegisterTemplate("retry_timeout_workflow", tmpl)

	// Create and run the workflow
	wf, err := engine.NewWorkflow("retry_timeout_workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Run the workflow - it should fail after all retries
	err = engine.RunSync(context.Background(), wf)
	
	// Verify the workflow failed due to timeout after retries
	if err == nil {
		t.Fatalf("expected workflow to fail with timeout error, but it succeeded")
	}
	
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout error, got: %v", err)
	}
	
	// Verify all retry attempts were made
	if attempts != 3 {
		t.Fatalf("expected 3 retry attempts, got %d", attempts)
	}
}
