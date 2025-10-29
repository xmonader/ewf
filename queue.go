package ewf

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Queue represents a workflow queue
type Queue interface {
	Name() string
	WorkflowName() string
	Enqueue(ctx context.Context, workflow *Workflow) error
	Dequeue(ctx context.Context) (*Workflow, error)
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


// RedisQueue is the Redis implementation of the Queue interface
type RedisQueue struct {
	name         string
	workflowName string
	workersDef   WorkersDefinition
	queueOptions QueueOptions
	client       *redis.Client
	ch           chan struct{}
	wfEngine	 *Engine
}

func NewRedisQueue(queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions, client *redis.Client,wfEngine *Engine) *RedisQueue {
		return &RedisQueue{
		name:         queueName,
		workflowName: workflowName,
		workersDef:   workersDefinition,
		queueOptions: queueOptions,
		client:       client,
		ch:           make(chan struct{}),
		wfEngine:    wfEngine,
	}
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
func (q *RedisQueue) Enqueue(ctx context.Context, workflow *Workflow) error {

	data, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow %w", err)
	}

	return q.client.LPush(ctx, q.name, data).Err()
}

// Dequeue retrieves and removes a workflow from the queue
func (q *RedisQueue) Dequeue(ctx context.Context) (*Workflow, error) {

	res, err := q.client.BRPop(ctx, 5*time.Second, q.name).Result()

	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dequeue error %w", err)
	}

	var wf Workflow
	if err := json.Unmarshal([]byte(res[1]), &wf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow %w", err)
	}

	return &wf, nil
}

// Close closes the queue and deletes it after DeleteAfter duration if AutoDelete is set
func (q *RedisQueue) Close(ctx context.Context) error {

	if q.ch != nil {
		select {
		case <-q.ch: // already closed
		default:
			close(q.ch)
		}
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

					if err := q.wfEngine.RunSync(ctx, wf); err != nil {
						fmt.Printf("Worker %d: error processing workflow %s: %v\n", workerID, wf.Name, err)
					} else {
						fmt.Printf("Worker %d: successfully processed workflow %s\n", workerID, wf.Name)
					}
					
				}
			}
		}(i)
	}
}
