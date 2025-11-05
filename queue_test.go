package ewf

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestEnqueueDequeue tests enqueuing and dequeuing workflows from the queue
func TestEnqueueDequeue(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	store, err := NewSQLiteStore("test.db")
	if err != nil {
		t.Fatalf("store error: %v", err)
	}

	idleSince := time.Now()

	queue := NewRedisQueue(
		"test-queue",
		WorkersDefinition{Count: 1, PollInterval: 200 * time.Millisecond},
		QueueOptions{AutoDelete: false},
		client,
		&idleSince,
	)
	t.Cleanup(func() {
		if err := queue.Close(context.Background()); err != nil {
			t.Errorf("failed to close queue: %v", err)
		}

		if err := store.Close(); err != nil {
			t.Errorf("failed to close store: %v", err)
		}
	})

	for i := 0; i < 5; i++ {
		workflow := NewWorkflow(fmt.Sprintf("test-workflow%d", i), WithStore(store))
		err = queue.Enqueue(t.Context(), workflow)
		if err != nil {
			t.Fatalf("failed to enqueue workflow: %v", err)
		}
	}

	len, err := queue.client.LLen(t.Context(), "test-queue").Result()
	if err != nil {
		t.Fatalf("failed to get queue length: %v", err)
	}
	// check queue length == 5
	if len != 5 {
		t.Fatalf("expected queue length 5, got %d", len)
	}

	// dequeue workflows and check they exist and their names (to check the dequeue order)
	for i := 0; i < 5; i++ {
		wf, err := queue.Dequeue(t.Context())
		if err != nil {
			t.Fatalf("failed to dequeue workflow: %v", err)
		}
		if wf == nil {
			t.Fatalf("expected workflow, got nil")
		}
		expectedName := fmt.Sprintf("test-workflow%d", i)
		if wf.Name != expectedName {
			t.Errorf("expected workflow name %s, got %s", expectedName, wf.Name)
		}
	}

	// check queue is empty now
	len, err = queue.client.LLen(t.Context(), "test-queue").Result()
	if err != nil {
		t.Fatalf("failed to get queue length: %v", err)
	}
	if len != 0 {
		t.Errorf("expected queue length 0, got %d", len)
	}
}

// TestClose tests closing the queue and ensuring it is deleted from Redis
func TestClose(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})

	idleSince := time.Now()

	queue := NewRedisQueue(
		"test-queue",
		WorkersDefinition{Count: 1, PollInterval: 200 * time.Millisecond},
		QueueOptions{AutoDelete: false},
		client,
		&idleSince,
	)
	err := queue.Close(t.Context())
	if err != nil {
		t.Fatalf("failed to close queue: %v", err)
	}

	exists, err := queue.client.Exists(t.Context(), "test-queue").Result()
	if err != nil {
		t.Fatalf("failed to check queue existence: %v", err)
	}

	// check that the queue is deleted from redis
	if exists != 0 {
		t.Errorf("expected queue to be deleted, but it still exists")
	}

}
