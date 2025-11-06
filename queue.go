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
	CloseCh() <-chan struct{}
}

// QueueMetadata defines the queue data that needs to persist
type QueueSettings struct {
	Name         string            `json:"name"`
	WorkersDef   WorkersDefinition `json:"worker_def"`
	QueueOptions QueueOptions      `json:"queue_options"`
	LastActiveAt time.Time         `json:"idle_since"`
}
