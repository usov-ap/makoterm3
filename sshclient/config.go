package sshclient

import (
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Config holds the process-level inputs Connect needs. The zero value is valid
// and means "use the real process environment"; tests override individual
// fields to keep a connection hermetic.
type Config struct {
	// In and Out carry interactive prompts (password, host key acceptance).
	// Defaults to os.Stdin and os.Stderr.
	In  io.Reader
	Out io.Writer

	// HomeDir is used to find ~/.ssh. Defaults to the current user's home.
	HomeDir string

	// KnownHostsPath overrides the known_hosts file location.
	KnownHostsPath string

	// Terminal is the file whose descriptor is put into raw mode and queried
	// for its size. Defaults to os.Stdin; tests can supply a plain file.
	Terminal *os.File

	// SkipRawMode leaves the terminal alone. Only for tests: without a real
	// TTY the raw-mode ioctl fails.
	SkipRawMode bool

	// AgentSocket overrides SSH_AUTH_SOCK. An empty string therefore means
	// "no agent"; the default is the value of the environment variable.
	AgentSocket string

	// NoAgent disables SSH agent authentication entirely.
	NoAgent bool

	// Password is used instead of prompting when a host has no stored
	// password. Empty means "prompt interactively".
	Password string

	// KeyPassphrase answers encrypted private key prompts instead of asking.
	// Use PromptPassphrase to distinguish "no passphrase" from "not set".
	KeyPassphrase string

	// PromptPassphrase allows asking the user for an encrypted key's
	// passphrase (default true).
	PromptPassphrase *bool

	// KeepAlive is the interval between keepalive requests. Zero disables
	// keepalives; negative means the default (30s).
	KeepAlive time.Duration

	// Logger, when set, receives debug diagnostics.
	Logger *log.Logger

	// Dial opens the transport to address (a "host:port" string). Defaults to
	// a TCP dial. Tests use it to run the client against an in-process server
	// over an in-memory pipe.
	Dial func(network, address string) (net.Conn, error)
}

// DebugEnvVar enables debug logging when it holds a path, or when set to any
// non-empty value other than "0" (in which case ~/.makoterm.log is used).
const DebugEnvVar = "MAKOTERM_DEBUG"

// defaultKeepAlive is how often a keepalive request is sent to the server.
const defaultKeepAlive = 30 * time.Second

// resolve fills in the defaults for every unset field.
func (c Config) resolve() (Config, error) {
	if c.In == nil {
		c.In = os.Stdin
	}
	if c.Out == nil {
		c.Out = os.Stderr
	}
	if c.HomeDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return c, err
		}
		c.HomeDir = home
	}
	if c.KnownHostsPath == "" {
		c.KnownHostsPath = filepath.Join(c.HomeDir, ".ssh", "known_hosts")
	}
	if c.Terminal == nil {
		c.Terminal = os.Stdin
	}
	if c.AgentSocket == "" && !c.NoAgent {
		c.AgentSocket = os.Getenv("SSH_AUTH_SOCK")
	}
	if c.KeepAlive < 0 {
		c.KeepAlive = defaultKeepAlive
	}
	if c.Dial == nil {
		c.Dial = net.Dial
	}
	return c, nil
}

// wantsPassphrasePrompt reports whether the user may be asked for key
// passphrases. Defaults to true.
func (c Config) wantsPassphrasePrompt() bool {
	return c.PromptPassphrase == nil || *c.PromptPassphrase
}

// loggerFromEnv builds the debug logger requested by MAKOTERM_DEBUG.
//
// It returns nil when debug logging is off, and never fails the connection:
// a logger that cannot be opened is simply skipped.
func loggerFromEnv(override *log.Logger) *log.Logger {
	if override != nil {
		return override
	}
	value := os.Getenv(DebugEnvVar)
	if value == "" || value == "0" {
		return nil
	}

	path := value
	if value == "1" || value == "true" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		path = filepath.Join(home, ".makoterm.log")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil
	}
	return log.New(f, "makoterm ssh: ", log.LstdFlags)
}
