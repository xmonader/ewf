package ewf

import (
	"context"
	"time"
)

// Queue represents a workflow queue
type Queue interface {
	Name() string
	Enqueue(ctx context.Context, workflow *Workflow) error
	Dequeue(ctx context.Context) (*Workflow, error)
	Close(ctx context.Context) error
	WorkersDefinition() WorkersDefinition
	CloseCh() <-chan struct{}
}

// WorkersDefinition defines the worker pool for processing workflows in the queue
type WorkersDefinition struct {
	Count        int           // number of workers
	PollInterval time.Duration // interval between polling the queue for new workflows
}

// QueueOptions defines options for the queue
type QueueOptions struct {
	AutoDelete  bool          // Delete when no longer in use
	DeleteAfter time.Duration // Ignored if AutoDelete is false
	popTimeout  time.Duration // timeout for dequeue operations
}
