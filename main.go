// Command makoterm is a terminal UI SSH client. It keeps its connection list
// in a local SQLite database and opens sessions with golang.org/x/crypto/ssh.
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"makoterm/database"
	"makoterm/sshclient"
	"makoterm/ui"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	// Own the exit code from one place, so a failed connection still returns
	// the user to MakoTerm instead of killing the process.
	os.Exit(run())
}

func run() int {
	if err := database.InitDB(""); err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing database: %v\n", err)
		return 1
	}

	logger, logPath := operationLogger()
	if logger != nil {
		defer logger.Printf("--- makoterm started ---")
		logger.Printf("database initialised")
	}

	// A single reader is shared by every interactive prompt: buffering stdin
	// more than once would swallow keystrokes (the old fmt.Scanln(&s) read
	// nothing when the SSH session had already consumed the input).
	stdin := bufio.NewReader(os.Stdin)
	prompts := &promptReader{reader: stdin, out: os.Stderr}

	model := ui.InitialModel()

	for {
		p := tea.NewProgram(model, tea.WithAltScreen())
		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "TUI error: %v\n", err)
			return 1
		}

		m, ok := finalModel.(ui.Model)
		if !ok {
			fmt.Fprintln(os.Stderr, "unexpected model type from the UI")
			return 1
		}

		if m.ShouldQuit || m.SelectedToConnect == nil {
			return 0
		}

		// Keep the UI state (cursor, selected group) to resume after SSH.
		model = m
		model.SelectedToConnect = nil

		host := *m.SelectedToConnect
		fmt.Printf("\n🚀 Connecting to %s (%s@%s:%d)...\n", host.Name, host.User, host.Address, host.Port)

		if err := sshclient.ConnectWithConfig(host, sshclient.Config{
			In:  prompts.in(),
			Out: os.Stderr,
		}); err != nil {
			if logger != nil {
				logger.Printf("connection to %s (%s:%d) failed: %v", host.Name, host.Address, host.Port, err)
			}
			fmt.Printf("\n❌ SSH error: %v\n", err)
			if logPath != "" {
				fmt.Printf("Details were written to %s\n", logPath)
			}
			fmt.Print("\nPress [Enter] to return to MakoTerm...")
			prompts.waitForEnter()
		}
	}
}

// promptReader serialises interactive prompts on the shared stdin reader.
//
// It exists so that host-key questions asked deep inside the SSH handshake and
// the "press Enter" prompt at the end use the same buffer.
type promptReader struct {
	reader *bufio.Reader
	out    *os.File
}

func (p *promptReader) in() *bufio.Reader { return p.reader }

// waitForEnter blocks until the user presses Enter (or stdin ends), which is
// more forgiving than fmt.Scanln: it keeps the reader state consistent and
// does not fire immediately on leftover input.
func (p *promptReader) waitForEnter() {
	if _, err := p.reader.ReadString('\n'); err != nil {
		// Nothing more to read (closed pipe, EOF). Returning immediately is
		// still better than blocking forever.
		fmt.Fprintln(p.out)
	}
}

// operationLogger returns a logger for connection diagnostics, and the path it
// writes to. Logging is opt-in through MAKOTERM_DEBUG, mirroring sshclient:
// a path enables that file, "1"/"true" means ~/.makoterm.log.
func operationLogger() (*log.Logger, string) {
	value := os.Getenv(sshclient.DebugEnvVar)
	if value == "" || value == "0" {
		return nil, ""
	}

	path := value
	if value == "1" || value == "true" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, ""
		}
		path = filepath.Join(home, ".makoterm.log")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, ""
	}
	return log.New(f, "makoterm: ", log.LstdFlags), path
}
