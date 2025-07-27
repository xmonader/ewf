package ewf

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestConstantBackOff(t *testing.T) {
	interval := 50 * time.Millisecond
	b := NewConstantBackOff(interval)

	// Test NextBackOff returns the correct interval
	if next := b.NextBackOff(); next != interval {
		t.Errorf("Expected interval %v, got %v", interval, next)
	}

	// Test Reset is a no-op for constant backoff
	b.Reset()
	if next := b.NextBackOff(); next != interval {
		t.Errorf("After reset, expected interval %v, got %v", interval, next)
	}
}

func TestExponentialBackOff(t *testing.T) {
	initialInterval := 50 * time.Millisecond
	maxInterval := 1 * time.Second
	multiplier := 2.0
	b := NewExponentialBackOff(initialInterval, maxInterval, multiplier)

	// Test initial interval
	if next := b.NextBackOff(); next != initialInterval {
		t.Errorf("Initial interval should be %v, got %v", initialInterval, next)
	}

	// Test subsequent intervals increase by multiplier
	next := b.NextBackOff()
	expected := time.Duration(float64(initialInterval) * multiplier)
	if next != expected {
		t.Errorf("Expected next interval to be %v, got %v", expected, next)
	}

	// Test that intervals are capped at maxInterval
	for i := 0; i < 10; i++ {
		b.NextBackOff()
	}
	if next := b.NextBackOff(); next > maxInterval {
		t.Errorf("Interval exceeded maxInterval: %v > %v", next, maxInterval)
	}

	// Test Reset
	b.Reset()
	if next := b.NextBackOff(); next != initialInterval {
		t.Errorf("After reset, expected initial interval %v, got %v", initialInterval, next)
	}
}

func TestExponentialBackOffWithJitter(t *testing.T) {
	initialInterval := 100 * time.Millisecond
	maxInterval := 1 * time.Second
	multiplier := 2.0
	jitter := 0.5 // 50% jitter
	b := NewExponentialBackOffWithJitter(initialInterval, maxInterval, multiplier, jitter)

	// With jitter, we can only test that the value is within expected bounds
	next := b.NextBackOff()
	delta := time.Duration(float64(initialInterval) * jitter)
	minExpected := initialInterval - delta
	maxExpected := initialInterval + delta

	if next < minExpected || next > maxExpected {
		t.Errorf("With jitter, expected interval between %v and %v, got %v", minExpected, maxExpected, next)
	}
}

func TestZeroBackOff(t *testing.T) {
	b := NewZeroBackOff()
	if next := b.NextBackOff(); next != 0 {
		t.Errorf("ZeroBackOff should return 0, got %v", next)
	}
}

func TestStopBackOff(t *testing.T) {
	b := NewStopBackOff()
	if next := b.NextBackOff(); next != Stop {
		t.Errorf("StopBackOff should return Stop (%v), got %v", Stop, next)
	}
}

func TestWithContext(t *testing.T) {
	interval := 50 * time.Millisecond
	b := NewConstantBackOff(interval)

	// Test with a valid context
	ctx := context.Background()
	backOffWithCtx := WithContext(b, ctx)
	if next := backOffWithCtx.NextBackOff(); next != interval {
		t.Errorf("Expected interval %v, got %v", interval, next)
	}

	// Test with a cancelled context
	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel()
	backOffWithCancelledCtx := WithContext(b, ctxCancelled)
	if next := backOffWithCancelledCtx.NextBackOff(); next != Stop {
		t.Errorf("Expected Stop with cancelled context, got %v", next)
	}
}

func TestRetry(t *testing.T) {
	b := NewConstantBackOff(5 * time.Millisecond)

	// Test successful retry
	attempts := 0
	err := Retry(func() error {
		attempts++
		if attempts < 3 {
			return errors.New("temporary error")
		}
		return nil
	}, b)

	if err != nil {
		t.Errorf("Expected successful retry, got error: %v", err)
	}
	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}

	// Test with permanent error
	attempts = 0
	permanentErr := errors.New("permanent error")
	err = Retry(func() error {
		attempts++
		return PermanentError(permanentErr)
	}, b)

	if !errors.Is(err, permanentErr) {
		t.Errorf("Expected permanent error, got: %v", err)
	}
	if attempts != 1 {
		t.Errorf("Expected 1 attempt with permanent error, got %d", attempts)
	}
}

func TestSerialization(t *testing.T) {
	// Test ConstantBackOffImpl serialization
	constBackoff := &ConstantBackOffImpl{Interval: 100 * time.Millisecond}
	constJSON, err := json.Marshal(constBackoff)
	if err != nil {
		t.Fatalf("Failed to marshal ConstantBackOffImpl: %v", err)
	}

	var constDeserialized ConstantBackOffImpl
	err = json.Unmarshal(constJSON, &constDeserialized)
	if err != nil {
		t.Fatalf("Failed to unmarshal ConstantBackOffImpl: %v", err)
	}

	if constDeserialized.Interval != constBackoff.Interval {
		t.Errorf("Deserialized interval doesn't match: expected %v, got %v", 
			constBackoff.Interval, constDeserialized.Interval)
	}

	// Test ExponentialBackOffImpl serialization
	expBackoff := &ExponentialBackOffImpl{
		InitialInterval:     100 * time.Millisecond,
		MaxInterval:         10 * time.Second,
		Multiplier:          2.0,
		RandomizationFactor: 0.5,
		MaxElapsedTime:      30 * time.Second,
	}
	expJSON, err := json.Marshal(expBackoff)
	if err != nil {
		t.Fatalf("Failed to marshal ExponentialBackOffImpl: %v", err)
	}

	var expDeserialized ExponentialBackOffImpl
	err = json.Unmarshal(expJSON, &expDeserialized)
	if err != nil {
		t.Fatalf("Failed to unmarshal ExponentialBackOffImpl: %v", err)
	}

	// Check all serialized fields
	if expDeserialized.InitialInterval != expBackoff.InitialInterval {
		t.Errorf("InitialInterval doesn't match: expected %v, got %v", 
			expBackoff.InitialInterval, expDeserialized.InitialInterval)
	}
	if expDeserialized.MaxInterval != expBackoff.MaxInterval {
		t.Errorf("MaxInterval doesn't match: expected %v, got %v", 
			expBackoff.MaxInterval, expDeserialized.MaxInterval)
	}
	if expDeserialized.Multiplier != expBackoff.Multiplier {
		t.Errorf("Multiplier doesn't match: expected %v, got %v", 
			expBackoff.Multiplier, expDeserialized.Multiplier)
	}
	if expDeserialized.RandomizationFactor != expBackoff.RandomizationFactor {
		t.Errorf("RandomizationFactor doesn't match: expected %v, got %v", 
			expBackoff.RandomizationFactor, expDeserialized.RandomizationFactor)
	}
	if expDeserialized.MaxElapsedTime != expBackoff.MaxElapsedTime {
		t.Errorf("MaxElapsedTime doesn't match: expected %v, got %v", 
			expBackoff.MaxElapsedTime, expDeserialized.MaxElapsedTime)
	}

	// Check that non-serialized fields were correctly initialized to zero values
	if expDeserialized.CurrentInterval != 0 {
		t.Errorf("CurrentInterval should be zero after deserialization, got %v", 
			expDeserialized.CurrentInterval)
	}
	if expDeserialized.Clock != nil {
		t.Errorf("Clock should be nil after deserialization")
	}
}

func TestWorkflowSerialization(t *testing.T) {
	// Create a workflow with retry policies
	workflow := NewWorkflow("test-workflow")
	
	// Add steps with different backoff strategies
	workflow.Steps = []Step{
		{
			Name: "constant-backoff",
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 3,
				BackOff:     NewConstantBackOff(100 * time.Millisecond),
			},
		},
		{
			Name: "exponential-backoff",
			RetryPolicy: &RetryPolicy{
				MaxAttempts: 5,
				BackOff:     NewExponentialBackOff(100*time.Millisecond, 1*time.Second, 2.0),
			},
		},
	}

	// Check if workflow can be serialized with our custom JSON marshaling
	data, err := json.Marshal(workflow)
	if err != nil {
		t.Fatalf("Failed to marshal workflow with custom serialization: %v", err)
	}

	// Deserialize and verify
	var deserializedWorkflow Workflow
	err = json.Unmarshal(data, &deserializedWorkflow)
	if err != nil {
		t.Fatalf("Failed to unmarshal workflow: %v", err)
	}

	// Verify retry policies were properly serialized/deserialized
	if len(deserializedWorkflow.Steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(deserializedWorkflow.Steps))
	}

	// Check constant backoff step
	if deserializedWorkflow.Steps[0].RetryPolicy == nil {
		t.Errorf("RetryPolicy for step 0 is nil")
	} else if deserializedWorkflow.Steps[0].RetryPolicy.MaxAttempts != 3 {
		t.Errorf("Expected MaxAttempts=3, got %d", deserializedWorkflow.Steps[0].RetryPolicy.MaxAttempts)
	}

	// Check exponential backoff step
	if deserializedWorkflow.Steps[1].RetryPolicy == nil {
		t.Errorf("RetryPolicy for step 1 is nil")
	} else if deserializedWorkflow.Steps[1].RetryPolicy.MaxAttempts != 5 {
		t.Errorf("Expected MaxAttempts=5, got %d", deserializedWorkflow.Steps[1].RetryPolicy.MaxAttempts)
	}
}
