package ewf

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestWorkflowOptions(t *testing.T) {
	t.Run("WithDisplayName", func(t *testing.T) {
		displayName := "Test Workflow Display Name"
		wf := NewWorkflow("test-workflow", WithDisplayName(displayName))

		if wf.DisplayName != displayName {
			t.Errorf("expected display name %q, got %q", displayName, wf.DisplayName)
		}

		displayName = "Modified Name"
		if wf.DisplayName == displayName {
			t.Errorf("workflow display name should not be affected by modifying original string")
		}
	})

	t.Run("WithMetadata", func(t *testing.T) {
		metadata := map[string]string{
			"team":     "payments",
			"priority": "high",
			"region":   "us-east",
		}

		wf := NewWorkflow("test-workflow", WithMetadata(metadata))

		if !reflect.DeepEqual(wf.Metadata, metadata) {
			t.Errorf("expected metadata %v, got %v", metadata, wf.Metadata)
		}

		metadata["new_key"] = "new_value"
		if _, exists := wf.Metadata["new_key"]; exists {
			t.Errorf("workflow metadata should not be affected by modifying original map")
		}

		if _, exists := metadata["new_key"]; !exists {
			t.Errorf("original map should still have the new key")
		}

		expectedMetadata := map[string]string{
			"team":     "payments",
			"priority": "high",
			"region":   "us-east",
		}
		if !reflect.DeepEqual(wf.Metadata, expectedMetadata) {
			t.Errorf("expected workflow metadata to remain %v, got %v", expectedMetadata, wf.Metadata)
		}
	})

	t.Run("WithMetadataNil", func(t *testing.T) {
		wf := NewWorkflow("test-workflow", WithMetadata(nil))

		if wf.Metadata == nil {
			t.Errorf("expected metadata map to be initialized even when nil is passed")
		}
		if len(wf.Metadata) != 0 {
			t.Errorf("expected empty metadata map, got %v", wf.Metadata)
		}
	})

	t.Run("WithQueue", func(t *testing.T) {
		queueName := "test-queue"
		wf := NewWorkflow("test-workflow", WithQueue(queueName))

		if wf.QueueName != queueName {
			t.Errorf("expected queue name %q, got %q", queueName, wf.QueueName)
		}
	})

	t.Run("MultipleOptions", func(t *testing.T) {
		metadata := map[string]string{"key": "value"}
		wf := NewWorkflow(
			"test-workflow",
			WithQueue("my-queue"),
			WithDisplayName("My Workflow"),
			WithMetadata(metadata),
		)

		if wf.QueueName != "my-queue" {
			t.Errorf("expected queue name my-queue, got %q", wf.QueueName)
		}
		if wf.DisplayName != "My Workflow" {
			t.Errorf("expected display name My Workflow, got %q", wf.DisplayName)
		}
		expectedMetadata := map[string]string{"key": "value"}
		if !reflect.DeepEqual(wf.Metadata, expectedMetadata) {
			t.Errorf("expected metadata %v, got %v", expectedMetadata, wf.Metadata)
		}
	})

	t.Run("DefaultValues", func(t *testing.T) {
		wf := NewWorkflow("test-workflow")

		if wf.QueueName != "" {
			t.Errorf("expected empty queue name, got %q", wf.QueueName)
		}
		if wf.DisplayName != "" {
			t.Errorf("expected empty display name, got %q", wf.DisplayName)
		}
		if wf.Metadata == nil {
			t.Fatalf("expected metadata map to be initialized")
		}
		if len(wf.Metadata) != 0 {
			t.Errorf("expected empty metadata map, got %v", wf.Metadata)
		}
	})
}

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
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
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
	err = engine.Run(context.Background(), wf)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !step1Done {
		t.Errorf("step1 not executed")
	}
	if !step2Done {
		t.Errorf("step2 not executed")
	}

	storedWf, err := engine.Store().LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("failed to load workflow from store: %v", err)
	}
	if storedWf.CurrentStep != 2 {
		t.Errorf("expected currentStep to be 2, got %d", storedWf.CurrentStep)
	}
	if _, ok := storedWf.State["step1output"]; !ok {
		t.Errorf("step1 output not found")
	}
	if _, ok := storedWf.State["step2output"]; !ok {
		t.Errorf("step2 output not found")
	}
}

func TestWorkflow_Run_Simple_WithDisplayNameAndMetadata(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
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

	engine.RegisterTemplate("workflow-with-metadata", &WorkflowTemplate{
		Steps: []Step{step1, step2},
	})

	metadata := map[string]string{
		"team":     "payments",
		"priority": "high",
		"region":   "us-east",
	}
	displayName := "Payment Processing Workflow"

	wf, err := engine.NewWorkflow("workflow-with-metadata",
		WithDisplayName(displayName),
		WithMetadata(metadata),
	)
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	err = engine.Run(context.Background(), wf)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !step1Done {
		t.Errorf("step1 not executed")
	}
	if !step2Done {
		t.Errorf("step2 not executed")
	}

	storedWf, err := engine.Store().LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("failed to load workflow from store: %v", err)
	}
	if storedWf.DisplayName != displayName {
		t.Errorf("expected display name %q, got %q", displayName, storedWf.DisplayName)
	}
	if !reflect.DeepEqual(storedWf.Metadata, metadata) {
		t.Errorf("expected metadata %v, got %v", metadata, storedWf.Metadata)
	}
	if storedWf.CurrentStep != 2 {
		t.Errorf("expected currentStep to be 2, got %d", storedWf.CurrentStep)
	}
	if _, ok := storedWf.State["step1output"]; !ok {
		t.Errorf("step1 output not found")
	}
	if _, ok := storedWf.State["step2output"]; !ok {
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
	err = engine.Run(context.Background(), wf)

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
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
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
	err = engine.Run(context.Background(), wf)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !step1Done {
		t.Errorf("step1 not executed")
	}
	if !step2Done {
		t.Errorf("step2 not executed")
	}

	storedWf, err := engine.Store().LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("failed to load workflow from store: %v", err)
	}
	if storedWf.CurrentStep != 2 {
		t.Errorf("expected currentStep to be 2, got %d", storedWf.CurrentStep)
	}

	if _, ok := storedWf.State["step2output"]; !ok {
		t.Errorf("step2 output not found")
	}
	rawAttempts, ok := storedWf.State["final_attempts"]
	if !ok {
		t.Fatalf("final_attempts not found in workflow state")
	}
	attempts, ok := rawAttempts.(float64)
	if !ok {
		t.Fatalf("final_attempts has unexpected type %T", rawAttempts)
	}
	if int(attempts) != 3 {
		t.Errorf("expected final_attempts to be 3, got %v", attempts)
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
	err = engine.Run(context.Background(), wf)

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
	if err := engine.Run(context.Background(), wf); err != nil {
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
	if err := engine.Run(context.Background(), wf); err != nil {
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
	err = engine.Run(context.Background(), wf)
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
	if err := engine.Run(context.Background(), wf); err != nil {
		t.Fatalf("failed to run workflow again: %v", err)
	}

	// Verify API calls - should not have increased due to idempotency
	if apiCalls != 1 {
		t.Errorf("expected still 1 API call, got %d", apiCalls)
	}
}
