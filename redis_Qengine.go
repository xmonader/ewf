package ewf

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrQueueNotFound = fmt.Errorf("queue not found")

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

// CreateQueue creates a new queue and uses 0 timeout by default for dequeue operations
func (e *RedisQueueEngine) CreateQueue(ctx context.Context, queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions, wfEngine *Engine) (Queue, error) {
	return e.CreateQueueWithTimeout(ctx, queueName, workflowName, workersDefinition, queueOptions, wfEngine, 0)
}

// CreateQueue creates a new queue and uses the passes timeout for dequeue operations
func (e *RedisQueueEngine) CreateQueueWithTimeout(ctx context.Context, queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions, wfEngine *Engine, popTimeout time.Duration) (Queue, error) {
	e.mu.Lock()

	if _, ok := e.queues[queueName]; ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("queue %s already exists", queueName)
	}

	q := NewRedisQueue(
		queueName,
		workflowName,
		workersDefinition,
		queueOptions,
		e.client,
		wfEngine,
		func(name string) {
			e.mu.Lock()
			defer e.mu.Unlock()
			delete(e.queues, name)
		},
		popTimeout,
	)

	e.queues[queueName] = q
	e.mu.Unlock()

	e.startQueueWorkers(ctx, q)

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

func (e *RedisQueueEngine) startQueueWorkers(ctx context.Context, q *RedisQueue) {
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

									q.closeOnce.Do(func() {
										if err := q.deleteQueue(ctx); err != nil {
											fmt.Println("deleteQueue error:", err)
										}
										close(q.closeCh)
										if q.onDelete != nil {
											q.onDelete(q.name)
										}
									})
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
