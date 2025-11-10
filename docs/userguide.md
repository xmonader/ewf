# EWF User Guide

Welcome to the **EWF (Embeddedable Workflow Framework)** user guide! This document will help you understand the building blocks, features, and best practices for using EWF to build robust, durable, and idempotent workflows in Go.

---

## Table of Contents
1. [Concepts & Motivation](#concepts--motivation)
2. [Quickstart: Your First Workflow](#quickstart-your-first-workflow)
3. [Defining Steps](#defining-steps)
4. [Hooks: Before/After Workflow & Steps](#hooks-beforeafter-workflow--steps)
5. [Error Handling & Retries](#error-handling--retries)
6. [Step Timeouts](#step-timeouts)
7. [Idempotency: Safe & Repeatable Steps](#idempotency-safe--repeatable-steps)
8. [Persistence & Recovery](#persistence--recovery)
9. [Queue & Queue Engine](#queue--queue-engine)
10. [Testing Workflows](#testing-workflows)
11. [Best Practices & Patterns](#best-practices--patterns)

---

## 1. Concepts & Motivation

**EWF** is a minimal, ergonomic Go library for orchestrating durable workflows. It is designed to:
- Make complex, multi-step business processes reliable, observable, and resumable
- Provide hooks, retries, timeouts, and idempotency support out of the box
- Be simple to use, with a focus on Go idioms and best practices

**What EWF adds:**
- Workflow and step definitions as Go structs
- Pluggable persistence (SQLite, etc.) for durability
- Retry policies, timeouts, and lifecycle hooks
- Ergonomic idempotency patterns
- Easy integration with your business logic

---

## 2. Quickstart: Your First Workflow

```go
// Define your step functions
func StepA(ctx context.Context, state ewf.State) error {
    // ...
    return nil
}
func StepB(ctx context.Context, state ewf.State) error {
    // ...
    return nil
}

// Register steps and workflow
engine := ewf.NewEngine(store)
engine.Register("StepA", StepA)
engine.Register("StepB", StepB)

tmpl := &ewf.WorkflowTemplate{
    Steps: []ewf.Step{
        {Name: "StepA"},
        {Name: "StepB"},
    },
}
engine.RegisterTemplate("my_workflow", tmpl)

// Run a workflow
wf, _ := engine.NewWorkflow("my_workflow")
_ = engine.RunSync(context.Background(), wf)
```

---

## 3. Defining Steps

A **step** is a unit of work. Each step function has the signature:
```go
func(ctx context.Context, state ewf.State) error
```
- `ctx`: carries cancellation, timeout, and metadata
- `state`: a map for storing workflow state between steps

Steps are registered with names and referenced in workflow templates.

---

## 4. Hooks: Before/After Workflow & Steps

EWF supports lifecycle hooks for custom logic:
- **Before/After Workflow Hooks**: Run at workflow start/end
- **Before/After Step Hooks**: Run before/after each step

Example:
```go
tmpl := &ewf.WorkflowTemplate{
    Steps: ...,
    BeforeWorkflowHooks: []ewf.BeforeWorkflowHook{
        func(ctx context.Context, wf *ewf.Workflow) { log.Println("Starting workflow") },
    },
    AfterWorkflowHooks: []ewf.AfterWorkflowHook{
        func(ctx context.Context, wf *ewf.Workflow, err error) { log.Println("Workflow finished") },
    },
}
```

---

## 5. Error Handling & Retries

Each step can specify a flexible retry policy using the standard [backoff library](https://pkg.go.dev/github.com/cenkalti/backoff/v4) and ergonomic helpers in `backoffs.go`:

```go
// Constant backoff (fixed delay between retries)
step := ewf.Step{
    Name: "StepA",
    RetryPolicy: &ewf.RetryPolicy{
        MaxAttempts: 3, // total attempts (including the first)
        BackOff:     ewf.ConstantBackoff(2 * time.Second),
    },
}

// Exponential backoff (delay grows exponentially)
step := ewf.Step{
    Name: "StepB",
    RetryPolicy: &ewf.RetryPolicy{
        MaxAttempts: 5,
        BackOff:     ewf.ExponentialBackoff(500*time.Millisecond, 10*time.Second, 2.0),
    },
}
```
- `MaxAttempts` caps the number of attempts (including the first try).
- The `BackOff` field controls the delay pattern (constant, exponential, etc.).
- Use helpers from `backoffs.go` for common patterns.
- If `BackOff` is nil, the step will not be retried.
- Return `ewf.ErrFailWorkflowNow` to fail the workflow immediately, skipping retries.

---

## 6. Step Timeouts

You can set a timeout for each step:
```go
step := ewf.Step{
    Name:    "LongTask",
    Timeout: 30 * time.Second, // Step must finish in 30s
}
```
- The engine will cancel the step's context after the timeout
- **Best practice:** Use context-aware APIs and check `ctx.Done()` in your step for graceful cancellation

---

## 7. Idempotency: Safe & Repeatable Steps

**Why?** Durable workflows may retry steps or resume after crashes. To avoid duplicate side effects (e.g., double-charging a user), steps should be idempotent.

**Pattern:** Use the step name from context and flags in state:
```go
func MyStep(ctx context.Context, state ewf.State) error {
    stepName, _ := ctx.Value(ewf.StepNameContextKey).(string)
    idempotencyKey := fmt.Sprintf("__completed_%s", stepName)
    if _, done := state[idempotencyKey]; done {
        return nil // Already done
    }
    // Do side effect
    state[idempotencyKey] = true
    return nil
}
```
- For data-dependent idempotency, include input data in the key
- For external APIs, use idempotency keys if supported

---

## 8. Persistence & Recovery

- EWF supports pluggable persistence (e.g., SQLite) for workflow state and queue metadata
- Workflows can be resumed after process restarts or crashes
- State is saved after each step

---
## 9. Queue & Queue Engine

EWF includes a lightweight, embeddable **queue system** designed for asynchronous, concurrent task execution.

### Queue

The **Queue** is a concurrent-safe structure that allows you to enqueue and process jobs (like workflow runs or background tasks).  
Each queue automatically starts its worker loop upon creation.

Key features:
- Thread-safe implementation  
- Automatic worker pool management  
- Configurable number of workers  
- Graceful shutdown via context cancellation
- Optional persistence

Example:
```go
    client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	queue := NewRedisQueue(
		"testQueue",
		QueueOptions{AutoDelete: false},
		client,
	)
	defer queue.Close(context)

	workflow := NewWorkflow("workflow")
	err = queue.Enqueue(context, workflow)
	if err != nil {
		return fmt.Errorf("failed to enqueue workflow: %v", err)
	}
	
	wf, err := queue.Dequeue(context)
	if err != nil {
		return fmt.Errorf("failed to dequeue workflow: %v", err)
	}

```

### Queue Engine 
The **Queue Engine** acts as a scheduler and execution manager for queued jobs, ensuring:
* Automatic worker startup when a queue is created.
* Graceful shutdowns respecting context cancellation.
* Optional persistence layer integration for durable queues.

Example:
```go
    client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	engine := NewRedisQueueEngine(client)
	defer func() {
		if err := engine.Close(context); err != nil {
			fmt.Errorf("failed to close engine: %v", err)
		}
	}

	queue, err := engine.CreateQueue(
		context,
		"testQueue",
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
	)
	if err != nil {
		fmt.Errorf("failed to create queue: %v", err)
	}

	// get the created queue
	gotQueue, err := engine.GetQueue(t.Context(), "testQueue")
	if err != nil {
		fmt.Errorf("failed to get queue: %v", err)
	}
```

## 10. Testing Workflows

- Use Go's `testing` package for unit, integration, and end-to-end tests
- Use the in-memory or SQLite store for fast, realistic tests
- Example:
```go
t := testing.T{}
engine := ewf.NewEngine(ewf.NewInMemoryStore())
// ...register steps and templates...
wf, _ := engine.NewWorkflow("my_workflow")
err := engine.RunSync(context.Background(), wf)
if err != nil {
    t.Fatalf("workflow failed: %v", err)
}
```

---

## 11. Best Practices & Patterns

- **Always use context-aware APIs in steps** (e.g., HTTP, DB)
- **Check for ctx.Done()** in long-running steps
- **Mark idempotency in state before side effects**
- **Test your workflows** with realistic stores
- **Use hooks for logging, metrics, and auditing**
- **Document your step input/output contracts**

---

## More Resources
- See the `examples/` directory for CLI and HTTP workflow demos
- Read the GoDoc for API details
- File issues or feature requests on GitHub!

---

Happy workflowing! 🚀
