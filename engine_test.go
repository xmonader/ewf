package ewf

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestEngine_Rehydration_FromStore guarantees workflows are rehydrated from persistent store, not memory.
//
// Scenario:
// 1. Removes any existing SQLite DB file ("test_rehydration.db") to start fresh.
// 2. Creates a store and engine, registers two activities and a template.
// 3. Instantiates a workflow and manually runs the first step, updating state and step index.
// 4. Persists the workflow to the DB.
// 5. Simulates a process restart by creating a new engine and re-registering activities.
// 6. Loads the workflow from the DB and resumes execution.
// 7. Verifies the workflow resumes from the correct step, state is preserved, and all steps complete as expected.
func TestEngine_Rehydration_FromStore(t *testing.T) {
	const dbFile = "test_rehydration.db"
	if err := os.Remove(dbFile); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove db: %v", err)
	}
	store, err := NewSQLiteStore(dbFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Logf("Failed to close store: %v", err)
		}
		if err := os.Remove(dbFile); err != nil {
			t.Logf("Failed to remove db file: %v", err)
		}
	})
	if err := store.Setup(); err != nil {
		t.Fatalf("failed to setup store: %v", err)
	}

	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

	var step1Done, step2Done bool
	engine.Register("step1", func(ctx context.Context, state State) error {
		step1Done = true
		state["step1output"] = "output from step1"
		return nil
	})
	engine.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		state["step2output"] = "output from step2"
		return nil
	})

	engine.RegisterTemplate("rehydration-workflow", &WorkflowTemplate{
		Steps: []Step{{Name: "step1"}, {Name: "step2"}},
	})

	// Create and run workflow until after step1
	wf, err := engine.NewWorkflow("rehydration-workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	// Manually run the first step function to simulate partial progress
	if err := engine.activities["step1"](context.Background(), wf.State); err != nil {
		t.Fatalf("failed to run step1: %v", err)
	}
	step1Done = true
	wf.CurrentStep = 1
	if !step1Done {
		t.Errorf("step1 not executed")
	}
	if wf.CurrentStep != 1 {
		t.Errorf("expected currentStep to be 1 after step1, got %d", wf.CurrentStep)
	}
	// Save workflow
	if err := store.SaveWorkflow(context.Background(), wf); err != nil {
		t.Fatalf("failed to persist workflow: %v", err)
	}

	// Simulate process restart: create new engine, reload workflow from DB
	engine2, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine2: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})
	// Re-register activities after restart
	engine2.Register("step1", func(ctx context.Context, state State) error {
		step1Done = true
		state["step1output"] = "output from step1"
		return nil
	})
	engine2.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		state["step2output"] = "output from step2"
		return nil
	})
	step1Done, step2Done = false, false // reset flags to ensure only step2 runs
	wf2, err := engine2.Store().LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("failed to reload workflow: %v", err)
	}
	if wf2.CurrentStep != 1 {
		t.Fatalf("expected rehydrated workflow to be at step 1, got %d", wf2.CurrentStep)
	}

	// Resume workflow from rehydrated state
	if err := engine2.Run(context.Background(), wf2); err != nil {
		t.Errorf("unexpected error running rehydrated workflow: %v", err)
	}
	if !step2Done {
		t.Errorf("step2 not executed after rehydration")
	}
	if wf2.CurrentStep != 2 {
		t.Errorf("expected currentStep to be 2 after completion, got %d", wf2.CurrentStep)
	}
	if _, ok := wf2.State["step1output"]; !ok {
		t.Errorf("rehydrated workflow missing step1 output")
	}
	if _, ok := wf2.State["step2output"]; !ok {
		t.Errorf("rehydrated workflow missing step2 output")
	}
}

// TestEngine_DynamicTemplatePersistenceAndRecovery tests that dynamically created templates are persisted and can be recovered.
//
// Scenario:
// 1. Removes any existing SQLite DB file ("e2e_dyn_template.db") to start fresh.
// 2. Creates a store and engine.
// 3. Dynamically creates a template with 5 steps and registers it.
// 4. Simulates a process restart by creating a new engine instance.
// 5. Verifies the new engine can create a workflow from the persisted template.
// 6. Verifies the workflow has the correct number of steps.
// 7. Cleans up by closing the store and removing the DB file.
func TestEngine_DynamicTemplatePersistenceAndRecovery(t *testing.T) {
	if err := os.Remove("e2e_dyn_template.db"); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove db: %v", err)
	}
	store, err := NewSQLiteStore("e2e_dyn_template.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}

		if err := os.Remove("e2e_dyn_template.db"); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove db: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

	// Simulate dynamic creation
	name := "deployk8s-5"
	tmpl := &WorkflowTemplate{Steps: []Step{}}
	for i := 0; i < 5; i++ {
		tmpl.Steps = append(tmpl.Steps, Step{Name: "deploy-node"})
	}
	engine.RegisterTemplate(name, tmpl)

	// Simulate crash: create new engine instance
	engine2, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine2: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})
	// Should have template loaded
	wf, err := engine2.NewWorkflow(name)
	if err != nil {
		t.Fatalf("engine2 failed to create workflow from persisted template: %v", err)
	}
	if len(wf.Steps) != 5 {
		t.Fatalf("engine2 loaded workflow has wrong step count: %d", len(wf.Steps))
	}
}

// TestEngine_HookInvocationCounts tests that hooks are called exactly once per run.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a template with all types of hooks (before/after workflow, before/after step).
// 3. Registers a step activity.
// 4. Creates and runs a workflow.
// 5. Verifies each hook is called exactly once.
// 6. Resets the workflow and runs it again.
// 7. Verifies each hook is called exactly once more (total 2 times).
func TestEngine_HookInvocationCounts(t *testing.T) {
	var beforeWorkflowCalls, afterWorkflowCalls int
	var beforeStepCalls, afterStepCalls int

	beforeWorkflow := func(ctx context.Context, w *Workflow) { beforeWorkflowCalls++ }
	afterWorkflow := func(ctx context.Context, w *Workflow, err error) { afterWorkflowCalls++ }
	beforeStep := func(ctx context.Context, w *Workflow, step *Step) { beforeStepCalls++ }
	afterStep := func(ctx context.Context, w *Workflow, step *Step, err error) { afterStepCalls++ }

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
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

	tmpl := &WorkflowTemplate{
		Steps:               []Step{{Name: "step1"}},
		BeforeWorkflowHooks: []BeforeWorkflowHook{beforeWorkflow},
		AfterWorkflowHooks:  []AfterWorkflowHook{afterWorkflow},
		BeforeStepHooks:     []BeforeStepHook{beforeStep},
		AfterStepHooks:      []AfterStepHook{afterStep},
	}
	engine.RegisterTemplate("test", tmpl)

	// Register dummy activity for step1
	engine.Register("step1", func(ctx context.Context, state State) error { return nil })

	wf, err := engine.NewWorkflow("test")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	if err := engine.Run(context.Background(), wf); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if beforeWorkflowCalls != 1 {
		t.Errorf("beforeWorkflow hook called %d times, expected 1", beforeWorkflowCalls)
	}
	if afterWorkflowCalls != 1 {
		t.Errorf("afterWorkflow hook called %d times, expected 1", afterWorkflowCalls)
	}
	if beforeStepCalls != 1 {
		t.Errorf("beforeStep hook called %d times, expected 1", beforeStepCalls)
	}
	if afterStepCalls != 1 {
		t.Errorf("afterStep hook called %d times, expected 1", afterStepCalls)
	}

	// Reset workflow to rerun steps
	wf.Status = StatusPending
	wf.CurrentStep = 0

	// Run again: counters should increment by 1 each
	if err := engine.Run(context.Background(), wf); err != nil {
		t.Fatalf("Run (second) failed: %v", err)
	}
	if beforeWorkflowCalls != 2 {
		t.Errorf("beforeWorkflow hook called %d times after second run, expected 2", beforeWorkflowCalls)
	}
	if afterWorkflowCalls != 2 {
		t.Errorf("afterWorkflow hook called %d times after second run, expected 2", afterWorkflowCalls)
	}
	if beforeStepCalls != 2 {
		t.Errorf("beforeStep hook called %d times after second run, expected 2", beforeStepCalls)
	}
	if afterStepCalls != 2 {
		t.Errorf("afterStep hook called %d times after second run, expected 2", afterStepCalls)
	}
}

// TestEngine_FailFastErrorBypassesRetries tests that ErrFailWorkflowNow causes workflow to fail immediately.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a step that returns ErrFailWorkflowNow.
// 3. Registers a template with the step.
// 4. Creates and runs a workflow.
// 5. Verifies the workflow fails with ErrFailWorkflowNow.
// 6. Verifies the step is called exactly once (no retries).
func TestEngine_FailFastErrorBypassesRetries(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
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
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

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
	err = engine.Run(context.Background(), wf)
	if err == nil || !strings.Contains(err.Error(), ErrFailWorkflowNow.Error()) {
		t.Fatalf("expected workflow to fail with ErrFailWorkflowNow, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected step to be called once, got %d", calls)
	}
}

// TestEngine_WrappedFailFastErrorBypassesRetries tests that a wrapped ErrFailWorkflowNow is still detected.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a step that returns a wrapped ErrFailWorkflowNow.
// 3. Registers a template with the step.
// 4. Creates and runs a workflow.
// 5. Verifies the workflow fails with ErrFailWorkflowNow.
// 6. Verifies the step is called exactly once (no retries).
func TestEngine_WrappedFailFastErrorBypassesRetries(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
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
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

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
	err = engine.Run(context.Background(), wf)
	if err == nil || !strings.Contains(err.Error(), "wrapped: fail workflow now") {
		t.Fatalf("expected workflow to fail with wrapped ErrFailWorkflowNow, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected step to be called once, got %d", calls)
	}
}

// TestEngine_NormalRetryPolicyStillWorks tests that normal retry policy works as expected.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a step that always fails with context.DeadlineExceeded.
// 3. Registers a template with the step and a retry policy (max 2 attempts).
// 4. Creates and runs a workflow.
// 5. Verifies the workflow fails.
// 6. Verifies the step is called exactly twice (original + 1 retry).
func TestEngine_NormalRetryPolicyStillWorks(t *testing.T) {
	calls := 0
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite store: %v", err)
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
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

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
	err = engine.Run(context.Background(), wf)
	if err == nil {
		t.Fatal("expected workflow to fail, got nil")
	}
	if calls != 2 {
		t.Fatalf("expected step to be called 2 times (for retries), got %d", calls)
	}
}

// TestEngine_StepPanic verifies that panics in steps are caught and treated as errors.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a step that always panics.
// 3. Registers a template with the step and a retry policy (max 2 attempts).
// 4. Creates and runs a workflow.
// 5. Verifies the workflow fails with a panic error.
// 6. Verifies the step is called exactly twice (original + 1 retry).
func TestEngine_StepPanic(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

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
				BackOff:     ConstantBackoff(10 * time.Millisecond),
			},
		}},
	}
	engine.RegisterTemplate("panic_workflow", tmpl)

	wf, err := engine.NewWorkflow("panic_workflow")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}

	err = engine.Run(context.Background(), wf)
	if err == nil {
		t.Fatalf("expected workflow to fail due to panic, but it succeeded")
	}
	if !strings.Contains(err.Error(), "panic in step 'PanickyStep': something went wrong!") {
		t.Errorf("expected panic error, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts due to retry policy, got %d", attempts)
	}
}

// TestEngine_StepTimeout verifies that steps respect their timeout setting.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a step that takes longer than its timeout.
// 3. Registers a template with the step and a short timeout.
// 4. Creates and runs a workflow.
// 5. Verifies the workflow fails with a timeout error.
// 6. Verifies the step was interrupted by the timeout.
func TestEngine_StepTimeout(t *testing.T) {
	// Create a test store
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	// Create an engine with the store
	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

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
	err = engine.Run(context.Background(), wf)

	// Verify the workflow failed due to timeout
	if err == nil {
		t.Fatalf("expected workflow to fail with timeout error, but it succeeded")
	}

	if !strings.Contains(err.Error(), "step 'SlowStep' timed out after 100ms") {
		t.Fatalf("expected timeout error, got: %v", err)
	}

	if !slowStepInterrupted {
		t.Fatalf("step was not interrupted by timeout")
	}
}

// TestEngine_StepTimeoutWithRetry verifies that step timeout applies to each retry attempt.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a step that always times out.
// 3. Registers a template with the step, a short timeout, and a retry policy.
// 4. Creates and runs a workflow.
// 5. Verifies the workflow fails with a timeout error.
// 6. Verifies all retry attempts were made.
func TestEngine_StepTimeoutWithRetry(t *testing.T) {
	// Create a test store
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	// Create an engine with the store
	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

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
					MaxAttempts: 2,
					BackOff:     ConstantBackoff(10 * time.Millisecond),
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
	err = engine.Run(context.Background(), wf)

	// Verify the workflow failed due to timeout after retries
	if err == nil {
		t.Fatalf("expected workflow to fail with timeout error, but it succeeded")
	}

	if !strings.Contains(err.Error(), "step 'RetryStep' timed out after 50ms") {
		t.Fatalf("expected timeout error, got: %v", err)
	}

	// Verify all retry attempts were made
	if attempts != 2 {
		t.Fatalf("expected 2 retry attempts, got %d", attempts)
	}
}

// TestEngine_ResumeWorkflows_ResumesRunningWorkflows tests that ResumeWorkflows resumes workflows with running status.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a template and activity.
// 3. Creates a workflow and manually sets it to running status with partial progress.
// 4. Saves the workflow to the store.
// 5. Creates a new engine instance (simulating restart).
// 6. Calls ResumeWorkflows.
// 7. Waits for workflow completion.
// 8. Verifies the workflow completed successfully.
func TestEngine_ResumeWorkflows_ResumesRunningWorkflows(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

	var step2Done bool
	engine.Register("step1", func(ctx context.Context, state State) error {
		return nil
	})
	engine.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		return nil
	})

	engine.RegisterTemplate("resume-test", &WorkflowTemplate{
		Steps: []Step{{Name: "step1"}, {Name: "step2"}},
	})

	// Create workflow and manually set to running with partial progress
	wf, err := engine.NewWorkflow("resume-test")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	wf.Status = StatusRunning
	wf.CurrentStep = 1 // Completed step1, need to run step2

	if err := store.SaveWorkflow(context.Background(), wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Create new engine instance (simulating restart)
	engine2, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine2: %v", err)
	}
	t.Cleanup(func() {
		if err := engine2.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine2: %v", err)
		}
	})

	engine2.Register("step1", func(ctx context.Context, state State) error {
		return nil
	})
	engine2.Register("step2", func(ctx context.Context, state State) error {
		step2Done = true
		return nil
	})

	// Reset flag
	step2Done = false

	// Resume workflows
	engine2.ResumeWorkflows()

	// Wait for workflow to complete
	time.Sleep(500 * time.Millisecond)

	// Verify workflow completed
	if !step2Done {
		t.Errorf("expected step2 to be executed after resume")
	}

	// Verify workflow status is completed
	wf2, err := store.LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	if wf2.Status != StatusCompleted {
		t.Errorf("expected workflow status to be completed, got %s", wf2.Status)
	}
}

// TestEngine_ResumeWorkflows_ResumesPendingNonQueued tests that ResumeWorkflows resumes pending workflows that are not queued.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a template and activity.
// 3. Creates a pending workflow without a queue name.
// 4. Saves the workflow to the store.
// 5. Creates a new engine instance (simulating restart).
// 6. Calls ResumeWorkflows.
// 7. Waits for workflow completion.
// 8. Verifies the workflow completed successfully.
func TestEngine_ResumeWorkflows_ResumesPendingNonQueued(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

	var stepExecuted bool
	engine.Register("step1", func(ctx context.Context, state State) error {
		stepExecuted = true
		return nil
	})

	engine.RegisterTemplate("pending-test", &WorkflowTemplate{
		Steps: []Step{{Name: "step1"}},
	})

	// Create pending workflow without queue name
	wf, err := engine.NewWorkflow("pending-test")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	wf.Status = StatusPending
	wf.QueueName = "" // Not queued

	if err := store.SaveWorkflow(context.Background(), wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Create new engine instance (simulating restart)
	engine2, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine2: %v", err)
	}
	t.Cleanup(func() {
		if err := engine2.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine2: %v", err)
		}
	})

	engine2.Register("step1", func(ctx context.Context, state State) error {
		stepExecuted = true
		return nil
	})

	// Reset flag
	stepExecuted = false

	// Resume workflows
	engine2.ResumeWorkflows()

	// Wait for workflow to complete
	time.Sleep(500 * time.Millisecond)

	// Verify workflow completed
	if !stepExecuted {
		t.Errorf("expected step1 to be executed after resume")
	}

	// Verify workflow status is completed
	wf2, err := store.LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	if wf2.Status != StatusCompleted {
		t.Errorf("expected workflow status to be completed, got %s", wf2.Status)
	}
}

// TestEngine_ResumeWorkflows_SkipsPendingQueued tests that ResumeWorkflows skips pending workflows that are queued.
//
// Scenario:
// 1. Creates a store and engine.
// 2. Registers a template and activity.
// 3. Creates a pending workflow with a queue name.
// 4. Saves the workflow to the store.
// 5. Creates a new engine instance (simulating restart).
// 6. Calls ResumeWorkflows.
// 7. Verifies the workflow was not resumed (remains pending).
func TestEngine_ResumeWorkflows_SkipsPendingQueued(t *testing.T) {
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	engine, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine: %v", err)
		}
	})

	var stepExecuted bool
	engine.Register("step1", func(ctx context.Context, state State) error {
		stepExecuted = true
		return nil
	})

	engine.RegisterTemplate("queued-test", &WorkflowTemplate{
		Steps: []Step{{Name: "step1"}},
	})

	// Create pending workflow with queue name
	wf, err := engine.NewWorkflow("queued-test")
	if err != nil {
		t.Fatalf("failed to create workflow: %v", err)
	}
	wf.Status = StatusPending
	wf.QueueName = "test-queue" // Queued

	if err := store.SaveWorkflow(context.Background(), wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Create new engine instance (simulating restart)
	engine2, err := NewEngine(WithStore(store))
	if err != nil {
		t.Fatalf("failed to create engine2: %v", err)
	}
	t.Cleanup(func() {
		if err := engine2.Close(context.Background()); err != nil {
			t.Fatalf("failed to close engine2: %v", err)
		}
	})

	engine2.Register("step1", func(ctx context.Context, state State) error {
		stepExecuted = true
		return nil
	})

	// Reset flag
	stepExecuted = false

	// Resume workflows
	engine2.ResumeWorkflows()

	// Wait a bit to ensure workflow is not processed
	time.Sleep(500 * time.Millisecond)

	// Verify workflow was NOT executed (should be skipped)
	if stepExecuted {
		t.Errorf("expected step1 NOT to be executed for queued workflow")
	}

	// Verify workflow status remains pending
	wf2, err := store.LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	if wf2.Status != StatusPending {
		t.Errorf("expected workflow status to remain pending, got %s", wf2.Status)
	}
	if wf2.QueueName != "test-queue" {
		t.Errorf("expected queue name to be preserved, got %s", wf2.QueueName)
	}
}
