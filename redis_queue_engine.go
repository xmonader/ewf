package ewf

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrQueueNotFound indicates that the queue is deleted or never created
var ErrQueueNotFound error = errors.New("queue doesn't exist")

var _ QueueEngine = (*RedisQueueEngine)(nil)

// RedisQueueEngine is the Redis implementation of the QueueEngine interface
type RedisQueueEngine struct {
	client *redis.Client
	mu     sync.Mutex
	queues map[string]*RedisQueue
}

// NewRedisQueueEngine creates a new RedisQueueEngine with the given Redis address
func NewRedisQueueEngine(client *redis.Client) *RedisQueueEngine {
	return &RedisQueueEngine{
		client: client,
		queues: make(map[string]*RedisQueue),
	}
}

// CreateQueue creates a new queue
func (e *RedisQueueEngine) CreateQueue(ctx context.Context, queueName string, queueOptions QueueOptions) (Queue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.queues[queueName]; ok {
		return nil, fmt.Errorf("queue %s already exists", queueName)
	}

	idleSince := time.Now()

	q := NewRedisQueue(
		queueName,
		queueOptions,
		e.client,
		&idleSince,
	)

	e.queues[queueName] = q

	if queueOptions.AutoDelete {
		e.monitorAutoDelete(ctx, q)
	}

	return q, nil
}

// GetQueue retrieves an existing queue by its name
func (e *RedisQueueEngine) GetQueue(ctx context.Context, queueName string) (Queue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	q, ok := e.queues[queueName]
	if !ok {
		return nil, ErrQueueNotFound
	}

	return q, nil
}

// CloseQueue closes and removes a queue by its name
func (e *RedisQueueEngine) CloseQueue(ctx context.Context, queueName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	q, ok := e.queues[queueName]
	if !ok {
		return ErrQueueNotFound
	}

	if err := q.Close(ctx); err != nil {
		return err
	}

	delete(e.queues, queueName)

	return nil
}

// Close closes all queues and the Redis client
func (e *RedisQueueEngine) Close(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, q := range e.queues {
		err := q.Close(ctx)
		if err != nil {
			return err
		}
	}

	e.queues = make(map[string]*RedisQueue)
	return e.client.Close()
}

func (e *RedisQueueEngine) monitorAutoDelete(ctx context.Context, q *RedisQueue) {

	go func() {
		ticker := time.NewTicker(1*time.Second) // TODO: FIX
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-q.closeCh:
				return
			case <-ticker.C:
				length, err := q.client.LLen(ctx, q.name).Result()
				if err != nil {
					log.Printf("failed to check queue length: %v", err)
					continue
				}

				if length == 0 {

					if time.Since(*q.idleSince) >= q.queueOptions.DeleteAfter {

						if err := e.CloseQueue(ctx, q.name); err != nil {
							log.Printf("error deleting queue: %v", err)
							return
						}
						log.Printf("Successfully auto-deleted queue: %s\n", q.name)
					}
				}
			}
		}
	}()
}
