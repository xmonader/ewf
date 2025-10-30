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
	closeCh      chan struct{}
	wfEngine     *Engine
	onDelete     func(string)
	popTimeout   time.Duration
}

func NewRedisQueue(queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions, client *redis.Client, wfEngine *Engine, onDelete func(string), popTimeout time.Duration) *RedisQueue {
	return &RedisQueue{
		name:         queueName,
		workflowName: workflowName,
		workersDef:   workersDefinition,
		queueOptions: queueOptions,
		client:       client,
		closeCh:      make(chan struct{}),
		wfEngine:     wfEngine,
		onDelete:     onDelete,
		popTimeout:   popTimeout,
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

	// default timeout to 1 second if not set
	timeout := q.popTimeout
	if timeout <= 0 {
		timeout = 1 * time.Second
	}

	res, err := q.client.BRPop(ctx, timeout, q.name).Result()

	if err == redis.Nil {
		return nil, err
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

func (q *RedisQueue) deleteQueue(ctx context.Context) error {
	if err := q.client.Del(ctx, q.name).Err(); err != nil {
		return fmt.Errorf("failed to delete queue %s: %v", q.name, err)
	}

	return nil
}

// Close closes the queue and deletes it from Redis
func (q *RedisQueue) Close(ctx context.Context) error {

	if q.closeCh != nil {
		select {
		case <-q.closeCh: // already closed
		default:
			close(q.closeCh)
		}
	}

	return q.deleteQueue(ctx)
}

func (q *RedisQueue) workerLoop(ctx context.Context) {
	for i := 0; i < q.workersDef.Count; i++ {

		go func(workerID int) {
			ticker := time.NewTicker(q.workersDef.PollInterval)
			defer ticker.Stop()

			var idleSince *time.Time

			for {
				select {
				case <-ctx.Done():
					return
				case <-q.closeCh:
					return
				case <-ticker.C:
					wf, err := q.Dequeue(ctx)

					if err != nil && err != redis.Nil {
						fmt.Printf("Worker %d: error dequeuing workflow: %v\n", workerID, err)
						continue
					}

					if wf == nil { // empty queue
						if q.queueOptions.AutoDelete {
							now := time.Now()

							if idleSince == nil {
								idleSince = &now
							}

							if time.Since(*idleSince) >= q.queueOptions.DeleteAfter {
								length, err := q.client.LLen(ctx, q.name).Result()

								if err == nil && length == 0 {

									fmt.Printf("auto-deleting empty queue: %s\n", q.name)
									if err := q.deleteQueue(ctx); err != nil {
										fmt.Println("deleteQueue error:", err)
									}
									close(q.closeCh)
									if q.onDelete != nil {
										q.onDelete(q.name)
									}
									return
								}
							}
						}
						continue
					}
					idleSince = nil // reset idle timer

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
