package ewf

import "time"

// QueueMetadata defines the queue data that needs to persist
type QueueMetadata struct {
	Name         string            `json:"name"`
	WorkersDef   WorkersDefinition `json:"worker_def"`
	QueueOptions QueueOptions      `json:"queue_options"`
	IdleSince    time.Time         `json:"idle_since"`
}
