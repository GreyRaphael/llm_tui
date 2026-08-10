package main

import (
	"fmt"
	"log"
	"os"

	"llm_tui/internal/db"
	"llm_tui/internal/tui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Initialize SQLite database located in executable directory
	dbPath := db.GetDefaultDBPath()
	database, err := db.InitDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize SQLite database at %s: %v", dbPath, err)
	}
	defer database.Close()

	// Launch Bubbletea TUI application
	app := tui.NewAppModel(database)
	p := tea.NewProgram(app, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running LLM Provider TUI: %v\n", err)
		os.Exit(1)
	}
}
