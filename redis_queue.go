package ewf

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"time"

	"github.com/redis/go-redis/v9"
)

var _ Queue = (*redisQueue)(nil)

// redisQueue is the Redis implementation of the Queue interface
type redisQueue struct {
	name         string        // name of the queue
	queueOptions QueueOptions  // some queue configurations
	client       *redis.Client // Redis client
	closeCh      chan struct{} // channel to signal closure
	activityCh   chan struct{} // channel used to signal activity to avoid queue auto-deletion
}

func NewRedisQueue(queueName string, queueOptions QueueOptions, client *redis.Client) (Queue, error) {
	validName := regexp.MustCompile(`^[a-zA-Z0-9:_/]+$`)

	if !validName.MatchString(queueName) {
		return nil, fmt.Errorf("invalid queue name: %q, must be alphanumeric", queueName)
	}
	return &redisQueue{
		name:         queueName,
		queueOptions: queueOptions,
		client:       client,
		closeCh:      make(chan struct{}),
		activityCh:   make(chan struct{}),
	}, nil
}

// Name returns the name of the queue
func (q *redisQueue) Name() string {
	return q.name
}

// Length returns the length of the queue
func (q *redisQueue) Length(ctx context.Context) (int64, error) {
	length, err := q.client.LLen(ctx, q.name).Result()
	if err != nil {
		return -1, fmt.Errorf("failed to get queue length: %v", err)
	}
	return length, nil
}

// CloseCh returns the channel to signal queue closure
func (q *redisQueue) CloseCh() <-chan struct{} {
	return q.closeCh
}

// ActivityCh returns the channel to signal queue activity
func (q *redisQueue) ActivityCh() <-chan struct{} {
	return q.activityCh
}

// Enqueue adds a workflow to the queue
func (q *redisQueue) Enqueue(ctx context.Context, workflow *Workflow) error {

	data, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow %w", err)
	}

	err = q.client.LPush(ctx, q.name, data).Err()
	if err != nil {
		return err
	}

	// signal queue activity, avoid blocking on full channel
	select {
	case q.activityCh <- struct{}{}:
	default:
	}

	return nil
}

// Dequeue retrieves and removes a workflow from the queue
func (q *redisQueue) Dequeue(ctx context.Context) (*Workflow, error) {

	// default timeout to 1 second if not set
	timeout := q.queueOptions.PopTimeout
	if timeout <= 0 {
		timeout = 1 * time.Second
	}

	res, err := q.client.BRPop(ctx, timeout, q.name).Result()

	if err == redis.Nil {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("dequeue error %w", err)
	}

	// signal queue activity, avoid blocking on full channel
	select {
	case q.activityCh <- struct{}{}:
	default:
	}

	if len(res) < 2 {
		return nil, fmt.Errorf("dequeue error: result length should be at least 2")
	}

	var wf Workflow
	if err := json.Unmarshal([]byte(res[1]), &wf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow %w", err)
	}

	return &wf, nil
}

func (q *redisQueue) deleteQueue(ctx context.Context) error {
	if err := q.client.Del(ctx, q.name).Err(); err != nil {
		return fmt.Errorf("failed to delete queue %s: %v", q.name, err)
	}

	return nil
}

// Close closes the queue and deletes it from Redis
func (q *redisQueue) Close(ctx context.Context) error {
	select {
	case <-q.closeCh: // already closed
	default:
		close(q.closeCh)
	}

	return q.deleteQueue(ctx)
}
