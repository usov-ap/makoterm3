package main

import (
	"fmt"
	"os"

	"makoterm/database"
	"makoterm/sshclient"
	"makoterm/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	err := database.InitDB("") // Uses ~/.makoterm.db
	if err != nil {
		fmt.Printf("Error initializing database: %v\n", err)
		os.Exit(1)
	}

	model := ui.InitialModel()

	for {
		// Create a new program instance with the same model state
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			fmt.Printf("Alas, there's been an error: %v", err)
			os.Exit(1)
		}

		m := finalModel.(ui.Model)
		
		// If user pressed q or ctrl+c
		if m.ShouldQuit {
			break
		}

		// If user selected a host
		if m.SelectedToConnect != nil {
			// Save the state to resume later
			model = m
			model.SelectedToConnect = nil // Reset so we don't connect in a loop
			
			// We are now outside AltScreen. The terminal is ours.
			fmt.Printf("\nConnecting to %s...\n", m.SelectedToConnect.Name)
			err = sshclient.Connect(*m.SelectedToConnect)
			if err != nil {
				// To display the error in the UI when it resumes:
				// (Wait, we can't easily display it without a state, but we can print it and wait, or pass it to model)
				fmt.Printf("\nSSH Error: %v\nPress enter to continue...", err)
				var dummy string
				fmt.Scanln(&dummy)
			}
		} else {
			// Some other exit reason?
			break
		}
	}
}
