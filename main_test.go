package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"llm_tui/internal/db"
)

func TestPrintVersionIfRequested(t *testing.T) {
	expect := "llm_tui version " + Version + "\n"

	tests := []struct {
		name     string
		args     []string
		wantStop bool
		wantOut  string
	}{
		{name: "no args", args: nil, wantStop: false},
		{name: "unrelated arg", args: []string{"--help"}, wantStop: false},
		{name: "short flag", args: []string{"-v"}, wantStop: true, wantOut: expect},
		{name: "long flag", args: []string{"--version"}, wantStop: true, wantOut: expect},
		{name: "version word", args: []string{"version"}, wantStop: true, wantOut: expect},
		{name: "single dash word", args: []string{"-version"}, wantStop: true, wantOut: expect},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if got := printVersionIfRequested(tt.args, &out); got != tt.wantStop {
				t.Fatalf("printVersionIfRequested(%v) = %v, want %v", tt.args, got, tt.wantStop)
			}
			if got := out.String(); got != tt.wantOut {
				t.Fatalf("unexpected version output %q, want %q", got, tt.wantOut)
			}
		})
	}
}

func TestVersionIsReleaseTag(t *testing.T) {
	if len(Version) < 2 || Version[0] != 'v' {
		t.Fatalf("Version must be a v-prefixed release tag, got %q", Version)
	}
	if strings.ContainsAny(Version, " \t\n") {
		t.Fatalf("Version must not contain whitespace, got %q", Version)
	}
}

func TestRunStartsProgramAndReturnsZero(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "providers.db")

	var startedDB *db.DB
	var startedVersion string
	exitCode := run(nil, &bytes.Buffer{}, &bytes.Buffer{}, dbPath, func(database *db.DB, version string) error {
		startedDB = database
		startedVersion = version
		// The database handed to the UI must be usable.
		if _, err := database.ListRecords(); err != nil {
			t.Fatalf("database passed to the UI is unusable: %v", err)
		}
		return nil
	})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if startedDB == nil {
		t.Fatal("expected the UI to be started with a database")
	}
	if startedVersion != Version {
		t.Fatalf("expected version %q, got %q", Version, startedVersion)
	}
}

func TestRunSkipsStartupForVersionRequest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "providers.db")
	var out bytes.Buffer
	started := false

	exitCode := run([]string{"--version"}, &out, &bytes.Buffer{}, dbPath, func(*db.DB, string) error {
		started = true
		return nil
	})

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if started {
		t.Fatal("version request must not start the UI")
	}
	if !strings.Contains(out.String(), Version) {
		t.Fatalf("expected version output, got %q", out.String())
	}
}

func TestRunReportsDatabaseFailure(t *testing.T) {
	// A directory path cannot be opened as a SQLite database.
	dbPath := t.TempDir()
	var errOut bytes.Buffer
	started := false

	exitCode := run(nil, &bytes.Buffer{}, &errOut, dbPath, func(*db.DB, string) error {
		started = true
		return nil
	})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if started {
		t.Fatal("UI must not start when the database cannot be opened")
	}
	if !strings.Contains(errOut.String(), "Failed to initialize SQLite database") {
		t.Fatalf("expected database error output, got %q", errOut.String())
	}
}

func TestRunReportsProgramFailure(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "providers.db")
	var errOut bytes.Buffer

	exitCode := run(nil, &bytes.Buffer{}, &errOut, dbPath, func(*db.DB, string) error {
		return errors.New("terminal exploded")
	})

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}
	if !strings.Contains(errOut.String(), "terminal exploded") {
		t.Fatalf("expected program error output, got %q", errOut.String())
	}
}

func TestRunClosesDatabase(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "providers.db")
	var captured *db.DB

	if exitCode := run(nil, &bytes.Buffer{}, &bytes.Buffer{}, dbPath, func(database *db.DB, _ string) error {
		captured = database
		return nil
	}); exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	// run defers Close, so the database must already be closed here.
	if _, err := captured.ListRecords(); err == nil {
		t.Fatal("expected the database to be closed after run returns")
	}
}
