package ewf

import (
	"context"
	"encoding/json"
	"fmt"

	"time"

	"github.com/redis/go-redis/v9"
)

var _ Queue = (*RedisQueue)(nil)

// RedisQueue is the Redis implementation of the Queue interface
type RedisQueue struct {
	name         string            // name of the queue
	workersDef   WorkersDefinition // definition of the worker pool
	queueOptions QueueOptions      // options for the queue
	client       *redis.Client     // Redis client
	closeCh      chan struct{}     // channel to signal closure
	idleSince    *time.Time        // queue idle time used in auto-deletion
}

func NewRedisQueue(queueName string, workersDefinition WorkersDefinition, queueOptions QueueOptions, client *redis.Client, idleSince *time.Time) *RedisQueue {
	return &RedisQueue{
		name:         queueName,
		workersDef:   workersDefinition,
		queueOptions: queueOptions,
		client:       client,
		closeCh:      make(chan struct{}),
		idleSince:    idleSince,
	}
}

// Name returns the name of the queue
func (q *RedisQueue) Name() string {
	return q.name
}

// WorkersDefinition returns the queue workers definition
func (q *RedisQueue) WorkersDefinition() WorkersDefinition {
	return q.workersDef
}

// CloseCh returns the channel to signal queue closure
func (q *RedisQueue) CloseCh() <-chan struct{} {
	return q.closeCh
}

// Enqueue adds a workflow to the queue
func (q *RedisQueue) Enqueue(ctx context.Context, workflow *Workflow) error {

	data, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow %w", err)
	}

	err = q.client.LPush(ctx, q.name, data).Err()
	if err != nil {
		return err
	}

	now := time.Now()
	q.idleSince = &now
	
	return nil
}

// Dequeue retrieves and removes a workflow from the queue
func (q *RedisQueue) Dequeue(ctx context.Context) (*Workflow, error) {

	// default timeout to 1 second if not set
	timeout := q.queueOptions.popTimeout
	if timeout <= 0 {
		timeout = 1 * time.Second
	}

	res, err := q.client.BRPop(ctx, timeout, q.name).Result()

	if err == redis.Nil {
		return nil, nil
	}

	if err != nil || len(res) < 2 {
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
