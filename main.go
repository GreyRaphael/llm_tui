package main

import (
	"fmt"
	"io"
	"os"

	"llm_tui/internal/db"
	"llm_tui/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// Version can be set at build time via -ldflags="-X main.Version=v1.11.0"
var Version = "v1.11.0"

// programStarter launches the TUI for an initialized database. It is a seam so
// run's startup and error handling can be tested without a real terminal.
type programStarter func(database *db.DB, version string) error

// printVersionIfRequested writes the version for -v/--version/version/-version
// arguments and reports whether the program should stop.
func printVersionIfRequested(args []string, out io.Writer) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "-v", "--version", "version", "-version":
		fmt.Fprintf(out, "llm_tui version %s\n", Version)
		return true
	}
	return false
}

// run executes one program invocation and returns the process exit code.
func run(args []string, out, errOut io.Writer, dbPath string, start programStarter) int {
	if printVersionIfRequested(args, out) {
		return 0
	}

	database, err := db.InitDB(dbPath)
	if err != nil {
		fmt.Fprintf(errOut, "Failed to initialize SQLite database at %s: %v\n", dbPath, err)
		return 1
	}
	defer database.Close()

	if err := start(database, Version); err != nil {
		fmt.Fprintf(errOut, "Error running LLM Provider TUI: %v\n", err)
		return 1
	}
	return 0
}

// runProgram starts the Bubbletea UI in the alternate screen buffer.
func runProgram(database *db.DB, version string) error {
	p := tea.NewProgram(tui.NewAppModel(database, version), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, db.GetDefaultDBPath(), runProgram))
}
