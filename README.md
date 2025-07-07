# EWF - Embeddable Workflow Engine for Go

EWF is a simple, lightweight, and embeddable workflow engine for Go applications. It allows you to define stateful, multi-step processes that are resilient to application crashes.

## Core Features

*   **Stateful Workflows**: Each workflow maintains its own state, which can be passed between steps.
*   **Resilience & Resumption**: Workflow state can be persisted after each step using a `Store`, allowing you to resume an interrupted workflow from exactly where it left off.
*   **Pluggable Storage**: Comes with a built-in `SQLiteStore`, but you can implement the `Store` interface to use any key-value backend.
*   **Lifecycle Hooks**: Execute custom logic before or after a workflow or a specific step.
*   **Automatic Retries**: Define simple retry policies for steps that might fail intermittently.

## Installation

```sh
go get github.com/xmonader/ewf
```

## Basic Usage

Here's a simple example of a two-step workflow. In a real application, you would add a `Store` to enable persistence and resumption.

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/xmonader/ewf"
)

// A simple step that prints a message and waits.
func printAndWait(message string, duration time.Duration) ewf.StepFn {
	return func(ctx context.Context, state ewf.State) error {
		log.Printf("Step: %s - Started", message)
		// You can read/write to the state map here
		// state["my_key"] = "my_value"
		time.Sleep(duration)
		log.Printf("Step: %s - Finished", message)
		return nil
	}
}

func main() {
	// 1. Define the steps for the workflow
	steps := []ewf.Step{
		{Name: "First Step", Fn: printAndWait("Hello from Step 1", 5*time.Second)},
		{Name: "Second Step", Fn: printAndWait("Hello from Step 2", 5*time.Second)},
	}

	// 2. (Optional) Set up a store for persistence
	// For this simple example, we won't persist, but in a real app you would:
	// store, err := ewf.NewSQLiteStore("my_workflow.db")
	// if err != nil { log.Fatalf("store error: %v", err) }
	// defer store.Close()
	// store.Setup()

	// 3. Create a new workflow
	wf := ewf.NewWorkflow(
		"my-first-workflow",
		ewf.WithSteps(steps...),
		// ewf.WithStore(store), // You would add this for persistence
	)

	// 4. Run the workflow
	log.Println("Starting workflow...")
	if err := wf.Run(context.Background()); err != nil {
		log.Fatalf("Workflow failed: %v", err)
	}

	log.Println("Workflow completed successfully!")
}
```

## Running Tests

To run the test suite for the library:

```sh
go test -v ./...
```

Or using the Makefile:

```sh
make test
```
