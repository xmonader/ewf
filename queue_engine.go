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
	AutoDelete  bool          // Delete when no longer in use
	DeleteAfter time.Duration // Ignored if AutoDelete is false
	popTimeout  time.Duration // timeout for dequeue operations
}
