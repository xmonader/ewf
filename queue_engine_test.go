package ewf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// checkQueueDeleted checks that queue is deleted from redis, queue engine map, channel is closed
func checkQueueDeleted(t *testing.T, qEngine *RedisQueueEngine, q *RedisQueue) {
	t.Helper()

	// queue should now be deleted from redis
	exists, err := qEngine.client.Exists(t.Context(), q.Name()).Result()
	if err != nil {
		t.Fatalf("failed to check Redis key: %v", err)
	}
	if exists != 0 {
		t.Errorf("expected queue to be deleted from Redis, but it still exists")
	}

	// try to get the closed queue
	_, err = qEngine.GetQueue(t.Context(), "test-queue")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}

	// check that closeCh was closed after deletion
	select {
	case <-q.closeCh:
		// success
	default:
		t.Errorf("expected closeCh to be closed after delete")
	}
}

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
	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	engine := NewRedisQueueEngine(client)
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	})

	q, err := engine.CreateQueue(
		t.Context(),
		"test-queue",
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
	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	engine := NewRedisQueueEngine(client)
	t.Cleanup(func() {
		if err := engine.Close(context.Background()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	})

	q, err := engine.CreateQueue(
		t.Context(),
		"test-queue",
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

	checkQueueDeleted(t, engine, q.(*RedisQueue))

	// close a non existing queue
	err = engine.CloseQueue(t.Context(), "test")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}

// TestCloseEngine tests closing the engine with multiple queues and ensures all queues are closed, and tries to get a queue after engine is closed
func TestCloseEngine(t *testing.T) {
	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	engine := NewRedisQueueEngine(client)

	for i := 0; i < 3; i++ {
		_, err := engine.CreateQueue(
			t.Context(),
			fmt.Sprintf("test-queue-%d", i),
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
	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	qEngine := NewRedisQueueEngine(client)

	t.Cleanup(func() {
		if err := qEngine.Close(context.Background()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	})

	wfengine, err := NewEngine(WithQueueEngine(qEngine))
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	q, err := wfengine.CreateQueue(
		t.Context(),
		"test-queue",
		WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
		QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	// wait for longer than DeleteAfter duration
	time.Sleep(5 * time.Second)

	checkQueueDeleted(t, qEngine, q.(*RedisQueue))
}

// TestAutoDeleteMultipleQueues tests creating a queue with AutoDelete option, waits for time larger than DeleteAfter duration, and checks if the queue is deleted from redis and engine map for 3 queues
func TestAutoDeleteMultipleQueues(t *testing.T) {
	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	qEngine := NewRedisQueueEngine(client)

	t.Cleanup(func() {
		if err := qEngine.Close(context.Background()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	})

	wfengine, err := NewEngine(WithQueueEngine(qEngine))
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	var createdQueues []Queue
	for i := 0; i < 3; i++ {
		q, err := wfengine.CreateQueue(
			t.Context(),
			fmt.Sprintf("test-queue_%d", i),
			WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
			QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
		)
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}
		createdQueues = append(createdQueues, q)
	}

	// wait for longer than DeleteAfter duration
	time.Sleep(5 * time.Second)

	for _, q := range createdQueues {
		checkQueueDeleted(t, qEngine, q.(*RedisQueue))
	}

}

// TestWorkerLoop tests that one worker can dequeue and process workflows correctly
func TestWorkerLoop(t *testing.T) {

	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	qEngine := NewRedisQueueEngine(client)

	t.Cleanup(func() {
		if err := qEngine.Close(context.Background()); err != nil {
			t.Errorf("failed to close queue engine: %v", err)
		}
	})

	wfengine, err := NewEngine(WithQueueEngine(qEngine))
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	queue, err := wfengine.CreateQueue(
		t.Context(),
		"test-worker-queue",
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
	length, err := qEngine.client.LLen(t.Context(), "test-worker-queue").Result()
	if err != nil {
		t.Fatalf("failed to check queue length: %v", err)
	}
	if length != 0 {
		t.Errorf("expected queue to be empty after processing, got %d items", length)
	}

	time.Sleep(3 * time.Second)

	checkQueueDeleted(t, qEngine, queue.(*RedisQueue))
}

// TestWorkerLoopMultiWorkers tests that multiple workers can dequeue and process workflows concurrently
func TestWorkerLoopMultiWorkers(t *testing.T) {
	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)
	qEngine := NewRedisQueueEngine(client)

	t.Cleanup(func() {
		if err := qEngine.Close(context.Background()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	})

	wfengine, err := NewEngine(WithQueueEngine(qEngine))
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	queue, err := wfengine.CreateQueue(
		t.Context(),
		"test-worker-queue",
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
	length, err := qEngine.client.LLen(t.Context(), "test-worker-queue").Result()
	if err != nil {
		t.Fatalf("failed to check queue length: %v", err)
	}
	if length != 0 {
		t.Errorf("expected queue to be empty after processing")
	}

	time.Sleep(3 * time.Second)

	checkQueueDeleted(t, qEngine, queue.(*RedisQueue))
}

func TestEnqueueIdleTimeReset(t *testing.T) {
	client := redis.NewClient(
		&redis.Options{Addr: "localhost:6379"},
	)

	store, err := NewSQLiteStore("test.db")
	if err != nil {
		t.Fatalf("store error: %v", err)
	}

	qEngine := NewRedisQueueEngine(client)

	t.Cleanup(func() {
		if err := qEngine.Close(context.Background()); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}

		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	wfengine, err := NewEngine(WithQueueEngine(qEngine))
	if err != nil {
		t.Fatalf("wf engine error: %v", err)
	}

	// now idleSince is initialized to time.now
	q, err := wfengine.CreateQueue(
		t.Context(),
		"test-queue",
		WorkersDefinition{Count: 1, PollInterval: 500 * time.Millisecond},
		QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	// sleep for 1s, time since idleSince should be 1s now
	time.Sleep(1 * time.Second)

	workflow := NewWorkflow("test-workflow", WithStore(store))

	// now time since idleSince should be reset to 0 on enqueue
	err = q.Enqueue(t.Context(), workflow)
	if err != nil {
		t.Fatalf("failed to enqueue workflow: %v", err)
	}

	// Dequeue it immediately to simulate quick processing
	_, err = q.Dequeue(t.Context())
	if err != nil {
		t.Fatalf("failed to dequeue workflow: %v", err)
	}

	// wait one more second --> less than DeleteAfter since enqueue
	time.Sleep(1 * time.Second)

	_, err = qEngine.GetQueue(t.Context(), q.Name())
	if err != nil {
		t.Errorf(" expected err to be nil because queue still exists, got %v", err)
	}

	time.Sleep(3 * time.Second)

	checkQueueDeleted(t, qEngine, q.(*RedisQueue))
}
