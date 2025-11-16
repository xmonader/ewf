package ewf

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// TestBackoffRecovery tests that backoff state can be properly recovered after serialization.
// This is critical for workflow persistence and recovery scenarios.
func TestBackoffRecovery(t *testing.T) {
	// Create a workflow with a retry policy using exponential backoff
	initialInterval := 10 * time.Millisecond
	maxInterval := 100 * time.Millisecond
	multiplier := 2.0

	// Create an exponential backoff and advance it a few times to build state
	originalBackoff := NewExponentialBackOff(initialInterval, maxInterval, multiplier)

	// Record the first few intervals
	var originalIntervals []time.Duration
	for i := 0; i < 3; i++ {
		interval := originalBackoff.NextBackOff()
		originalIntervals = append(originalIntervals, interval)
	}

	// After 3 calls to NextBackOff, the CurrentInterval should be initialInterval * multiplier^3
	// But the next call will return the current value before incrementing
	expectedCurrentInterval := initialInterval * time.Duration(multiplier*multiplier)
	if expectedCurrentInterval > maxInterval {
		expectedCurrentInterval = maxInterval
	}

	// Serialize the backoff
	serialized, err := MarshalBackOff(originalBackoff)
	if err != nil {
		t.Fatalf("Failed to serialize backoff: %v", err)
	}

	// Deserialize to a new backoff instance
	recoveredBackoff, err := UnmarshalBackOff(serialized)
	if err != nil {
		t.Fatalf("Failed to deserialize backoff: %v", err)
	}

	// Verify the recovered backoff continues from where it left off
	recoveredInterval := recoveredBackoff.NextBackOff()

	// The next interval should follow the exponential pattern
	expectedNextInterval := expectedCurrentInterval * time.Duration(multiplier)
	if expectedNextInterval > maxInterval {
		expectedNextInterval = maxInterval
	}

	// Allow for small floating point differences
	tolerance := time.Millisecond
	if diff := recoveredInterval - expectedNextInterval; diff > tolerance || diff < -tolerance {
		t.Errorf("Recovered backoff returned incorrect interval. Got %v, expected %v (within %v)",
			recoveredInterval, expectedNextInterval, tolerance)
	}

	t.Logf("Original intervals: %v", originalIntervals)
	t.Logf("After recovery, next interval: %v", recoveredInterval)
}

// TestWorkflowBackoffRecovery tests that a workflow with a retry policy can be
// serialized, stored, and recovered with its backoff state intact.
func TestWorkflowBackoffRecovery(t *testing.T) {
	// Create an in-memory store
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Logf("Failed to close store: %v", err)
		}
	}()

	// Create an engine
	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Register a step that will fail a few times to advance the backoff
	attempts := 0
	maxAttempts := 5
	engine.Register("retryStep", func(ctx context.Context, state State) error {
		attempts++
		state["attempts"] = attempts
		if attempts < maxAttempts {
			return fmt.Errorf("simulated failure, attempt %d", attempts)
		}
		return nil
	})

	// Create a workflow with exponential backoff
	initialInterval := 5 * time.Millisecond
	maxInterval := 50 * time.Millisecond
	multiplier := 2.0

	step := Step{
		Name: "retryStep",
		RetryPolicy: &RetryPolicy{
			MaxAttempts: uint(maxAttempts),
			BackOff:     ExponentialBackoff(initialInterval, maxInterval, multiplier),
		},
	}

	engine.RegisterTemplate("recovery-test", &WorkflowTemplate{
		Steps: []Step{step},
	})

	// Create and start the workflow
	wf, err := engine.NewWorkflow("recovery-test")
	if err != nil {
		t.Fatalf("Failed to create workflow: %v", err)
	}

	// Run the workflow for a few attempts but interrupt it quickly
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	// This should timeout before completion, simulating an interruption
	_ = engine.Run(ctx, wf)

	// Verify some attempts were made but not all
	currentAttempts, ok := wf.State["attempts"].(int)
	if !ok || currentAttempts == 0 {
		t.Fatalf("Expected some attempts to be made, got %v", wf.State["attempts"])
	}
	if currentAttempts >= maxAttempts {
		t.Fatalf("Expected workflow to be interrupted before completion, but it completed with %d attempts", currentAttempts)
	}

	t.Logf("Workflow interrupted after %d attempts", currentAttempts)

	// Serialize the workflow
	serialized, err := json.Marshal(wf)
	if err != nil {
		t.Fatalf("Failed to serialize workflow: %v", err)
	}

	// Deserialize to a new workflow instance
	var recoveredWf Workflow
	if err := json.Unmarshal(serialized, &recoveredWf); err != nil {
		t.Fatalf("Failed to deserialize workflow: %v", err)
	}

	// Re-register the activity for the recovered workflow
	engine2, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("Failed to create second engine: %v", err)
	}

	finalAttempts := 0
	engine2.Register("retryStep", func(ctx context.Context, state State) error {
		current, _ := state["attempts"].(int)
		finalAttempts = current + 1
		state["attempts"] = finalAttempts
		if finalAttempts < maxAttempts {
			return fmt.Errorf("simulated failure, attempt %d", finalAttempts)
		}
		return nil
	})

	// Resume the workflow
	if err := engine2.Run(context.Background(), &recoveredWf); err != nil {
		t.Fatalf("Failed to resume workflow: %v", err)
	}

	// Verify the workflow completed successfully
	if finalAttempts != maxAttempts {
		t.Errorf("Expected workflow to complete after %d attempts, but got %d", maxAttempts, finalAttempts)
	}

	t.Logf("Workflow completed after recovery. Total attempts: %d", finalAttempts)
}
