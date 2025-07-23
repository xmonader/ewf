package ewf

import (
	"os"
	"testing"
)

func TestE2E_DynamicTemplatePersistenceAndRecovery(t *testing.T) {
	if err := os.Remove("e2e_dyn_template.db"); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove db: %v", err)
	}
	store, err := NewSQLiteStore("e2e_dyn_template.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("failed to close store: %v", err)
		}
	}()
	defer func() {
		if err := os.Remove("e2e_dyn_template.db"); err != nil && !os.IsNotExist(err) {
			t.Fatalf("failed to remove db: %v", err)
		}
	}()

	engine, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Simulate dynamic creation
	name := "deployk8s-5"
	tmpl := &WorkflowTemplate{Steps: []Step{}}
	for i := 0; i < 5; i++ {
		tmpl.Steps = append(tmpl.Steps, Step{Name: "deploy-node"})
	}
	engine.RegisterTemplate(name, tmpl)

	// Simulate crash: create new engine instance
	engine2, err := NewEngine(store)
	if err != nil {
		t.Fatalf("failed to create engine2: %v", err)
	}
	// Should have template loaded
	wf, err := engine2.NewWorkflow(name)
	if err != nil {
		t.Fatalf("engine2 failed to create workflow from persisted template: %v", err)
	}
	if len(wf.Steps) != 5 {
		t.Fatalf("engine2 loaded workflow has wrong step count: %d", len(wf.Steps))
	}
}
