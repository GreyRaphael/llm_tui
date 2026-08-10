package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// DB wraps the *sql.DB database connection
type DB struct {
	*sql.DB
}

// GetDefaultDBPath calculates the sqlite database file path adjacent to the executable.
// If running in `go run` or if resolution fails, it falls back to current working directory.
func GetDefaultDBPath() string {
	exePath, err := os.Executable()
	if err == nil {
		// Check if running under `go run` temporary directory
		if !strings.Contains(exePath, "go-build") && !strings.Contains(exePath, "/tmp/") {
			dir := filepath.Dir(exePath)
			return filepath.Join(dir, "providers.db")
		}
	}
	cwd, err := os.Getwd()
	if err == nil {
		return filepath.Join(cwd, "providers.db")
	}
	return "providers.db"
}

// InitDB initializes SQLite database at the given path (or default path if empty).
// Supports ":memory:" for in-memory unit testing.
func InitDB(dbPath string) (*DB, error) {
	if dbPath == "" {
		dbPath = GetDefaultDBPath()
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db at %s: %w", dbPath, err)
	}

	// Test connection
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to ping sqlite db: %w", err)
	}

	database := &DB{DB: conn}
	if err := database.migrate(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return database, nil
}

// migrate creates the necessary table schemas if they do not exist
func (d *DB) migrate() error {
	query := `
	CREATE TABLE IF NOT EXISTS provider_records (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		base_url TEXT NOT NULL,
		api_key TEXT NOT NULL,
		api_type TEXT NOT NULL,
		model TEXT NOT NULL,
		reasoning_effort TEXT NOT NULL DEFAULT 'none',
		custom_payload TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := d.Exec(query)
	return err
}
