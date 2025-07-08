package ewf

import (
	"context"
	"os"
	"testing"
)

func TestSQLiteStore_SaveAndLoad(t *testing.T) {
	dbFile := "./test.db"
	defer os.Remove(dbFile)

	store, err := NewSQLiteStore(dbFile)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()
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

	loadedWf, err := store.LoadWorkflow(context.Background(), wf.UUID)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
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
	defer os.Remove(dbFile)

	store, err := NewSQLiteStore(dbFile)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	_, err = store.LoadWorkflow(context.Background(), "non-existent-id")
	if err == nil {
		t.Fatal("Expected an error when loading a non-existent workflow, but got nil")
	}
}
