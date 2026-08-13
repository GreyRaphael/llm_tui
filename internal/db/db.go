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
// If running under `go run` (detected by temp directory heuristics) or if resolution fails,
// it falls back to current working directory.
func GetDefaultDBPath() string {
	exePath, err := os.Executable()
	if err == nil {
		// Resolve symlinks for accurate path detection
		if resolved, resolveErr := filepath.EvalSymlinks(exePath); resolveErr == nil {
			exePath = resolved
		}
		// Heuristic: detect temporary directories used by `go run`.
		// On Linux/macOS, `go run` compiles to a path containing "go-build".
		// On Windows, the compiled binary lives under os.TempDir().
		tempDir := os.TempDir()
		if !strings.Contains(exePath, "go-build") && !strings.HasPrefix(exePath, tempDir) {
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
		api_key TEXT,
		api_type TEXT NOT NULL,
		model TEXT NOT NULL,
		reasoning_effort TEXT NOT NULL DEFAULT 'none',
		custom_payload TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	`
	if _, err := d.Exec(query); err != nil {
		return err
	}
	return d.migrateAPIKeyNullable()
}

// migrateAPIKeyNullable relaxes the api_key column on databases created before
// empty API keys were supported (older schema used api_key TEXT NOT NULL).
func (d *DB) migrateAPIKeyNullable() error {
	rows, err := d.Query("PRAGMA table_info(provider_records)")
	if err != nil {
		return err
	}
	defer rows.Close()

	apiKeyNotNull := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dfltValue interface{}
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == "api_key" && notNull == 1 {
			apiKeyNotNull = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !apiKeyNotNull {
		return nil
	}

	// SQLite cannot drop a NOT NULL constraint in place; rebuild the table.
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	rebuild := []string{
		`CREATE TABLE provider_records_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT,
			api_type TEXT NOT NULL,
			model TEXT NOT NULL,
			reasoning_effort TEXT NOT NULL DEFAULT 'none',
			custom_payload TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`INSERT INTO provider_records_new (id, name, base_url, api_key, api_type, model, reasoning_effort, custom_payload, created_at, updated_at)
			SELECT id, name, base_url, api_key, api_type, model, reasoning_effort, custom_payload, created_at, updated_at FROM provider_records`,
		`DROP TABLE provider_records`,
		`ALTER TABLE provider_records_new RENAME TO provider_records`,
	}
	for _, stmt := range rebuild {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}
