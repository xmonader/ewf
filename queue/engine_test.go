package queue

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCreateAndGetQueue(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")

	q, err := engine.CreateQueue(
		context.Background(),
		"test-queue",
		"test-workflow",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	if engine.queues["test-queue"] == nil {
		t.Errorf("expected queue to be created")
	}

	// get the created queue
	gotQueue, err := engine.GetQueue(context.Background(), "test-queue")
	if err != nil {
		t.Fatalf("failed to get queue: %v", err)
	}
	if gotQueue.Name() != q.Name() {
		t.Errorf("expected queue name %s, got %s", q.Name(), gotQueue.Name())
	}

	// get a non existing queue
	_, err = engine.GetQueue(context.Background(), "test")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}

}

func TestCloseQueue(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")

	_, err := engine.CreateQueue(
		context.Background(),
		"test-queue",
		"test-workflow",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	err = engine.CloseQueue(context.Background(), "test-queue")
	if err != nil {
		t.Fatalf("failed to close queue: %v", err)
	}

	// try to get the closed queue
	_, err = engine.GetQueue(context.Background(), "test-queue")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}

	// close a non existing queue
	err = engine.CloseQueue(context.Background(), "test")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}

func TestCloseEngine(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")

	for i := 0; i < 3; i++ {
		_, err := engine.CreateQueue(
			context.Background(),
			fmt.Sprintf("test-queue-%d", i),
			"test-workflow",
			WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
			QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
		)
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}
	}
	err := engine.Close(context.Background())
	if err != nil {
		t.Fatalf("failed to close engine: %v", err)
	}

	if len(engine.queues) != 0 {
		t.Errorf("expected all queues to be closed")
	}

	// try to get a queue after engine is closed
	_, err = engine.GetQueue(context.Background(), "test-queue-0")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}
