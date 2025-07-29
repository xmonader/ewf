// Package main provides a CLI example for using the ewf workflow engine.
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
	store, err := ewf.NewSQLiteStore(dbFile)
	if err != nil {
		log.Fatalf("Failed to create sqlite store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("failed to close store: %v", err)
		}
	}()

	engine, err := ewf.NewEngine(store)
	if err != nil {
		log.Fatalf("Failed to create engine: %v", err)
		return
	}

	engine.Register("wait_5_seconds", waitStep(5*time.Second))
	engine.Register("wait_10_seconds", waitStep(10*time.Second))
	engine.Register("wait_15_seconds", waitStep(15*time.Second))

	def := &ewf.WorkflowTemplate{
		Steps: []ewf.Step{
			{Name: "wait_5_seconds"},
			{Name: "wait_10_seconds"},
			{Name: "wait_15_seconds"},
		},
		BeforeStepHooks: []ewf.BeforeStepHook{beforeStepHook},
	}

	engine.RegisterTemplate("long-timer", def)

	ctx := context.Background()
	// Load the pending workflow from the store
	uuids, _ := store.ListWorkflowUUIDsByStatus(ctx, ewf.StatusRunning)
	if len(uuids) == 0 {
		println("NEW")
		wf, err := engine.NewWorkflow("long-timer")
		if err != nil {
			log.Fatalf("Failed to create workflow: %v", err)
		}

		if err := engine.RunSync(ctx, wf); err != nil {
			log.Fatalf("Workflow failed: %v", err)
		}
		log.Println("workflow completed successfully!")
		return

	}
	println("RESUMING")
	for _, id := range uuids {
		wf, err := store.LoadWorkflowByUUID(ctx, id)
		if err != nil {
			log.Printf("Failed to load workflow '%s'. Was it ever started? Error: %v\n", workflowName, err)
			continue
		}

		if wf.Status == ewf.StatusCompleted {
			log.Println("Workflow was already completed. Nothing to do, delete the DB file.")
			continue
		}
		if err := engine.RunSync(ctx, wf); err != nil {
			log.Fatalf("Workflow failed: %v", err)
		}
		log.Println("workflow completed successfully!")

	}

}
