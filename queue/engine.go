package queue

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

// QueueEngine defines the interface for a queue engine that manages multiple queues
type QueueEngine interface {
	CreateQueue(ctx context.Context, queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions) (Queue, error)
	GetQueue(ctx context.Context, queueName string) (Queue, error)
	CloseQueue(ctx context.Context, queueName string) error
	Close(ctx context.Context) error
}

var _ QueueEngine = (*RedisQueueEngine)(nil)

// RedisQueueEngine is the Redis implementation of the QueueEngine interface
type RedisQueueEngine struct {
	client *redis.Client
	mu     sync.Mutex
	queues map[string]*RedisQueue
}

// NewRedisQueueEngine creates a new RedisQueueEngine with the given Redis address
func NewRedisQueueEngine(address string) *RedisQueueEngine {
	client := redis.NewClient(&redis.Options{
		Addr: address,
	})

	return &RedisQueueEngine{
		client: client,
		queues: make(map[string]*RedisQueue),
	}
}

// CreateQueue creates a new queue with the specified parameters
func (e *RedisQueueEngine) CreateQueue(ctx context.Context, queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions) (Queue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if _, ok := e.queues[queueName]; ok {
		return nil, fmt.Errorf("queue %s already exists", queueName)
	}

	q := &RedisQueue{
		name:         queueName,
		workflowName: workflowName,
		workersDef:   workersDefinition,
		queueOptions: queueOptions,
		client:       e.client,
		ch:           make(chan struct{}),
	}
	e.queues[queueName] = q

	return q, nil
}

// GetQueue retrieves an existing queue by its name
func (e *RedisQueueEngine) GetQueue(ctx context.Context, queueName string) (Queue, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	q, ok := e.queues[queueName]
	if !ok {
		return nil, fmt.Errorf("queue %s does not exist", queueName)
	}

	return q, nil
}

// CloseQueue closes and removes a queue by its name
func (e *RedisQueueEngine) CloseQueue(ctx context.Context, queueName string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	q, ok := e.queues[queueName]
	if !ok {
		return fmt.Errorf("queue %s does not exist", queueName)
	}

	q.Close(ctx)
	delete(e.queues, queueName)
	return nil
}

// Close closes all queues and the Redis client
func (e *RedisQueueEngine) Close(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, q := range e.queues {
		q.Close(ctx)
	}

	e.queues = make(map[string]*RedisQueue)
	return e.client.Close()
}
