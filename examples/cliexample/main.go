package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xmonader/ewf"
)

const (
	dbFile       = "./cli_demo.db"
	workflowName = "long_timer_workflow"
)

// createTimerWorkflow centralizes the definition of our workflow steps.
func createTimerWorkflow(name string, store *ewf.SQLiteStore) *ewf.Workflow {
	timerSteps := []ewf.Step{
		{Name: "wait_5_seconds", Fn: waitStep(5 * time.Second)},
		{Name: "wait_10_seconds", Fn: waitStep(10 * time.Second)},
		{Name: "wait_15_seconds", Fn: waitStep(15 * time.Second)},
	}

	return ewf.NewWorkflow(
		name,
		ewf.WithStore(store),
		ewf.WithSteps(timerSteps...),
	)
}

// waitStep is a function that returns a StepFn.
// This allows us to create steps with different durations easily.
func waitStep(duration time.Duration) ewf.StepFn {
	return func(ctx context.Context, state ewf.State) error {
		stepName, _ := state["current_step_name"].(string)
		log.Printf("--- Running step: '%s'. Waiting for %s ---", stepName, duration)
		time.Sleep(duration)
		log.Printf("--- Step '%s' finished. ---", stepName)
		return nil
	}
}

// beforeStepHook is a hook that runs before each step to log its name.
// We store the step name in the state so our waitStep function can access it.
func beforeStepHook(ctx context.Context, w *ewf.Workflow, step *ewf.Step) {
	w.State["current_step_name"] = step.Name
	fmt.Println("Executing step:", w.Steps[w.CurrentStep].Name)
}

func main() {
	// Setup the SQLite store
	store, err := ewf.NewSQLiteStore(dbFile)
	if err != nil {
		log.Fatalf("Failed to create sqlite store: %v", err)
	}
	defer store.Close()

	if err := store.Prepare(); err != nil {
		log.Fatalf("Failed to prepare database: %v", err)
	}

	ctx := context.Background()
	log.Println("Attempting to resume workflow if any...")
	// Load the workflow from the store
	loadedWf, err := store.LoadWorkflow(ctx, workflowName)
	if err != nil {

		log.Printf("Failed to load workflow '%s'. Was it ever started? Error: %v\n", workflowName, err)
	}
	wf := createTimerWorkflow(workflowName, store)
	if err == nil {
		wf.CreatedAt = loadedWf.CreatedAt
		wf.CurrentStep = loadedWf.CurrentStep
	}
	wf.SetBeforeStepHooks(beforeStepHook)

	if wf.Status == ewf.StatusCompleted {
		log.Println("Workflow was already completed. Nothing to do, delete the DB file.")
		return
	}
	if err := wf.Run(ctx); err != nil {
		log.Fatalf("Workflow failed: %v", err)
	}
	log.Println("workflow completed successfully!")

}
