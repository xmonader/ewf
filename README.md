# EWF - Embeddable Workflow Framework for Go

EWF is a simple, lightweight, and embeddable workflow framework for Go applications. It allows you to define stateful, multi-step processes that are resilient to application crashes and interruptions.

## Core Features

- **Centralized Engine**: A powerful `Engine` manages workflow definitions, activities, and execution.
- **Queue-Based Execution** _(New)_: Integrates a lightweight queue for simultaneous and concurrent workflow processing.
- **Stateful & Resilient Workflows**: Each workflow maintains its own state, which is persisted after each step to a `Store`.
- **Automatic Resumption**: The engine automatically finds and resumes interrupted workflows on startup, ensuring no work is lost.
- **Asynchronous Execution**: Run workflows in the background using `Run` with `WithAsync()` option, perfect for use in HTTP servers and other concurrent applications.
- **Pluggable Storage**: Comes with a built-in `SQLiteStore`, but you can implement the `Store` interface to use any key-value backend.
- **Context-Aware Retries**: Define robust retry policies for steps that might fail, with delays that respect context cancellation to prevent resource leaks.
- **Lifecycle Hooks**: Execute custom logic before or after a workflow or a specific step.

## Feature Matrix

| Feature                      | Supported | Notes                                                                                     |
| ---------------------------- | :-------: | --------------------------------------------------------------------------------------    |
| Step Retry Policies          |    ✅     | Per-step, with customizable attempts and flexible backoff (constant, exponential, etc)    |
| Step Timeouts                |    ✅     | Per-step, context-based cancellation                                                      |
| Idempotency Helpers/Patterns |    ✅     | Ergonomic, context-based, with docs/examples                                              |
| Before/After Workflow Hooks  |    ✅     | For setup, teardown, logging, etc.                                                        |
| Before/After Step Hooks      |    ✅     | For auditing, metrics, etc.                                                               |
| State Persistence            |    ✅     | SQLite built-in; pluggable interface                                                      |
| Workflow Resumption          |    ✅     | Survives crashes/restarts                                                                 |
| Asynchronous Execution       |    ✅     | Run workflows in background                                                               |
| Synchronous Execution        |    ✅     | For tests and CLI                                                                         |
| Pluggable Storage            |    ✅     | Implement your own Store                                                                  |
| Queue Engine                 |    ✅     | Provides an interface for concurrent background processing for queued tasks and workflows |
| Queue                        |    ✅     | Built-in interface; supports enqueue, dequeue, and worker pool management                 |
| CLI/HTTP Example Workflows   |    ✅     | See `examples/` directory                                                                 |
| Context Propagation          |    ✅     | Step context carries deadlines, values                                                    |
| Step Metadata in Context     |    ✅     | Step name injected for idempotency                                                        |
| Testing Support              |    ✅     | Unit, integration, E2E patterns                                                           |
| GoDoc & User Guide           |    ✅     | See `docs/userguide.md`                                                                   |

## Installation

```sh
go get github.com/xmonader/ewf
```

## Concepts

- **Engine**: The central hub of the framework. It holds registered `Activity` functions and `WorkflowTemplate` definitions. It's responsible for creating and running workflows.
- **Activity**: A simple Go function (`StepFn`) that represents a single unit of work. Activities are registered with the engine by a unique name.
- **WorkflowTemplate**: A blueprint for a workflow, defining the sequence of activities (steps) to be executed.
- **Workflow**: A running instance of a `WorkflowTemplate`. Each workflow has a unique ID, its own state, and tracks its progress through the steps.
- **Store**: A persistence layer (e.g., `SQLiteStore`) that saves and loads workflow state and queue metadata, enabling resilience.
- **Queue**: A concurrent-safe structure that holds pending tasks or workflow jobs.
- **QueueEngine**: It acts as a scheduler and execution manager for queued jobs, ensuring:
  - Automatic worker startup when a queue is created.
  - Graceful shutdowns respecting context cancellation.
  - Optional persistence layer integration for durable queues.

## Basic Usage

Here's a simple example of a two-step workflow using the modern, engine-centric approach.

```go
package main

import (
 "context"
 "log"
 "time"

 "github.com/xmonader/ewf"
)

// An activity that waits for a given duration.
func waitActivity(duration time.Duration) ewf.StepFn {
 return func(ctx context.Context, state ewf.State) error {
  log.Printf("Waiting for %s...", duration)
  time.Sleep(duration)
  return nil
 }
}

func main() {
 // 1. Set up a store for persistence.
 store, err := ewf.NewSQLiteStore("cli_example.db")
 if err != nil {
  log.Fatalf("store error: %v", err)
 }
 defer store.Close()

// 2. Create a new engine.
engine, err := ewf.NewEngine(ewf.WithStore(store))
 if err != nil {
  log.Fatalf("engine error: %v", err)
 }

 // 3. Register your activities (the building blocks of workflows).
 engine.Register("wait_5s", waitActivity(5*time.Second))
 engine.Register("wait_10s", waitActivity(10*time.Second))

 // 4. Define and register a workflow template.
 myWorkflow := &ewf.WorkflowTemplate{
  Steps: []ewf.Step{
   {
    Name: "wait_5s",
    RetryPolicy: &ewf.RetryPolicy{
     MaxAttempts: 3,
     BackOff:     ewf.ConstantBackoff(2 * time.Second),
    },
   },
   {
    Name: "wait_10s",
    RetryPolicy: &ewf.RetryPolicy{
     MaxAttempts: 5,
     BackOff:     ewf.ExponentialBackoff(500*time.Millisecond, 10*time.Second, 2.0),
    },
   },
  },
 }
 engine.RegisterTemplate("my_workflow", myWorkflow)

 // 5. Create a new workflow instance from the template.
 wf, err := engine.NewWorkflow("my_workflow")
 if err != nil {
  log.Fatalf("failed to create workflow: %v", err)
 }

// 6. Run the workflow synchronously.
log.Println("Starting workflow...")
if err := engine.Run(context.Background(), wf); err != nil {
 log.Fatalf("Workflow failed: %v", err)
}

// Reload from the store to inspect the most recent workflow state.
latest, err := store.LoadWorkflowByUUID(context.Background(), wf.UUID)
if err != nil {
 log.Fatalf("failed to load workflow: %v", err)
}
log.Printf("Workflow completed successfully at step %d!\n", latest.CurrentStep)
}
```

This example shows the usage of the `Queue Engine`:

```go
// first, implement QueueEngine, Queue Interface

 wfengine, err := NewEngine(WithQueueEngine(qEngine))
 if err != nil {
  log.Fatalf("wf engine error: %v", err)
 }
 defer func() {
  if err := wfengine.Close(context); err != nil {
   log.Fatalf("failed to close engine: %v", err)
  }
 }

    // queue with custom options
 err = wfengine.CreateQueue(
  context,
  name,
  WorkersDefinition{
   Count:        1,
   PollInterval: 300 * time.Millisecond,
  },
  QueueOptions{
   AutoDelete:  true,
   DeleteAfter: 2 * time.Second,
   PopTimeout:  1 * time.Second,
  },
 )
 if err != nil {
  log.Fatalf("failed to create queue: %v", err)
 }

 workflow, err := wfengine.NewWorkflow(wfName, WithQueue(name))
 if err != nil {
  log.Fatalf("failed to create workflow: %v", err)
 }
 wfengine.Run(context, workflow)
```

## Retry Policy & Backoff Examples

You can use helpers from `backoffs.go` for ergonomic retry strategies. For example:

```go
step := ewf.Step{
    Name: "StepA",
    RetryPolicy: &ewf.RetryPolicy{
        MaxAttempts: 3,
        BackOff:     ewf.ConstantBackoff(2 * time.Second),
    },
}

step := ewf.Step{
    Name: "StepB",
    RetryPolicy: &ewf.RetryPolicy{
        MaxAttempts: 5,
        BackOff:     ewf.ExponentialBackoff(500*time.Millisecond, 10*time.Second, 2.0),
    },
}
```

- `MaxAttempts` is the total number of attempts (including the first try).
- `BackOff` controls the delay pattern (constant, exponential, etc.).
- If `BackOff` is nil, the step will not be retried.
- Return `ewf.ErrFailWorkflowNow` to fail the workflow immediately, skipping retries.

## HTTP Server Example

The framework is perfect for building robust, asynchronous services. The included `httpexample` shows how to:

- Run the engine in a standard Go HTTP server.
- Start workflows asynchronously from an API endpoint.
- Immediately return a `workflow_id` to the client.
- Provide a separate `/status` endpoint to check the progress of a workflow.
- Automatically resume interrupted workflows when the server restarts.

To run the example:

```sh
cd examples/httpexample
go run main.go
```

In another terminal:

```sh
# Start a new workflow
curl -v http://localhost:8090/greet/EWF

# Check its status using the returned ID
curl http://localhost:8090/status/<workflow-id>
```

## Complex Struct State Example

The `structexample` demonstrates how to store and retrieve complex, nested structs in workflow state with type safety. **Important Note**: The workflow state uses `map[string]any`, so when retrieving structs, you must use type assertions (e.g., `person, ok := state["person"].(Person)`). This provides runtime type safety but requires careful type handling to avoid panics.

Key points:

- Store structs directly in state: `state["key"] = myStruct`
- Retrieve with type assertion: `myStruct, ok := state["key"].(MyStructType)`
- Always check the `ok` boolean to handle type assertion failures gracefully
- Nested structs work seamlessly with this approach

To run the example:

```sh
cd examples/structexample
go run main.go
```

## Running Tests

To run the test suite for the library:

```sh
go test -v ./...
```
