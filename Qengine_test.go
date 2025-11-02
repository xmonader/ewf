package ewf

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func waitStep(duration time.Duration) StepFn {
	return func(ctx context.Context, state State) error {
		time.Sleep(duration)
		return nil
	}
}

func registerWf(wfengine *Engine, t *testing.T, wfName string) {

	t.Helper()

	wfengine.Register("wait_5", waitStep(5*time.Millisecond))
	wfengine.Register("wait_10", waitStep(10*time.Millisecond))
	wfengine.Register("wait_15", waitStep(15*time.Millisecond))

	def := &WorkflowTemplate{
		Steps: []Step{
			{Name: "wait_5"},
			{Name: "wait_10"},
			{Name: "wait_15"},
		},
	}
	wfengine.RegisterTemplate(wfName, def)
}

// TestCreateAndGetQueue creates a queue engine, creates a queue, gets the created queue, and tries to get a non existing queue
func TestCreateAndGetQueue(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer func() {
		if err := engine.Close(t.Context()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	}()

	q, err := engine.CreateQueue(
		t.Context(),
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
	gotQueue, err := engine.GetQueue(t.Context(), "test-queue")
	if err != nil {
		t.Fatalf("failed to get queue: %v", err)
	}
	if gotQueue.Name() != q.Name() {
		t.Errorf("expected queue name %s, got %s", q.Name(), gotQueue.Name())
	}

	// get a non existing queue
	_, err = engine.GetQueue(t.Context(), "test")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}

}

// TestCloseQueue tests closing a queue and trying to get a closed queue, and closing a non existing queue
func TestCloseQueue(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer func() {
		if err := engine.Close(t.Context()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	}()

	_, err := engine.CreateQueue(
		t.Context(),
		"test-queue",
		"test-workflow",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
	)

	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	err = engine.CloseQueue(t.Context(), "test-queue")
	if err != nil {
		t.Fatalf("failed to close queue: %v", err)
	}

	// try to get the closed queue
	_, err = engine.GetQueue(t.Context(), "test-queue")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}

	// close a non existing queue
	err = engine.CloseQueue(t.Context(), "test")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}

// TestCloseEngine tests closing the engine with multiple queues and ensures all queues are closed, and tries to get a queue after engine is closed
func TestCloseEngine(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")

	for i := 0; i < 3; i++ {
		_, err := engine.CreateQueue(
			t.Context(),
			fmt.Sprintf("test-queue-%d", i),
			"test-workflow",
			WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
			QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
		)
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}
	}

	err := engine.Close(t.Context())
	if err != nil {
		t.Fatalf("failed to close engine: %v", err)
	}

	if len(engine.queues) != 0 {
		t.Errorf("expected all queues to be closed")
	}

	// try to get a queue after engine is closed
	_, err = engine.GetQueue(t.Context(), "test-queue-0")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}

// TestAutoDelete tests creating a queue with AutoDelete option, waits for time larger than DeleteAfter duration, and checks if the queue is deleted from redis and engine map
func TestAutoDelete(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer func() {
		if err := engine.Close(t.Context()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	}()

	q, err := engine.CreateQueue(
		t.Context(),
		"test-queue",
		"test-workflow",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}
	// wait for longer than DeleteAfter duration
	time.Sleep(5 * time.Second)

	// check if the queue itself is removed from redis
	exists, err := q.(*RedisQueue).client.Exists(t.Context(), "test-queue").Result()
	if err != nil {
		t.Fatalf("failed to check queue existence: %v", err)
	}
	if exists != 0 {
		t.Errorf("expected queue to be deleted from Redis")
	}

	// check if the queue is removed from engine's map
	_, err = engine.GetQueue(t.Context(), "test-queue")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}

// TestWorkerLoop tests that one worker can dequeue and process workflows correctly
func TestWorkerLoop(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer func() {
		if err := engine.Close(t.Context()); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	}()

	store, err := NewSQLiteStore("test.db")
	if err != nil {
		t.Fatalf("store error: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	}()

	wfengine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	queue, err := engine.CreateQueue(
		t.Context(),
		"test-worker-queue",
		"test-workflow",
		WorkersDefinition{
			Count:        1,
			PollInterval: 300 * time.Millisecond,
		},
		QueueOptions{
			AutoDelete:  true,
			DeleteAfter: 2 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	// enqueue some workflows
	for i := 0; i < 3; i++ {
		wfName := fmt.Sprintf("test-wf-%d", i)

		registerWf(wfengine, t, wfName)
		workflow, err := wfengine.NewWorkflow(wfName)
		if err != nil {
			t.Fatalf("failed to create workflow: %v", err)
		}

		err = queue.Enqueue(t.Context(), workflow)
		if err != nil {
			t.Fatalf("failed to enqueue workflow: %v", err)
		}
	}

	// wait for processing
	time.Sleep(5 * time.Second)

	// the queue should be empty after workers dequeue everything
	length, err := engine.client.LLen(t.Context(), "test-worker-queue").Result()
	if err != nil {
		t.Fatalf("failed to check queue length: %v", err)
	}
	if length != 0 {
		t.Errorf("expected queue to be empty after processing, got %d items", length)
	}

	time.Sleep(3 * time.Second)

	// queue should now be deleted from redis
	exists, err := engine.client.Exists(t.Context(), "test-worker-queue").Result()
	if err != nil {
		t.Fatalf("failed to check Redis key: %v", err)
	}
	if exists != 0 {
		t.Errorf("expected queue to be deleted from Redis, but it still exists")
	}

	// check that closeCh was closed after auto-deletion
	select {
	case <-queue.(*RedisQueue).closeCh:
		// success
	default:
		t.Errorf("expected closeCh to be closed after auto-delete")
	}
}

// TestWorkerLoopMultiWorkers tests that multiple workers can dequeue and process workflows concurrently
func TestWorkerLoopMultiWorkers(t *testing.T) {
	engine := NewRedisQueueEngine("localhost:6379")
	defer func() {
		if err := engine.Close(t.Context()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	}()

	store, err := NewSQLiteStore("test.db")
	if err != nil {
		t.Fatalf("store error: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	}()

	wfengine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	queue, err := engine.CreateQueue(
		t.Context(),
		"test-worker-queue",
		"test-workflow",
		WorkersDefinition{
			Count:        3,
			PollInterval: 300 * time.Millisecond,
		},
		QueueOptions{
			AutoDelete:  true,
			DeleteAfter: 2 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	// enqueue some workflows
	for i := 0; i < 3; i++ {
		wfName := fmt.Sprintf("test-wf-%d", i)

		registerWf(wfengine, t, wfName)
		workflow, err := wfengine.NewWorkflow(wfName)
		if err != nil {
			t.Fatalf("failed to create workflow: %v", err)
		}

		err = queue.Enqueue(t.Context(), workflow)
		if err != nil {
			t.Fatalf("failed to enqueue workflow: %v", err)
		}
	}

	// wait for processing
	time.Sleep(5 * time.Second)

	// the queue should be empty after workers dequeue everything
	length, err := engine.client.LLen(t.Context(), "test-worker-queue").Result()
	if err != nil {
		t.Fatalf("failed to check queue length: %v", err)
	}
	if length != 0 {
		t.Errorf("expected queue to be empty after processing")
	}

	time.Sleep(3 * time.Second)

	// queue should now be deleted from redis
	exists, err := engine.client.Exists(t.Context(), "test-worker-queue").Result()
	if err != nil {
		t.Fatalf("failed to check Redis key: %v", err)
	}
	if exists != 0 {
		t.Errorf("expected queue to be deleted from Redis, but it still exists")
	}

	// check that closeCh was closed after auto-deletion
	select {
	case <-queue.(*RedisQueue).closeCh:
		// success
	default:
		t.Errorf("expected closeCh to be closed after auto-delete")
	}
}
