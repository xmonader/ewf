package ewf

import (
	"context"
	"fmt"
	"testing"
)

// TestSimpleIdempotencyPattern demonstrates a simple idempotency pattern
// using the step name from context and flags in the workflow state.
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
	engine, err := NewEngine(store)
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
	engine, err := NewEngine(store)
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
