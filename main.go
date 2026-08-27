package main

import (
	"fmt"
	"log"
	"os"

	"llm_tui/internal/db"
	"llm_tui/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

// Version can be set at build time via -ldflags="-X main.Version=v1.9.0"
var Version = "v1.9.0"

func main() {
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if arg == "-v" || arg == "--version" || arg == "version" || arg == "-version" {
			fmt.Printf("llm_tui version %s\n", Version)
			os.Exit(0)
		}
	}

	// Initialize SQLite database located in executable directory
	dbPath := db.GetDefaultDBPath()
	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite database at %s: %v", dbPath, err)
	}
	defer database.Close()

	// Launch Bubbletea TUI application
	app := tui.NewAppModel(database, Version)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running LLM Provider TUI: %v\n", err)
		os.Exit(1)
	}
}
