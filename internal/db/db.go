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

// isDirWritable tests whether a directory exists and can be written to by creating and removing a test file.
func isDirWritable(dir string) bool {
	if dir == "" {
		return false
	}
	testFile := filepath.Join(dir, fmt.Sprintf(".test_write_%d", os.Getpid()))
	f, err := os.OpenFile(testFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(testFile)
	return true
}

// getUserConfigDBPath returns ~/.config/llm_tui/providers.db (or OS equivalent user config directory).
func getUserConfigDBPath() string {
	if configDir, err := os.UserConfigDir(); err == nil && configDir != "" {
		appDir := filepath.Join(configDir, "llm_tui")
		if err := os.MkdirAll(appDir, 0755); err == nil && isDirWritable(appDir) {
			return filepath.Join(appDir, "providers.db")
		}
	}
	if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
		appDir := filepath.Join(homeDir, ".config", "llm_tui")
		if err := os.MkdirAll(appDir, 0755); err == nil && isDirWritable(appDir) {
			return filepath.Join(appDir, "providers.db")
		}
	}
	return ""
}

// GetDefaultDBPath calculates the sqlite database file path.
// It prioritizes placing `providers.db` adjacent to the executable if the directory is writable.
// If running under `go run` (detected by temp directory heuristics) or if the executable directory
// is read-only (e.g. installed to /usr/local/bin), it falls back to user config directory
// (~/.config/llm_tui/providers.db) or current working directory.
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
			if isDirWritable(dir) {
				return filepath.Join(dir, "providers.db")
			}
			// Executable directory is read-only (e.g. /usr/local/bin); fallback to user config directory
			if userConfigPath := getUserConfigDBPath(); userConfigPath != "" {
				return userConfigPath
			}
		}
	}

	// For `go run` or if executable resolution fails, try current working directory
	cwd, err := os.Getwd()
	if err == nil && isDirWritable(cwd) {
		return filepath.Join(cwd, "providers.db")
	}

	// Fallback to user config directory
	if userConfigPath := getUserConfigDBPath(); userConfigPath != "" {
		return userConfigPath
	}

	if cwd != "" {
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

	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			_ = os.MkdirAll(dir, 0755)
		}
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
	if err := d.migrateAPIKeyNullable(); err != nil {
		return err
	}
	return d.migrateAliasNames()
}

// migrateAliasNames drops the redundant ` (api_type)` suffix that older versions
// appended to auto-generated aliases (e.g. "gpt-4o (openai_chat)" -> "gpt-4o").
func (d *DB) migrateAliasNames() error {
	_, err := d.Exec(`
		UPDATE provider_records
		SET name = TRIM(SUBSTR(name, 1, LENGTH(name) - LENGTH(api_type) - 3))
		WHERE name LIKE '% (' || api_type || ')'
	`)
	return err
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
