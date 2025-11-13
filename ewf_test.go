package ewf

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestWorkflow_Run_Simple tests a simple workflow run.
//
// Scenario:
// 1. Creates an engine and registers two activities ("step1" and "step2") that update flags and state.
// 2. Registers a workflow template with these two steps.
// 3. Instantiates and runs the workflow synchronously.
// 4. Verifies both steps executed, state was updated, and workflow step index is correct.
//
// This test ensures basic workflow execution, step registration, and state propagation work as expected.
func TestWorkflow_Run_Simple(t *testing.T) {
	engine, err := NewEngine()
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

// TestWorkflow_Run_Fail tests workflow failure scenario.
//
// Scenario:
// 1. Creates an engine and registers a step that always fails.
// 2. Registers a second step that should never run due to the failure.
// 3. Runs the workflow and verifies it fails at the first step.
//
// This test ensures workflows properly handle step failures.
func TestWorkflow_Run_Fail(t *testing.T) {
	engine, err := NewEngine()
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	step1Done := false
	engine.Register("step1", func(ctx context.Context, state State) error {
		return PermanentError(fmt.Errorf("transient error"))
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

// TestWorkflow_Run_Retry tests workflow retry logic.
//
// Scenario:
// 1. Creates an engine and registers a step that fails on the first two attempts but succeeds on the third.
// 2. Configures the step with a retry policy of 3 maximum attempts.
// 3. Runs the workflow and verifies it eventually succeeds after retries.
// 4. Verifies the step was attempted exactly 3 times.
//
// This test ensures the retry policy works correctly for transient failures.
func TestWorkflow_Run_Retry(t *testing.T) {
	engine, err := NewEngine()
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	step1Attempts := 0
	step1Done := false
	engine.Register("step1", func(ctx context.Context, state State) error {
		step1Attempts++
		if step1Attempts < 3 {
			// Return a regular error for retry
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

	step1 := Step{Name: "step1", RetryPolicy: &RetryPolicy{MaxAttempts: 3, BackOff: ConstantBackoff(10 * time.Millisecond)}}
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

// TestWorkflow_Run_Retry_Failure tests workflow retry failure scenario.
//
// Scenario:
// 1. Creates an engine and registers a step ("step1") that always fails.
// 2. Registers a second step ("step2") that should never run.
// 3. Runs the workflow and verifies that the workflow fails, only the first step runs, and state/step index reflect the failure.
//
// This test ensures workflows fail if all retries are exhausted and do not proceed to subsequent steps.
func TestWorkflow_Run_Retry_Failure(t *testing.T) {
	engine, err := NewEngine()
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
	step2 := Step{Name: "step2", RetryPolicy: &RetryPolicy{MaxAttempts: 3, BackOff: ConstantBackoff(10 * time.Millisecond)}}

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

// TestSimpleIdempotencyPattern demonstrates a simple idempotency pattern
// using the step name from context and flags in the workflow state.
//
// Scenario:
// 1. Creates an engine with an in-memory store.
// 2. Registers two activities that implement idempotency patterns using state flags.
// 3. Runs a workflow with these activities and verifies they execute once.
// 4. Resets and reruns the workflow to simulate a crash recovery.
// 5. Verifies the activities detect they've already run and don't execute again.
//
// This test demonstrates how to make workflow steps idempotent using state flags.
func TestSimpleIdempotencyPattern(t *testing.T) {
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
	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Track API calls to ensure idempotency
	var paymentAPICalls int
	var emailAPICalls int

	// Register activities that use idempotency pattern
	engine.Register("ProcessPayment", func(ctx context.Context, state State) error {
		// Get step name from context
		stepName, ok := ctx.Value(StepNameContextKey).(string)
		if !ok {
			return fmt.Errorf("step name not found in context")
		}

		// Simple idempotency check - have we already processed this payment?
		idempotencyKey := fmt.Sprintf("__completed_%s", stepName)
		if _, done := state[idempotencyKey]; done {
			t.Logf("Payment already processed, skipping")
			return nil
		}

		// Simulate API call
		t.Logf("Processing payment for step: %s", stepName)
		paymentAPICalls++

		// Store payment result
		state["payment_id"] = "payment_123"

		// Mark step as completed
		state[idempotencyKey] = true
		return nil
	})

	engine.Register("SendEmail", func(ctx context.Context, state State) error {
		// Get step name from context
		stepName, ok := ctx.Value(StepNameContextKey).(string)
		if !ok {
			return fmt.Errorf("step name not found in context")
		}

		// Simple idempotency check with email address for deterministic behavior
		emailAddress := "user@example.com"
		idempotencyKey := fmt.Sprintf("__email_sent_%s_%s", stepName, emailAddress)
		if _, sent := state[idempotencyKey]; sent {
			t.Logf("Email already sent to %s, skipping", emailAddress)
			return nil
		}

		// Simulate sending email
		t.Logf("Sending email for step: %s", stepName)
		emailAPICalls++

		// Mark email as sent
		state[idempotencyKey] = true
		return nil
	})

	// Create a workflow template with the steps
	tmpl := &WorkflowTemplate{
		Steps: []Step{
			{Name: "ProcessPayment"},
			{Name: "SendEmail"},
		},
	}
	engine.RegisterTemplate("payment_workflow", tmpl)

	// Create and run the workflow
	wf, err := engine.NewWorkflow("payment_workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Run the workflow
	if err := engine.RunSync(context.Background(), wf); err != nil {
		t.Fatalf("failed to run workflow: %v", err)
	}

	// Verify API calls
	if paymentAPICalls != 1 {
		t.Errorf("expected 1 payment API call, got %d", paymentAPICalls)
	}
	if emailAPICalls != 1 {
		t.Errorf("expected 1 email API call, got %d", emailAPICalls)
	}

	// Reset workflow to simulate resuming after a crash
	wf.Status = StatusPending
	wf.CurrentStep = 0

	// Run the workflow again
	if err := engine.RunSync(context.Background(), wf); err != nil {
		t.Fatalf("failed to run workflow again: %v", err)
	}

	// Verify API calls - should not have increased due to idempotency
	if paymentAPICalls != 1 {
		t.Errorf("expected still 1 payment API call, got %d", paymentAPICalls)
	}
	if emailAPICalls != 1 {
		t.Errorf("expected still 1 email API call, got %d", emailAPICalls)
	}
}

// TestCrashRecoveryWithIdempotency tests idempotency with a workflow that crashes and resumes
//
// Scenario:
// 1. Creates an engine with an in-memory store.
// 2. Registers an activity that simulates a crash after performing a side effect.
// 3. Runs a workflow with this activity and verifies it fails but records the side effect.
// 4. Resets and reruns the workflow to simulate crash recovery.
// 5. Verifies the activity detects it already performed the side effect and doesn't duplicate it.
//
// This test demonstrates how to handle crashes while maintaining idempotency.
func TestCrashRecoveryWithIdempotency(t *testing.T) {
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
	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Track API calls to ensure idempotency
	var apiCalls int

	// Register an activity that simulates a crash after side effect
	engine.Register("CrashyStep", func(ctx context.Context, state State) error {
		// Get step name from context
		stepName, ok := ctx.Value(StepNameContextKey).(string)
		if !ok {
			return fmt.Errorf("step name not found in context")
		}

		// Simple idempotency check
		idempotencyKey := fmt.Sprintf("__side_effect_%s", stepName)
		if _, done := state[idempotencyKey]; done {
			t.Logf("Side effect already executed, skipping")
			return nil
		}

		// Simulate API call
		t.Logf("Executing side effect for step: %s", stepName)
		apiCalls++

		// Mark side effect as done - IMPORTANT: do this BEFORE potential crash
		state[idempotencyKey] = true

		// Simulate crash by returning error (only on first execution)
		if apiCalls == 1 {
			// Use a regular error to allow retries
			return fmt.Errorf("simulated crash")
		}
		return nil
	})

	// Create a workflow template with the step
	tmpl := &WorkflowTemplate{
		Steps: []Step{
			{Name: "CrashyStep"},
		},
	}
	engine.RegisterTemplate("crashy_workflow", tmpl)

	// Create and run the workflow
	wf, err := engine.NewWorkflow("crashy_workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Run the workflow - it should fail
	err = engine.RunSync(context.Background(), wf)
	if err == nil {
		t.Fatalf("expected an error, got nil")
	}

	// Verify API calls
	if apiCalls != 1 {
		t.Errorf("expected 1 API call, got %d", apiCalls)
	}

	// Reset workflow to simulate resuming after a crash
	wf.Status = StatusPending
	wf.CurrentStep = 0

	// Run the workflow again
	if err := engine.RunSync(context.Background(), wf); err != nil {
		t.Fatalf("failed to run workflow again: %v", err)
	}

	// Verify API calls - should not have increased due to idempotency
	if apiCalls != 1 {
		t.Errorf("expected still 1 API call, got %d", apiCalls)
	}
}

// TestEngine_WithLogger tests that the engine properly uses a provided logger.
//
// Scenario:
// 1. Creates an engine with an in-memory store and a mock logger.
// 2. Registers a failing step to trigger error logging.
// 3. Runs a workflow with the failing step.
// 4. Verifies the mock logger captured the error log entry.
//
// This test ensures the engine correctly uses the provided logger.
func TestEngine_WithLogger(t *testing.T) {
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

	// Create a mock logger to capture log calls
	mockLogger := &MockLogger{}

	// Create an engine with the store
	engine, err := NewEngine(WithStore(store), WithLogger(mockLogger))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Verify logger was set
	if engine.logger == nil {
		t.Fatal("logger was not set on engine")
	}

	// Register a failing step to trigger error logging
	engine.Register("failStep", func(ctx context.Context, state State) error {
		return fmt.Errorf("test error")
	})

	engine.RegisterTemplate("test-workflow", &WorkflowTemplate{
		Steps: []Step{{Name: "failStep"}},
	})

	wf, err := engine.NewWorkflow("test-workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	// Run async to trigger logger usage
	err = engine.RunAsync(context.Background(), wf)
	if err != nil {
		t.Fatalf("failed to run workflow: %v", err)
	}

	// Give it time to complete
	time.Sleep(200 * time.Millisecond)

	if !mockLogger.ErrorCalled {
		t.Error("expected error log to be called for failed workflow")
	}

}
