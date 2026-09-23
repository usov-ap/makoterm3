// Package sshclient opens interactive SSH sessions for MakoTerm hosts.
//
// It speaks SSH directly through golang.org/x/crypto/ssh, so no external ssh
// binary is needed. Host keys are verified against known_hosts with
// interactive trust-on-first-use for unknown hosts.
package sshclient

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"makoterm/database"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// Connect starts a new full-screen SSH session using the process environment.
// It runs synchronously and blocks until the remote shell exits.
func Connect(host database.Host) error {
	return ConnectWithConfig(host, Config{})
}

// ConnectWithConfig is Connect with injectable I/O, paths and logging. Tests
// use it to talk to an in-process SSH server.
func ConnectWithConfig(host database.Host, cfg Config) error {
	resolved, err := cfg.resolve()
	if err != nil {
		return fmt.Errorf("resolve configuration: %w", err)
	}
	logger := loggerFromEnv(resolved.Logger)
	debugf := func(format string, args ...any) {
		if logger != nil {
			logger.Printf(format, args...)
		}
	}

	authMethods, agentCloser := buildAuthMethods(host, resolved, debugf)
	if agentCloser != nil {
		defer agentCloser.Close()
	}

	hostKeyCallback, err := newKnownHostsCallback(resolved, debugf)
	if err != nil {
		return fmt.Errorf("known_hosts: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
		Config: ssh.Config{
			Ciphers:      ciphers(),
			KeyExchanges: keyExchanges(),
			MACs:         macs(),
		},
	}

	addr := net.JoinHostPort(host.Address, strconv.Itoa(host.Port))
	debugf("connecting to %s as %s (%d auth method(s))", addr, host.User, len(authMethods))

	transport, err := resolved.Dial("tcp", addr)
	if err != nil {
		debugf("dial %s failed: %v", addr, err)
		return fmt.Errorf("connect to %s: %w", addr, err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(transport, addr, config)
	if err != nil {
		transport.Close()
		debugf("ssh handshake with %s failed: %v", addr, err)
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	if err != nil {
		debugf("dial %s failed: %v", addr, err)
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create session on %s: %w", addr, err)
	}
	defer session.Close()

	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	fd := int(resolved.Terminal.Fd())
	if !resolved.SkipRawMode {
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			return fmt.Errorf("set raw terminal: %w", err)
		}
		defer term.Restore(fd, oldState)
	}

	termWidth, termHeight, err := term.GetSize(fd)
	if err != nil {
		termWidth, termHeight = 80, 24
	}

	if err := session.RequestPty("xterm-256color", termHeight, termWidth, modes); err != nil {
		return fmt.Errorf("request PTY on %s: %w", addr, err)
	}

	// Forward terminal resize (SIGWINCH) to the remote PTY. The goroutine is
	// stopped and waited for before Connect returns, so it cannot touch the
	// session after it is closed.
	stopResize := forwardResizes(session, fd)
	defer stopResize()

	// Keep the session alive through NAT and idle firewalls.
	stopKeepAlive := startKeepAlive(client, resolved.KeepAlive, debugf)
	defer stopKeepAlive()

	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell on %s: %w", addr, err)
	}

	if err := session.Wait(); err != nil {
		// ExitError means the remote command exited non-zero — normal.
		if _, ok := err.(*ssh.ExitError); !ok && err != io.EOF {
			return err
		}
	}
	debugf("session on %s closed", addr)
	return nil
}

// ── Authentication ──────────────────────────────────────────────────

// buildAuthMethods assembles the authentication methods in priority order:
// SSH agent, the host's key, the standard ~/.ssh keys, then a password.
//
// All signers are exposed through a single PublicKeysCallback. Go's x/crypto/ssh
// treats every AuthMethod entry as one attempt at the "publickey" method: if an
// empty agent were its own entry, it would fail first and mark "publickey" as
// exhausted, so file-based keys would never be tried.
//
// The returned closer keeps the agent connection alive for as long as the
// signers are used.
func buildAuthMethods(host database.Host, cfg Config, debugf func(string, ...any)) ([]ssh.AuthMethod, io.Closer) {
	var (
		signers []ssh.Signer
		closer  io.Closer
	)

	if !cfg.NoAgent && cfg.AgentSocket != "" {
		conn, err := net.Dial("unix", cfg.AgentSocket)
		if err != nil {
			debugf("ssh agent unavailable at %s: %v", cfg.AgentSocket, err)
		} else {
			closer = conn
			agentSigners, err := agent.NewClient(conn).Signers()
			if err != nil {
				debugf("ssh agent returned no signers: %v", err)
			} else {
				debugf("ssh agent provided %d signer(s)", len(agentSigners))
				signers = append(signers, agentSigners...)
			}
		}
	}

	// Host-specific key, then the conventional default keys.
	paths := make([]string, 0, 3)
	if host.KeyPath != "" {
		paths = append(paths, expandHome(host.KeyPath, cfg.HomeDir))
	}
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		paths = append(paths, filepath.Join(cfg.HomeDir, ".ssh", name))
	}

	for _, path := range paths {
		signer, err := loadKeyInteractive(path, cfg, debugf)
		if err != nil {
			// Only mention keys the user explicitly configured; the default
			// paths are expected to be missing on many systems.
			if path == expandHome(host.KeyPath, cfg.HomeDir) {
				fmt.Fprintf(cfg.Out, "warning: %s: %v\n", path, err)
			}
			debugf("skipping key %s: %v", path, err)
			continue
		}
		debugf("loaded key %s (%s)", path, signer.PublicKey().Type())
		signers = append(signers, signer)
	}

	var methods []ssh.AuthMethod
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
			return signers, nil
		}))
	}

	if host.Password != "" {
		methods = append(methods, ssh.Password(host.Password))
		// Some servers (embedded Linux, older appliances) only offer
		// keyboard-interactive, so present the stored password there too.
		methods = append(methods, ssh.KeyboardInteractive(passwordKeyboardInteractive(host.Password)))
	} else {
		methods = append(methods, ssh.PasswordCallback(func() (string, error) {
			return promptPassword(cfg, host)
		}))
		methods = append(methods, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				return promptKeyboardInteractive(cfg, host, questions, echos)
			},
		))
	}

	return methods, closer
}

// passwordKeyboardInteractive answers every question with the stored password.
func passwordKeyboardInteractive(password string) ssh.KeyboardInteractiveChallenge {
	return func(user, instruction string, questions []string, echos []bool) ([]string, error) {
		answers := make([]string, len(questions))
		for i := range questions {
			answers[i] = password
		}
		return answers, nil
	}
}

// promptPassword asks for a password on the terminal without echoing it.
func promptPassword(cfg Config, host database.Host) (string, error) {
	if cfg.Password != "" {
		return cfg.Password, nil
	}
	fmt.Fprintf(cfg.Out, "%s@%s's password: ", host.User, host.Address)

	if f, ok := cfg.In.(*os.File); ok {
		pass, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(cfg.Out)
		if err != nil {
			return "", err
		}
		return string(pass), nil
	}

	// Not a terminal (tests, pipes): read a line.
	reader := bufio.NewReader(cfg.In)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// promptKeyboardInteractive asks the server's questions interactively, hiding
// the answers the server marks as secret.
func promptKeyboardInteractive(cfg Config, host database.Host, questions []string, echos []bool) ([]string, error) {
	answers := make([]string, len(questions))
	for i, q := range questions {
		if q != "" {
			fmt.Fprintf(cfg.Out, "%s", q)
		}

		if i < len(echos) && echos[i] {
			reader := bufio.NewReader(cfg.In)
			line, err := reader.ReadString('\n')
			if err != nil && line == "" {
				return nil, err
			}
			answers[i] = strings.TrimRight(line, "\r\n")
			continue
		}

		if f, ok := cfg.In.(*os.File); ok {
			pass, err := term.ReadPassword(int(f.Fd()))
			fmt.Fprintln(cfg.Out)
			if err != nil {
				return nil, err
			}
			answers[i] = string(pass)
			continue
		}

		reader := bufio.NewReader(cfg.In)
		line, err := reader.ReadString('\n')
		if err != nil && line == "" {
			return nil, err
		}
		answers[i] = strings.TrimRight(line, "\r\n")
	}
	return answers, nil
}

// loadKeyInteractive reads a private key, asking for its passphrase when the
// key is encrypted.
//
// Old versions ignored encrypted keys silently, which surfaced only as an
// unexplained "unable to authenticate".
func loadKeyInteractive(path string, cfg Config, debugf func(string, ...any)) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(key)
	if err == nil {
		return signer, nil
	}

	var needsPassphrase *ssh.PassphraseMissingError
	if !errors.As(err, &needsPassphrase) {
		return nil, err
	}

	// An explicit passphrase (tests, config) wins.
	if cfg.KeyPassphrase != "" {
		return ssh.ParsePrivateKeyWithPassphrase(key, []byte(cfg.KeyPassphrase))
	}
	if !cfg.wantsPassphrasePrompt() {
		return nil, fmt.Errorf("key is passphrase-protected (passphrase prompting disabled)")
	}

	fmt.Fprintf(cfg.Out, "Enter passphrase for key %s: ", path)
	passphrase, err := readSecret(cfg)
	if err != nil {
		return nil, err
	}
	debugf("retrying %s with a passphrase", path)
	return ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
}

// readSecret reads a line without echo when possible.
func readSecret(cfg Config) ([]byte, error) {
	if f, ok := cfg.In.(*os.File); ok {
		secret, err := term.ReadPassword(int(f.Fd()))
		fmt.Fprintln(cfg.Out)
		if err != nil {
			return nil, err
		}
		return secret, nil
	}
	reader := bufio.NewReader(cfg.In)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return nil, err
	}
	return []byte(strings.TrimRight(line, "\r\n")), nil
}

// expandHome expands a leading ~ in a user-supplied key path.
func expandHome(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

// ── Session plumbing ────────────────────────────────────────────────

// forwardResizes copies SIGWINCH events to the remote PTY. The returned
// function stops the goroutine and waits for it to exit.
func forwardResizes(session *ssh.Session, fd int) func() {
	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)

	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		for {
			select {
			case <-done:
				return
			case <-resizeCh:
				if w, h, err := term.GetSize(fd); err == nil {
					_ = session.WindowChange(h, w)
				}
			}
		}
	}()

	return func() {
		signal.Stop(resizeCh)
		close(done)
		<-stopped
	}
}

// startKeepAlive sends periodic keepalive requests so idle sessions survive
// NAT and firewall timeouts. The returned function stops the ticker and waits
// for the goroutine, which is important: it must not use the client after the
// caller closes it.
func startKeepAlive(client *ssh.Client, interval time.Duration, debugf func(string, ...any)) func() {
	if interval <= 0 {
		return func() {}
	}

	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if _, _, err := client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
					debugf("keepalive failed: %v", err)
					return
				}
			}
		}
	}()

	return func() {
		close(done)
		<-stopped
	}
}

// ── Algorithm selection ─────────────────────────────────────────────

// ciphers lists the supported encryption algorithms, modern first.
func ciphers() []string {
	return []string{
		"aes128-gcm@openssh.com",
		"aes256-gcm@openssh.com",
		"chacha20-poly1305@openssh.com",
		"aes128-ctr",
		"aes192-ctr",
		"aes256-ctr",
		"aes128-cbc",
		"3des-cbc",
		"aes192-cbc",
		"aes256-cbc",
	}
}

// keyExchanges lists the supported key exchange algorithms, modern first.
// The sha1 and group1 entries exist for legacy equipment (routers, switches).
func keyExchanges() []string {
	return []string{
		"curve25519-sha256",
		"curve25519-sha256@libssh.org",
		"ecdh-sha2-nistp256",
		"ecdh-sha2-nistp384",
		"ecdh-sha2-nistp521",
		"diffie-hellman-group14-sha256",
		"diffie-hellman-group14-sha1",
		"diffie-hellman-group1-sha1",
	}
}

// macs lists the supported message authentication codes, modern first.
func macs() []string {
	return []string{
		"hmac-sha2-256-etm@openssh.com",
		"hmac-sha2-256",
		"hmac-sha1",
		"hmac-sha1-96",
	}
}

// ── Host key verification ───────────────────────────────────────────

// newKnownHostsCallback returns a callback that verifies host keys against
// known_hosts: matching keys pass, changed keys are refused, and unknown hosts
// get an interactive trust-on-first-use prompt like OpenSSH.
func newKnownHostsCallback(cfg Config, debugf func(string, ...any)) (ssh.HostKeyCallback, error) {
	knownHostsPath := cfg.KnownHostsPath

	if err := os.MkdirAll(filepath.Dir(knownHostsPath), 0o700); err != nil {
		return nil, fmt.Errorf("create ssh directory: %w", err)
	}
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, fmt.Errorf("create known_hosts: %w", err)
		}
		f.Close()
	}

	checker, err := knownhosts.New(knownHostsPath)
	if err != nil {
		return nil, fmt.Errorf("parse known_hosts: %w", err)
	}

	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		err := checker(hostname, remote, key)
		if err == nil {
			return nil
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err
		}

		if len(keyErr.Want) > 0 {
			return refuseChangedHostKey(cfg, knownHostsPath, hostname, key, keyErr)
		}

		return trustNewHostKey(cfg, knownHostsPath, hostname, key, debugf)
	}, nil
}

// refuseChangedHostKey reports a changed host key, including the fingerprints
// so the user can compare them with the server out of band.
func refuseChangedHostKey(cfg Config, path, hostname string, key ssh.PublicKey, keyErr *knownhosts.KeyError) error {
	fmt.Fprintf(cfg.Out,
		"\n@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
			"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!     @\n"+
			"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
			"Host key for %s has changed.\n"+
			"Offered key:  %s %s\n",
		hostname, key.Type(), ssh.FingerprintSHA256(key))
	for _, want := range keyErr.Want {
		fmt.Fprintf(cfg.Out, "Known key:    %s %s\n", want.Key.Type(), ssh.FingerprintSHA256(want.Key))
	}
	fmt.Fprintf(cfg.Out,
		"This could indicate a man-in-the-middle attack.\n"+
			"Remove the offending line from %s if the change is expected.\n", path)

	return fmt.Errorf("host key for %s has changed", hostname)
}

// trustNewHostKey asks the user to accept an unknown host key and appends it
// to known_hosts on approval.
func trustNewHostKey(cfg Config, path, hostname string, key ssh.PublicKey, debugf func(string, ...any)) error {
	fingerprint := ssh.FingerprintSHA256(key)
	fmt.Fprintf(cfg.Out,
		"\nThe authenticity of host '%s' can't be established.\n"+
			"%s key fingerprint is %s.\n"+
			"Are you sure you want to continue connecting? (yes/no): ",
		hostname, key.Type(), fingerprint)

	reader := bufio.NewReader(cfg.In)
	answer, err := reader.ReadString('\n')
	if err != nil && answer == "" {
		return fmt.Errorf("host key verification aborted: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "yes", "y":
	default:
		return fmt.Errorf("host key verification rejected by user")
	}

	// knownhosts.Line normalizes the address itself, and the callback already
	// receives "host:port" (with brackets for non-standard ports), so the
	// entry matches what OpenSSH would write.
	line := knownhosts.Line([]string{hostname}, key)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("save to known_hosts: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, line); err != nil {
		return fmt.Errorf("write to known_hosts: %w", err)
	}
	debugf("added host key for %s to %s", hostname, path)

	fmt.Fprintf(cfg.Out, "Warning: Permanently added '%s' (%s) to the list of known hosts.\n",
		hostname, key.Type())
	return nil
}
