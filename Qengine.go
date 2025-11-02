package ewf

import (
	"context"
)

// QueueEngine defines the interface for a queue engine that manages multiple queues
type QueueEngine interface {
	CreateQueue(ctx context.Context, queueName string, workflowName string, workersDefinition WorkersDefinition, queueOptions QueueOptions, wfEngine *Engine) (Queue, error)
	GetQueue(ctx context.Context, queueName string) (Queue, error)
	CloseQueue(ctx context.Context, queueName string) error
	Close(ctx context.Context) error
}
