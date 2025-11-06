package ewf

import (
	"context"
	"time"
)

// QueueEngine defines the interface for a queue engine that manages multiple queues
type QueueEngine interface {
	CreateQueue(ctx context.Context, queueName string, queueOptions QueueOptions) (Queue, error)
	GetQueue(ctx context.Context, queueName string) (Queue, error)
	CloseQueue(ctx context.Context, queueName string) error
	Close(ctx context.Context) error
}

// QueueOptions defines options for the queue
type QueueOptions struct {
	AutoDelete  bool          `json:"auto_delete"`  // Delete when no longer in use
	DeleteAfter time.Duration `json:"delete_after"` // Ignored if AutoDelete is false
	PopTimeout  time.Duration `json:"pop_timeout"`  // timeout for dequeue operations
}
