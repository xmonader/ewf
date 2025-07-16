package ewf

import (
	"context"
	"os"
	"testing"
)

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
