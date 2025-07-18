package ewf

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)


// SQLiteStore implements the Store interface using SQLite for persistence.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLiteStore with the given DSN.
func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Setup prepares the SQLite database for use.
func (s *SQLiteStore) Setup() error {
	return s.prepWorkflowTable()
}

func (s *SQLiteStore) prepWorkflowTable() error {
	q := `
		CREATE TABLE IF NOT EXISTS workflows (
			uuid TEXT NOT NULL PRIMARY KEY UNIQUE,
			name TEXT NOT NULL,
			status TEXT NOT NULL,
			data BLOB NOT NULL
		);
	`
	_, err := s.db.Exec(q)
	if err != nil {
		return fmt.Errorf("failed to create workflows table: %w", err)
	}
	return nil
}

// SaveWorkflow saves the given workflow to the SQLite database.
func (s *SQLiteStore) SaveWorkflow(ctx context.Context, workflow *Workflow) error {
	data, err := json.Marshal(workflow)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow: %w", err)
	}

	query := `INSERT OR REPLACE INTO workflows (uuid, name, status, data) VALUES (?, ?, ?, ?)`
	_, err = s.db.ExecContext(ctx, query, workflow.UUID, workflow.Name, workflow.Status, data)
	if err != nil {
		return fmt.Errorf("sqlite store: failed to save workflow %s: %w", workflow.Name, err)
	}
	return nil
}

// ErrWorkflowNotFound is returned when a workflow is not found in the database.
var ErrWorkflowNotFound = errors.New("workflow not found")

func (s *SQLiteStore) LoadWorkflowByUUID(ctx context.Context, uuid string) (*Workflow, error) {
	var data []byte
	q := `SELECT data FROM workflows WHERE uuid = ?`
	err := s.db.QueryRowContext(ctx, q, uuid).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("sqlite store: failed to load workflow by UUID %s: %w", uuid, err)
	}

	var workflow Workflow
	err = json.Unmarshal(data, &workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}
	return &workflow, nil
}

func (s *SQLiteStore) LoadWorkflowByName(ctx context.Context, name string) (*Workflow, error) {
	var data []byte
	// Use the dedicated name column instead of JSON extraction
	q := `SELECT data FROM workflows WHERE name = ?`
	err := s.db.QueryRowContext(ctx, q, name).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrWorkflowNotFound
		}
		return nil, fmt.Errorf("sqlite store: failed to load workflow by name %s: %w", name, err)
	}

	var workflow Workflow
	err = json.Unmarshal(data, &workflow)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow: %w", err)
	}
	return &workflow, nil
}

func (s *SQLiteStore) ListWorkflowUUIDsByStatus(ctx context.Context, status WorkflowStatus) ([]string, error) {
	var ids []string
	q := `SELECT uuid FROM workflows WHERE status = ?`
	rows, err := s.db.QueryContext(ctx, q, status)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: failed to list workflow IDs by status %s: %w", status, err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("failed to close rows: %v", err)
		}
	}()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sqlite store: failed to scan workflow ID: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite store: failed to iterate over workflow IDs: %w", err)
	}
	return ids, nil
}

// Close closes the SQLite database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
