package ewf

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"testing/synctest"
	"time"
)

// TestIntegration_TemplatePersistence tests template persistence via SQLiteStore.
func TestIntegration_TemplatePersistence(t *testing.T) {
	if err := os.Remove("integration_templates.db"); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove db: %v", err)
	}
	store, err := NewSQLiteStore("integration_templates.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	}()
	defer func() {
		if err := os.Remove("integration_templates.db"); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove db: %v", err)
		}
	}()

	if err := store.Setup(); err != nil {
		t.Fatalf("failed to setup store: %v", err)
	}

	ctx := context.Background()

	tmpl := &WorkflowTemplate{
		Steps: []Step{{Name: "node-deploy-1"}, {Name: "node-deploy-2"}},
	}
	if err := store.SaveWorkflowTemplate(ctx, "deployk8s-2", tmpl); err != nil {
		t.Fatalf("failed to save template: %v", err)
	}

	loaded, err := store.LoadWorkflowTemplate(ctx, "deployk8s-2")
	if err != nil {
		t.Fatalf("failed to load template: %v", err)
	}
	if len(loaded.Steps) != 2 || loaded.Steps[0].Name != "node-deploy-1" || loaded.Steps[1].Name != "node-deploy-2" {
		t.Fatalf("loaded template mismatch: %+v", loaded)
	}

	all, err := store.LoadAllWorkflowTemplates(ctx)
	if err != nil {
		t.Fatalf("failed to load all templates: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 template, got %d", len(all))
	}
	if _, ok := all["deployk8s-2"]; !ok {
		t.Fatalf("deployk8s-2 not found in all templates")
	}
}

// TestSQLiteStore_SaveAndLoad tests saving and loading a workflow in SQLiteStore.
func TestSQLiteStore_SaveAndLoad(t *testing.T) {
	dbFile := "./test.db"
	defer func() {
		if err := os.Remove(dbFile); err != nil {
			t.Fatalf("failed to remove dbFile: %v", err)
		}
	}()
	store, err := NewSQLiteStore(dbFile)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	}()
	if err := store.Setup(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	wfName := "test-sqlite-workflow"
	wf := NewWorkflow(wfName)
	wf.Steps = []Step{{Name: "dummy_activity"}}
	wf.State["key"] = "value"
	wf.CurrentStep = 2
	wf.Status = StatusCompleted

	err = store.SaveWorkflow(context.Background(), wf)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	loadedWf, err := store.LoadWorkflowByUUID(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("LoadWorkflowByUUID() error = %v", err)
	}

	// Also test loading by name
	loadedByName, err := store.LoadWorkflowByName(context.Background(), wf.Name)
	if err != nil {
		t.Fatalf("LoadWorkflowByName() error = %v", err)
	}

	if loadedByName.UUID != wf.UUID {
		t.Errorf("Expected workflow UUID %s, got %s", wf.UUID, loadedByName.UUID)
	}

	if loadedWf.Name != wfName {
		t.Errorf("Expected workflow ID %s, got %s", wfName, loadedWf.Name)
	}
	if loadedWf.CurrentStep != 2 {
		t.Errorf("Expected CurrentStep to be 2, got %d", loadedWf.CurrentStep)
	}
	if loadedWf.Status != StatusCompleted {
		t.Errorf("Expected Status to be COMPLETED, got %s", loadedWf.Status)
	}
	if loadedWf.State["key"] != "value" {
		t.Errorf("Expected state['key'] to be 'value', got '%v'", loadedWf.State["key"])
	}
}

func TestSQLiteStore_LoadNotFound(t *testing.T) {
	dbFile := "./test_not_found.db"
	defer func() {
		if err := os.Remove(dbFile); err != nil {
			t.Fatalf("failed to remove dbFile: %v", err)
		}
	}()

	store, err := NewSQLiteStore(dbFile)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	}()

	// Test LoadWorkflowByUUID with non-existent UUID
	_, err = store.LoadWorkflowByUUID(context.Background(), "non-existent-id")
	if err == nil {
		t.Fatal("Expected an error when loading a non-existent workflow by UUID, but got nil")
	}

	// Test LoadWorkflowByName with non-existent name
	_, err = store.LoadWorkflowByName(context.Background(), "non-existent-name")
	if err == nil {
		t.Fatal("Expected an error when loading a non-existent workflow by name, but got nil")
	}
}

// TestSQLiteStore_SaveAndLoadQueues tests saving and loading of queues in SQLiteStore.
func TestSQLiteStore_SaveAndLoadQueues(t *testing.T) {
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

	if err := store.Setup(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	for i := 0; i < 4; i++ {
		name := fmt.Sprintf("store_test_queue_%d", i)
		workersDef := WorkersDefinition{Count: 2, PollInterval: 1 * time.Second}
		queueOpts := QueueOptions{AutoDelete: false, PopTimeout: 1 * time.Second}

		queueMetaData := &QueueMetadata{
			Name:         name,
			WorkersDef:   workersDef,
			QueueOptions: queueOpts,
		}

		err = store.SaveQueueMetadata(context.Background(), queueMetaData)
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}
	}

	queues, err := store.LoadAllQueueMetadata(context.Background())
	if err != nil {
		t.Fatalf("LoadAllQueueMetadata() error = %v", err)
	}

	if len(queues) != 4 {
		t.Errorf("Expected queues len to be 4, got %d", len(queues))
	}

	for i, q := range queues {
		expectedName := fmt.Sprintf("store_test_queue_%d", i)
		expectedWorkersDef := WorkersDefinition{Count: 2, PollInterval: 1 * time.Second}
		expectedQueueOpts := QueueOptions{AutoDelete: false, PopTimeout: 1 * time.Second}

		if q.Name != expectedName {
			t.Errorf("Expected queue name %s, got %s", q.Name, expectedName)
		}
		if !reflect.DeepEqual(q.WorkersDef, expectedWorkersDef) {
			t.Errorf("Expected workersDef to be %v, got %v", expectedWorkersDef, q.WorkersDef)
		}
		if !reflect.DeepEqual(q.QueueOptions, expectedQueueOpts) {
			t.Errorf("Expected queueOpts to be %v, got %v", expectedQueueOpts, q.QueueOptions)
		}
	}

}

// TestSQLiteStore_DeleteQueue tests deleting a queue from SQLiteStore.
func TestSQLiteStore_DeleteQueue(t *testing.T) {
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

		if err := store.Setup(); err != nil {
			t.Fatalf("Prepare() error = %v", err)
		}

		name := "store_test_queue"
		workersDef := WorkersDefinition{Count: 2, PollInterval: 1 * time.Second}
		queueOpts := QueueOptions{AutoDelete: false, PopTimeout: 1 * time.Second}

		queueMetaData := &QueueMetadata{
			Name:         name,
			WorkersDef:   workersDef,
			QueueOptions: queueOpts,
		}

		err = store.SaveQueueMetadata(context.Background(), queueMetaData)
		if err != nil {
			t.Fatalf("Save() error = %v", err)
		}

		err = store.DeleteQueueMetadata(context.Background(), queueMetaData.Name)
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		queues, err := store.LoadAllQueueMetadata(context.Background())
		if err != nil {
			t.Fatalf("loadAll() error = %v", err)
		}

		if len(queues) != 0 {
			t.Errorf("Expected queues len to be 0, got %d", len(queues))
		}
	})
}
