package ewf

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// TestCreateAndGetQueue creates a queue engine, creates a queue, gets the created queue, and tries to get a non existing queue 
func TestCreateAndGetQueue(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer engine.Close(context.Background())

	store, err := NewSQLiteStore("test.db")
	if err != nil {
		t.Fatalf("store error: %v", err)
	}
	defer store.Close()

	wfengine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	q, err := engine.CreateQueue(
		context.Background(),
		"test-queue",
		"test-workflow",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
		wfengine,
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

// TestCloseQueue tests closing a queue and trying to get a closed queue, and closing a non existing queue
func TestCloseQueue(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer engine.Close(context.Background())

	store, err := NewSQLiteStore("test.db")
	if err != nil {
		t.Fatalf("store error: %v", err)
	}
	defer store.Close()

	wfengine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	_, err = engine.CreateQueue(
		context.Background(),
		"test-queue",
		"test-workflow",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
		wfengine,
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

// TestCloseEngine tests closing the engine with multiple queues and ensures all queues are closed, and tries to get a queue after engine is closed
func TestCloseEngine(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer engine.Close(context.Background())

	for i := 0; i < 3; i++ {
		store, err := NewSQLiteStore("test.db")
		if err != nil {
			t.Fatalf("store error: %v", err)
		}
		defer store.Close()

		wfengine, err := NewEngine(store)
		if err != nil {
			t.Fatalf("wf engine error: %v", err)
		}

		_, err = engine.CreateQueue(
			context.Background(),
			fmt.Sprintf("test-queue-%d", i),
			"test-workflow",
			WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
			QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
			wfengine,
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

// TestAutoDelete tests creating a queue with AutoDelete option, waits for time larger than DeleteAfter duration, and checks if the queue is deleted from redis and engine map
func TestAutoDelete(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer engine.Close(context.Background())

	store, err := NewSQLiteStore("test.db")
	if err != nil {
		t.Fatalf("store error: %v", err)
	}
	defer store.Close()

	wfengine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	q, err := engine.CreateQueue(
		context.Background(),
		"test-queue",
		"test-workflow",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
		wfengine,
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}
	// wait for longer than DeleteAfter duration
	time.Sleep(5 * time.Second)

	// check if the queue itself is removed from redis
	exists, err := q.(*RedisQueue).client.Exists(context.Background(), "test-queue").Result()
	if err != nil {
		t.Fatalf("failed to check queue existence: %v", err)
	}
	if exists != 0 {
		t.Errorf("expected queue to be deleted from Redis")
	}

	// check if the queue is removed from engine's map
	_, err = engine.GetQueue(context.Background(), "test-queue")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}

