// Package ewf provides a workflow engine for defining and executing workflows in Go.
package ewf

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Engine is the central component for managing and executing workflows.
// It holds a registry of all available activities and a store for persistence.
// Engine is the central component for managing and executing workflows.
type Engine struct {
	activities  map[string]StepFn
	templates   map[string]*WorkflowTemplate
	store       Store
	queueEngine QueueEngine
}

// WorkersDefinition defines the worker pool for processing workflows in the queue
type WorkersDefinition struct {
	Count        int           `json:"worker_count"`  // number of workers
	PollInterval time.Duration `json:"poll_interval"` // interval between polling the queue for new workflows
	WorkTimeout  time.Duration `json:"work_timeout"`  // timeout for worker to process work
}

type EngineOption func(*Engine)

func Withstore(store Store) EngineOption {
	return func(e *Engine) {
		e.store = store
	}
}

func WithQueueEngine(queueEngine QueueEngine) EngineOption {
	return func(e *Engine) {
		e.queueEngine = queueEngine
	}
}

// NewEngine creates a new workflow engine.
func NewEngine(opts ...EngineOption) (*Engine, error) {
	engine := &Engine{
		activities: make(map[string]StepFn),
		templates:  make(map[string]*WorkflowTemplate),
	}

	for _, opt := range opts {
		opt(engine)
	}

	if engine.store != nil {
		if err := engine.store.Setup(); err != nil {
			return nil, fmt.Errorf("failed to setup store: %w", err)
		}

		templates, err := engine.store.LoadAllWorkflowTemplates(context.Background())
		if err == nil {
			for name, tmpl := range templates {
				engine.templates[name] = tmpl
			}
		}
	}

	if engine.queueEngine != nil && engine.store != nil {
		ctx := context.Background()
		queues, err := engine.store.LoadAllQueueMetadata(ctx)

		if err == nil {
			for _, qm := range queues {
				q, err := engine.queueEngine.CreateQueue(ctx, qm.Name, qm.QueueOptions)
				if err != nil {
					log.Printf("failed to recreate queue %s: %v", qm.Name, err)
					continue
				}

				engine.startQueueWorkers(ctx, q, qm.WorkersDef)
			}
		}
	}

	return engine, nil
}

// Register registers an activity function with the engine.
// This allows the activity to be used in workflow steps by its name.
// Register registers an activity function with the engine.
func (e *Engine) Register(name string, activity StepFn) {
	e.activities[name] = activity
}

// NewWorkflow creates a new workflow instance with the given name and steps.
// The workflow is associated with the engine's store.
// RegisterTemplate registers a workflow template with the engine.
// RegisterTemplate registers a workflow template with the engine.
func (e *Engine) RegisterTemplate(name string, def *WorkflowTemplate) {
	e.templates[name] = def
	if e.store != nil {
		_ = e.store.SaveWorkflowTemplate(context.Background(), name, def)
	}
}

// Store returns the store associated with the engine.
// This allows external access to the workflow store for querying workflow status and information.
// Users can use this method to retrieve workflow details by UUID, check workflow execution status,
// or list workflows with specific statuses. For example, after starting a workflow and receiving
// its UUID, clients can use engine.Store().LoadWorkflowByUUID(ctx, uuid) to get the workflow's current state.
// Store returns the store associated with the engine.
func (e *Engine) Store() Store {
	return e.store
}

// NewWorkflow creates a new workflow instance from a registered definition.
// NewWorkflow creates a new workflow instance from a registered definition.
func (e *Engine) NewWorkflow(name string) (*Workflow, error) {
	def, ok := e.templates[name]
	if !ok {
		return nil, fmt.Errorf("workflow template '%s' not registered", name)
	}

	w := NewWorkflow(name, WithStore(e.store))
	w.Steps = append([]Step{}, def.Steps...)
	w.beforeWorkflowHooks = append([]BeforeWorkflowHook{}, def.BeforeWorkflowHooks...)
	w.afterWorkflowHooks = append([]AfterWorkflowHook{}, def.AfterWorkflowHooks...)
	w.beforeStepHooks = append([]BeforeStepHook{}, def.BeforeStepHooks...)
	w.afterStepHooks = append([]AfterStepHook{}, def.AfterStepHooks...)

	return w, nil
}

// Run starts the execution of the given workflow.
// It resolves the activity for each step from the engine's registry.
// rehydrate applies the non-persisted fields from a workflow's definition.
// rehydrate applies the non-persisted fields from a workflow's definition.
func (e *Engine) rehydrate(w *Workflow) error {
	def, ok := e.templates[w.Name]
	if !ok {
		return fmt.Errorf("workflow template '%s' not registered", w.Name)
	}
	w.SetStore(e.store) // Re-attach the store for resumed workflows
	w.Steps = append([]Step{}, def.Steps...)
	w.beforeWorkflowHooks = append([]BeforeWorkflowHook{}, def.BeforeWorkflowHooks...)
	w.afterWorkflowHooks = append([]AfterWorkflowHook{}, def.AfterWorkflowHooks...)
	w.beforeStepHooks = append([]BeforeStepHook{}, def.BeforeStepHooks...)
	w.afterStepHooks = append([]AfterStepHook{}, def.AfterStepHooks...)
	return nil
}

// RunSync runs a workflow synchronously and blocks until it completes.
// RunSync runs a workflow synchronously and blocks until it completes.
func (e *Engine) RunSync(ctx context.Context, w *Workflow) (err error) {
	if err := e.rehydrate(w); err != nil {
		return err
	}
	// register all after workflow hooks
	defer func() {
		for _, hook := range w.afterWorkflowHooks {
			hook(ctx, w, err)
		}
	}()

	// execute all before workflow hooks
	for _, hook := range w.beforeWorkflowHooks {
		hook(ctx, w)
	}

	return w.run(ctx, e.activities)
}

// RunAsync runs a workflow asynchronously in a new goroutine.
// Errors are logged to standard output.
// RunAsync runs a workflow asynchronously in a new goroutine.
func (e *Engine) RunAsync(ctx context.Context, w *Workflow) {
	go func() {
		if err := e.RunSync(ctx, w); err != nil {
			// In a real application, you'd use a structured logger.
			log.Printf("async workflow %s failed: %v", w.UUID, err)
		}
	}()
}

// ResumeRunningWorkflows finds all workflows with a 'running' status in the store
// and resumes their execution in the background.
// ResumeRunningWorkflows finds all workflows with a 'running' status in the store and resumes their execution in the background.
func (e *Engine) ResumeRunningWorkflows() {
	go func() {
		// Create a new context for the background resumption process.
		ctx := context.Background()

		uuids, err := e.Store().ListWorkflowUUIDsByStatus(ctx, StatusRunning)
		if err != nil {
			log.Printf("error listing workflows for resumption: %v", err)
			return
		}

		if len(uuids) == 0 {
			log.Println("No pending workflows to resume.")
			return
		}

		log.Printf("Resuming %d pending workflows in the background...", len(uuids))
		for _, id := range uuids {
			wf, err := e.Store().LoadWorkflowByUUID(ctx, id)
			if err != nil {
				log.Printf("failed to load workflow %s for resumption: %v", id, err)
				continue
			}
			log.Printf("Resuming workflow %s", wf.UUID)
			e.RunAsync(ctx, wf)
		}
	}()
}

// CreateQueue creates a new queue and starts workers for it
func (e *Engine) CreateQueue(ctx context.Context, queueName string, workerDef WorkersDefinition, queueOptions QueueOptions) (Queue, error) {
	if e.queueEngine == nil {
		return nil, fmt.Errorf("no queue engine configured")
	}

	queue, err := e.queueEngine.CreateQueue(ctx, queueName, queueOptions)
	if err != nil {
		return nil, err
	}

	if e.Store() != nil {
		// save queue configurations to sqlite store
		err = e.Store().SaveQueueMetadata(ctx, &QueueMetadata{
			Name:         queueName,
			WorkersDef:   workerDef,
			QueueOptions: queueOptions,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to save queue settings: %v", err)
		}
	}

	if queueOptions.AutoDelete && queueOptions.DeleteAfter > 0 {
		e.monitorAutoDelete(ctx, queue, queueOptions)
	}

	e.startQueueWorkers(ctx, queue, workerDef)

	return queue, nil
}

func (e *Engine) startQueueWorkers(ctx context.Context, q Queue, workerDef WorkersDefinition) {

	for i := 0; i < workerDef.Count; i++ {

		go func(workerID int) {
			ticker := time.NewTicker(workerDef.PollInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-q.CloseCh():
					return
				case <-ticker.C:
					wf, err := q.Dequeue(ctx)
					if err != nil {
						log.Printf("Worker %d: error dequeuing workflow: %v\n", workerID, err)
						continue
					}

					if wf == nil {
						continue
					}

					workCtx := ctx
					cancel := func() {}
					if workerDef.WorkTimeout > 0 {
						workCtx, cancel = context.WithTimeout(ctx, workerDef.WorkTimeout)
					}

					if err := e.RunSync(workCtx, wf); err != nil {
						log.Printf("Worker %d: error processing workflow %s: %v\n", workerID, wf.Name, err)
					}
					cancel()
				}
			}
		}(i)
	}
}

func (e *Engine) deleteFromStore(ctx context.Context, q Queue) error {
	if e.Store() != nil {
		err := e.Store().DeleteQueueMetadata(ctx, q.Name())
		return err
	}
	return nil
}

func (e *Engine) monitorAutoDelete(ctx context.Context, q Queue, queueOptions QueueOptions) {

	go func() {
		timer := time.NewTimer(queueOptions.DeleteAfter)

		for {
			select {
			case <-ctx.Done():
				return
			case <-q.CloseCh():
				return
			case <-q.ActivityCh(): // queue is active, reset timer
				if !timer.Stop() {
					<-timer.C // drain if needed
				}
				timer.Reset(queueOptions.DeleteAfter)

			case <-timer.C: // timer expired, close queue if it's empty

				length, err := q.Length(ctx)
				if err != nil {
					log.Printf("failed to check queue length: %v", err)
					continue
				}

				if length == 0 {
					if err := e.queueEngine.CloseQueue(ctx, q.Name()); err != nil {
						log.Printf("error deleting queue: %v", err)

					} else {
						if err := e.deleteFromStore(ctx, q); err != nil {
							log.Printf("error deleting queue: %s from store\n", q.Name())
						}
					}
					return
				}
			}
		}
	}()
}
