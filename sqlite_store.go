package ewf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	_ "github.com/mattn/go-sqlite3" // SQLite driver
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(dsn string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}
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
func (s *SQLiteStore) LoadWorkflow(ctx context.Context, uuid string) (*Workflow, error) {
	var data []byte
	q := `SELECT data FROM workflows WHERE uuid = ?`
	err := s.db.QueryRowContext(ctx, q, uuid).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("sqlite store: failed to load workflow %s: %w", uuid, err)
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
	defer rows.Close()

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

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
