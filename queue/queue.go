package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/xmonader/ewf"
)

// Queue represents a workflow queue
type Queue interface {
	Name() string
	WorkflowName() string
	Enqueue(ctx context.Context, workflow *ewf.Workflow) error
	Dequeue(ctx context.Context) (*ewf.Workflow, error)
	Close(ctx context.Context) error
}

// WorkersDefinition defines the worker pool for processing workflows in the queue
type WorkersDefinition struct {
	Count        int
	PollInterval time.Duration
}

// QueueOptions defines options for the queue
type QueueOptions struct {
	AutoDelete  bool          // Delete when no longer in use
	DeleteAfter time.Duration // Ignored if AutoDelete is false
}

var _ Queue = (*RedisQueue)(nil)

type Processor func(ctx context.Context, wf *ewf.Workflow) error

// RedisQueue is the Redis implementation of the Queue interface
type RedisQueue struct {
	name         string
	workflowName string
	workersDef   WorkersDefinition
	queueOptions QueueOptions
	client       *redis.Client
	ch           chan struct{}
	processor    Processor
}

// Name returns the name of the queue
func (q *RedisQueue) Name() string {
	return q.name
}

// WorkflowName returns the associated workflow name of the queue
func (q *RedisQueue) WorkflowName() string {
	return q.workflowName
}

// Enqueue adds a workflow to the queue
func (q *RedisQueue) Enqueue(ctx context.Context, workflow *ewf.Workflow) error {

	data, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow %w", err)
	}

	return q.client.LPush(ctx, q.name, data).Err()
}

// Dequeue retrieves and removes a workflow from the queue
func (q *RedisQueue) Dequeue(ctx context.Context) (*ewf.Workflow, error) {

	res, err := q.client.BRPop(ctx, 5*time.Second, q.name).Result()

	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue error %w", err)
	}

	var wf ewf.Workflow
	if err := json.Unmarshal([]byte(res[1]), &wf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow %w", err)
	}

	return &wf, nil
}

// Start begins processing workflows in the queue using the provided processor function
func (q *RedisQueue) Start(ctx context.Context, proc Processor) {
	if proc == nil {
		panic("processor cannot be nil")
	}

	q.processor = proc
	if q.ch == nil {
		q.ch = make(chan struct{})
	}
	go q.workerLoop(ctx)
}

// Close closes the queue and deletes it after DeleteAfter duration if AutoDelete is set
func (q *RedisQueue) Close(ctx context.Context) error {
	close(q.ch)

	if q.queueOptions.AutoDelete {
		go func() {
			time.Sleep(q.queueOptions.DeleteAfter)
			q.client.Del(ctx, q.name)
		}()
	}
	return nil
}

func (q *RedisQueue) workerLoop(ctx context.Context) {
	for i := 0; i < q.workersDef.Count; i++ {
		ticker := time.NewTicker(q.workersDef.PollInterval)
		defer ticker.Stop()

		go func(workerID int) {
			for {
				select {
				case <-ctx.Done():
					return
				case <-q.ch:
					return
				case <-ticker.C:
					wf, err := q.Dequeue(ctx)
					if err != nil {
						fmt.Printf("Worker %d: error dequeuing workflow: %v\n", workerID, err)
						continue
					}
					if wf == nil {
						continue
					}
					
					fmt.Printf("Worker %d: processing workflow %s\n", workerID, wf.Name)

					if q.processor == nil {
						fmt.Printf("Worker %d: no processor configured\n", workerID)
						continue
					}
					if err := q.processor(ctx, wf); err != nil {
						fmt.Printf("Worker %d: error processing workflow %s: %v\n", workerID, wf.Name, err)
						continue
					}
					fmt.Printf("Worker %d: finished workflow %s\n", workerID, wf.Name)
				}
			}
		}(i)
	}
}
