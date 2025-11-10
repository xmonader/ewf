package ewf

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/redis/go-redis/v9"
)

// checkQueueDeleted checks that queue is deleted from redis, queue engine map, channel is closed
func checkQueueDeleted(t *testing.T, engine QueueEngine, queueName string) {
	t.Helper()

	qEngine := engine.(*redisQueueEngine)
	// queue should now be deleted from redis
	exists, err := qEngine.client.Exists(t.Context(), queueName).Result()
	if err != nil {
		t.Fatalf("failed to check Redis key: %v", err)
	}
	if exists != 0 {
		t.Errorf("expected queue to be deleted from Redis, but it still exists")
	}

	// try to get the closed queue
	q, err := qEngine.GetQueue(t.Context(), queueName)
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
	if q != nil {
		t.Errorf("expected queue to be nil after deletion")
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
		"testQueue",
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
	)
	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	if engine.(*redisQueueEngine).queues["testQueue"] == nil {
		t.Errorf("expected queue to be created")
	}

	// get the created queue
	gotQueue, err := engine.GetQueue(t.Context(), "testQueue")
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
		"testQueue",
		QueueOptions{AutoDelete: false, DeleteAfter: 10 * time.Minute},
	)

	if err != nil {
		t.Fatalf("failed to create queue: %v", err)
	}

	err = engine.CloseQueue(t.Context(), q.Name())
	if err != nil {
		t.Fatalf("failed to close queue: %v", err)
	}

	checkQueueDeleted(t, engine, q.Name())

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
			fmt.Sprintf("testQueue%d", i),
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

	if len(engine.(*redisQueueEngine).queues) != 0 {
		t.Errorf("expected all queues to be closed")
	}

	// try to get a queue after engine is closed
	_, err = engine.GetQueue(t.Context(), "testQueue0")
	if err != ErrQueueNotFound {
		t.Errorf(" expected err to be equal %v", ErrQueueNotFound)
	}
}

// TestAutoDelete tests creating a queue with AutoDelete option, waits for time larger than DeleteAfter duration, and checks if the queue is deleted from redis and engine map
func TestAutoDelete(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
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

		name := "testQueue"

		err = wfengine.CreateQueue(
			t.Context(),
			name,
			WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
			QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
		)
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}

		// wait for longer than DeleteAfter duration
		time.Sleep(5 * time.Second)

		checkQueueDeleted(t, qEngine, name)
	})
}

// TestAutoDeleteMultipleQueues tests creating a queue with AutoDelete option, waits for time larger than DeleteAfter duration, and checks if the queue is deleted from redis and engine map for 3 queues
func TestAutoDeleteMultipleQueues(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {

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

		var createdQueues []string
		for i := 0; i < 3; i++ {
			name := fmt.Sprintf("testQueue%d", i)
			err := wfengine.CreateQueue(
				t.Context(),
				name,
				WorkersDefinition{Count: 1, PollInterval: 1 * time.Second},
				QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
			)
			if err != nil {
				t.Fatalf("failed to create queue: %v", err)
			}
			createdQueues = append(createdQueues, name)
		}

		// wait for longer than DeleteAfter duration
		time.Sleep(5 * time.Second)

		for _, q := range createdQueues {
			checkQueueDeleted(t, qEngine, q)
		}
	})
}

// TestWorkerLoop tests that one worker can dequeue and process workflows correctly
func TestWorkerLoop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {

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

		name := "testWorkerQueue"

		err = wfengine.CreateQueue(
			t.Context(),
			name,
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

		queue, err := qEngine.GetQueue(t.Context(), name)
		if err != nil {
			t.Fatalf("failed to get queue: %v", err)
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
		length, err := queue.Length(t.Context())
		if err != nil {
			t.Fatalf("failed to check queue length: %v", err)
		}
		if length != 0 {
			t.Errorf("expected queue to be empty after processing, got %d items", length)
		}

		time.Sleep(3 * time.Second)

		checkQueueDeleted(t, qEngine, queue.Name())
	})
}

// TestWorkerLoopMultiWorkers tests that multiple workers can dequeue and process workflows concurrently
func TestWorkerLoopMultiWorkers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {

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

		name := "testWorkerQueue"
		err = wfengine.CreateQueue(
			t.Context(),
			name,
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

		queue, err := qEngine.GetQueue(t.Context(), name)
		if err != nil {
			t.Fatalf("failed to get queue: %v", err)
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
		length, err := queue.Length(t.Context())
		if err != nil {
			t.Fatalf("failed to check queue length: %v", err)
		}
		if length != 0 {
			t.Errorf("expected queue to be empty after processing")
		}

		time.Sleep(3 * time.Second)

		checkQueueDeleted(t, qEngine, queue.Name())
	})
}

func TestEnqueueIdleTimeReset(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {

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

		name := "testQueue"
		// now idleSince is initialized to time.now
		err = wfengine.CreateQueue(
			t.Context(),
			name,
			WorkersDefinition{Count: 1, PollInterval: 500 * time.Millisecond},
			QueueOptions{AutoDelete: true, DeleteAfter: 2 * time.Second},
		)
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}

		q, err := qEngine.GetQueue(t.Context(), name)
		if err != nil {
			t.Fatalf("failed to get queue: %v", err)
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

		checkQueueDeleted(t, qEngine, q.Name())
	})
}

// TestStoreActions tests storing, auto-deleting a queue from Store.
func TestStoreActions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dbFile := "./test.db"

		store, err := NewSQLiteStore(dbFile)
		if err != nil {
			t.Fatalf("NewSQLiteStore() error = %v", err)
		}
		t.Cleanup(func() {
			if err := os.Remove(dbFile); err != nil {
				t.Fatalf("failed to remove dbFile: %v", err)
			}

			if err := store.Close(); err != nil {
				t.Fatalf("failed to close store: %v", err)
			}
		})

		// create queue engine
		client := redis.NewClient(
			&redis.Options{Addr: "localhost:6379"},
		)
		qEngine := NewRedisQueueEngine(client)
		t.Cleanup(func() {
			if err := qEngine.Close(context.Background()); err != nil {
				t.Errorf("failed to close engine: %v", err)
			}
		})

		// Create an engine with the store, queue engine
		engine, err := NewEngine(Withstore(store), WithQueueEngine(qEngine))
		if err != nil {
			t.Fatalf("failed to create engine: %v", err)
		}

		name := "storeTestQueue"
		workersDef := WorkersDefinition{Count: 2, PollInterval: 1 * time.Second}
		queueOpts := QueueOptions{AutoDelete: true, PopTimeout: 1 * time.Second, DeleteAfter: 1 * time.Second}

		// create queue
		err = engine.CreateQueue(t.Context(), name, workersDef, queueOpts)
		if err != nil {
			t.Fatalf("failed to create queue: %v", err)
		}

		q, err := qEngine.GetQueue(t.Context(), name)
		if err != nil {
			t.Fatalf("failed to get queue: %v", err)
		}

		// check queue is saved to store
		queues, err := store.LoadAllQueueMetadata(context.Background())
		if err != nil {
			t.Fatalf("loadAll() error = %v", err)
		}

		expected := &QueueMetadata{
			Name:         name,
			WorkersDef:   workersDef,
			QueueOptions: queueOpts,
		}

		if len(queues) != 1 {
			t.Errorf("Expected queues len to be 1, got %d", len(queues))
		}

		if !reflect.DeepEqual(queues[0], expected) {
			t.Errorf("expected stored queue to be %v, got %v", expected, queues[0])
		}

		// do some activity
		err = q.Enqueue(t.Context(), NewWorkflow("name"))
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		_, err = q.Dequeue(t.Context())
		if err != nil {
			t.Fatalf("failed to enqueue: %v", err)
		}

		// sleep to auto-delete
		time.Sleep(2 * time.Second)

		queues, err = store.LoadAllQueueMetadata(context.Background())
		if err != nil {
			t.Fatalf("loadAll() error = %v", err)
		}

		if len(queues) != 0 {
			t.Errorf("Expected queues len to be 0, got %d", len(queues))
		}
	})
}
