package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
	"github.com/xmonader/ewf"
)

// App holds the application's dependencies.
type App struct {
	engine *ewf.Engine
	router *mux.Router
	logger *log.Logger
}

// NewApp creates and initializes a new application.
func NewApp(logger *log.Logger) (*App, error) {
	store, err := ewf.NewSQLiteStore("httpexample.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create sqlite store: %v", err)
	}
	engine, err := ewf.NewEngine(store)
	if err != nil {
		return nil, fmt.Errorf("failed to create engine: %v", err)
	}

	app := &App{
		engine: engine,
		router: mux.NewRouter(),
		logger: logger,
	}

	app.registerActivities()
	app.registerWorkflowTemplates()
	app.setupRoutes()

	return app, nil
}

// Run starts the application.
func (a *App) Run() error {
	// Resume any pending workflows in the background.
	a.engine.ResumeRunningWorkflows()

	a.logger.Println("Starting server on :8090")
	return http.ListenAndServe(":8090", a.router)
}

// registerActivities registers the workflow steps with the engine.
func (a *App) registerActivities() {
	a.engine.Register("sleep_5", waitStep(5*time.Second))
	a.engine.Register("sleep_10", waitStep(10*time.Second))
	a.engine.Register("sleep_15", waitStep(15*time.Second))
	a.engine.Register("create_file", createFileStep)
}

// registerWorkflowTemplates defines and registers the workflow templates.
func (a *App) registerWorkflowTemplates() {
	greetingTemplate := &ewf.WorkflowTemplate{
		Steps: []ewf.Step{
			{Name: "sleep_5"},
			{Name: "sleep_10"},
			{Name: "sleep_15"},
			{Name: "create_file"},
		},
	}
	a.engine.RegisterTemplate("greeting-workflow", greetingTemplate)
}

// setupRoutes configures the HTTP routes.
func (a *App) setupRoutes() {
	a.router.HandleFunc("/greet/{name}", a.greetHandler)
	a.router.HandleFunc("/status/{uuid}", a.statusHandler)
}

// greetHandler starts a new greeting workflow.
func (a *App) greetHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	name := vars["name"]

	wf, err := a.engine.NewWorkflow("greeting-workflow")
	if err != nil {
		jsonError(w, fmt.Sprintf("failed to create workflow: %v", err), http.StatusInternalServerError)
		return
	}

	// Set initial state.
	wf.State["name"] = name

	// Asynchronous execution.
	a.logger.Printf("Starting workflow %s asynchronously for name %s", wf.UUID, name)
	a.engine.RunAsync(context.Background(), wf)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"workflow_id": wf.UUID,
		"status_url":  fmt.Sprintf("/status/%s", wf.UUID),
	})
}

// statusHandler checks the status of a workflow.
func (a *App) statusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid := vars["uuid"]

	wf, err := a.engine.Store().LoadWorkflow(r.Context(), uuid)
	if err != nil {
		if err == ewf.ErrWorkflowNotFound {
			jsonError(w, fmt.Sprintf("workflow with id %s not found", uuid), http.StatusNotFound)
		} else {
			jsonError(w, fmt.Sprintf("failed to load workflow: %v", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(wf)
}

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags)

	app, err := NewApp(logger)
	if err != nil {
		logger.Fatalf("failed to create app: %v", err)
	}

	if err := app.Run(); err != nil {
		logger.Fatalf("failed to run app: %v", err)
	}
}

// waitStep is an activity that waits for a given duration.
func waitStep(duration time.Duration) ewf.StepFn {
	return func(ctx context.Context, state ewf.State) error {
		log.Printf("Waiting for %s...", duration)
		time.Sleep(duration)
		return nil
	}
}

// createFileStep is an activity that creates a file with the name from the workflow state.
func createFileStep(ctx context.Context, state ewf.State) error {
	name, ok := state["name"].(string)
	if !ok {
		return fmt.Errorf("name not found in workflow state")
	}


	// Use a filesystem-friendly timestamp for the filename.
	filename := fmt.Sprintf("hello-%s.txt", time.Now().Format("20060102-150405"))
	filepath := filepath.Join(os.TempDir(), filename)

	log.Printf("Creating file at %s with content: %s", filepath, name)

	if err := os.WriteFile(filepath, []byte(name), 0644); err != nil {
		log.Printf("ERROR: failed to create file for workflow: %v", err)
		return err
	}

	return nil
}

// jsonError is a helper to write a JSON error message.
func jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
