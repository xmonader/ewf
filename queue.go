package ewf

import (
	"context"
)

// Queue represents a workflow queue
type Queue interface {
	Name() string
	Enqueue(ctx context.Context, workflow *Workflow) error
	Dequeue(ctx context.Context) (*Workflow, error)
	Close(ctx context.Context) error
	CloseCh() <-chan struct{}
}
