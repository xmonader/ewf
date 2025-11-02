package ewf

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"time"

	"github.com/redis/go-redis/v9"
)

// WorkersDefinition defines the worker pool for processing workflows in the queue
type WorkersDefinition struct {
	Count        int           // number of workers
	PollInterval time.Duration // interval between polling the queue for new workflows
}

// QueueOptions defines options for the queue
type QueueOptions struct {
	AutoDelete  bool          // Delete when no longer in use
	DeleteAfter time.Duration // Ignored if AutoDelete is false
}

var _ Queue = (*RedisQueue)(nil)

// RedisQueue is the Redis implementation of the Queue interface
type RedisQueue struct {
	name         string            // name of the queue
	workflowName string            // associated workflow name
	workersDef   WorkersDefinition // definition of the worker pool
	queueOptions QueueOptions      // options for the queue
	client       *redis.Client     // Redis client
	closeCh      chan struct{}     // channel to signal closure
	popTimeout   time.Duration     // timeout for dequeue operations
	closeOnce    sync.Once		   // ensure closing the queue by one worker only 
}

func NewRedisQueue(queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions, client *redis.Client, popTimeout time.Duration) *RedisQueue {
	return &RedisQueue{
		name:         queueName,
		workflowName: workflowName,
		workersDef:   workersDefinition,
		queueOptions: queueOptions,
		client:       client,
		closeCh:      make(chan struct{}),
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
