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
	"strings"
	"syscall"
	"time"

	"makoterm/database"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/term"
)

// Connect starts a new full-screen SSH session.
// It runs synchronously and blocks until the remote shell exits.
func Connect(host database.Host) error {
	// Collect all SSH signers into a single callback.
	// IMPORTANT: Go's x/crypto/ssh treats each AuthMethod entry as one attempt
	// at the "publickey" method. If the first one (e.g. an empty agent) returns
	// no signers, the server's response marks "publickey" as failed and all
	// remaining publickey AuthMethods are skipped. By merging all signers into
	// a single PublicKeysCallback, we ensure every key is tried in one pass.
	var signers []ssh.Signer

	// 1. SSH Agent signers
	if authSock := os.Getenv("SSH_AUTH_SOCK"); authSock != "" {
		if conn, err := net.Dial("unix", authSock); err == nil {
			defer conn.Close()
			agentClient := agent.NewClient(conn)
			if agentSigners, err := agentClient.Signers(); err == nil {
				signers = append(signers, agentSigners...)
			}
		}
	}

	// 2. Host-specific key if configured
	if host.KeyPath != "" {
		if signer, err := loadKey(host.KeyPath); err == nil {
			signers = append(signers, signer)
		}
	}

	// 3. Standard keys (~/.ssh/id_ed25519, ~/.ssh/id_rsa)
	homeDir, _ := os.UserHomeDir()
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		p := filepath.Join(homeDir, ".ssh", name)
		if signer, err := loadKey(p); err == nil {
			signers = append(signers, signer)
		}
	}

	var authMethods []ssh.AuthMethod
	if len(signers) > 0 {
		authMethods = append(authMethods, ssh.PublicKeysCallback(func() ([]ssh.Signer, error) {
			return signers, nil
		}))
	}

	// 4. Fallback to password
	if host.Password != "" {
		authMethods = append(authMethods, ssh.Password(host.Password))

		// Also add keyboard-interactive with the stored password,
		// since some servers (e.g. Rockchip, embedded Linux) only accept this method.
		authMethods = append(authMethods, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					answers[i] = host.Password
				}
				return answers, nil
			},
		))
	} else {
		// 5. No stored password — prompt interactively
		authMethods = append(authMethods, ssh.PasswordCallback(func() (string, error) {
			fmt.Fprintf(os.Stderr, "%s@%s's password: ", host.User, host.Address)
			pass, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr) // newline after hidden input
			if err != nil {
				return "", err
			}
			return string(pass), nil
		}))

		authMethods = append(authMethods, ssh.KeyboardInteractive(
			func(user, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i, q := range questions {
					fmt.Fprintf(os.Stderr, "%s", q)
					if echos[i] {
						reader := bufio.NewReader(os.Stdin)
						line, _ := reader.ReadString('\n')
						answers[i] = strings.TrimSpace(line)
					} else {
						pass, err := term.ReadPassword(int(os.Stdin.Fd()))
						fmt.Fprintln(os.Stderr)
						if err != nil {
							return nil, err
						}
						answers[i] = string(pass)
					}
				}
				return answers, nil
			},
		))
	}

	// Host key verification via ~/.ssh/known_hosts
	hostKeyCallback, err := newKnownHostsCallback()
	if err != nil {
		return fmt.Errorf("known_hosts: %w", err)
	}

	config := &ssh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
		Config: ssh.Config{
			Ciphers: []string{
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
			},
			KeyExchanges: []string{
				"curve25519-sha256",
				"curve25519-sha256@libssh.org",
				"ecdh-sha2-nistp256",
				"ecdh-sha2-nistp384",
				"ecdh-sha2-nistp521",
				"diffie-hellman-group14-sha256",
				"diffie-hellman-group14-sha1",
				"diffie-hellman-group1-sha1",
			},
			MACs: []string{
				"hmac-sha2-256-etm@openssh.com",
				"hmac-sha2-256",
				"hmac-sha1",
				"hmac-sha1-96",
			},
		},
	}

	addr := fmt.Sprintf("%s:%d", host.Address, host.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("connect to %s: %w", addr, err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("create session on %s: %w", addr, err)
	}
	defer session.Close()

	// Setup standard IO
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	session.Stdin = os.Stdin

	// Request pseudo terminal
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("set raw terminal: %w", err)
	}
	defer term.Restore(fd, oldState)

	termWidth, termHeight, err := term.GetSize(fd)
	if err != nil {
		termWidth, termHeight = 80, 24
	}

	if err := session.RequestPty("xterm-256color", termHeight, termWidth, modes); err != nil {
		return fmt.Errorf("request PTY on %s: %w", addr, err)
	}

	// Forward terminal resize (SIGWINCH) to the remote PTY
	done := make(chan struct{})
	resizeCh := make(chan os.Signal, 1)
	signal.Notify(resizeCh, syscall.SIGWINCH)
	defer func() {
		signal.Stop(resizeCh)
		close(done)
	}()

	go func() {
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

	// Start remote shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("start shell on %s: %w", addr, err)
	}

	// Wait for session to finish
	if err := session.Wait(); err != nil {
		// ExitError means the remote command exited non-zero — this is normal
		if _, ok := err.(*ssh.ExitError); !ok && err != io.EOF {
			return err
		}
	}

	return nil
}

// loadKey reads and parses an SSH private key from the given path.
func loadKey(path string) (ssh.Signer, error) {
	key, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(key)
}

// newKnownHostsCallback returns a host key callback that verifies against
// ~/.ssh/known_hosts. If the host is unknown, it prompts the user interactively
// (similar to OpenSSH). If the host key has changed, it refuses the connection.
func newKnownHostsCallback() (ssh.HostKeyCallback, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	knownHostsPath := filepath.Join(homeDir, ".ssh", "known_hosts")

	// Ensure ~/.ssh directory exists with proper permissions
	if err := os.MkdirAll(filepath.Dir(knownHostsPath), 0700); err != nil {
		return nil, fmt.Errorf("create .ssh directory: %w", err)
	}

	// Create known_hosts file if it doesn't exist
	if _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {
		f, err := os.OpenFile(knownHostsPath, os.O_CREATE|os.O_WRONLY, 0600)
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
			return nil // Host key matches
		}

		var keyErr *knownhosts.KeyError
		if !errors.As(err, &keyErr) {
			return err // Some other error
		}

		if len(keyErr.Want) > 0 {
			// Host key has CHANGED — refuse connection
			fmt.Fprintf(os.Stderr,
				"\n@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
					"@    WARNING: REMOTE HOST IDENTIFICATION HAS CHANGED!    @\n"+
					"@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@\n"+
					"Host key for %s has changed.\n"+
					"This could indicate a man-in-the-middle attack.\n"+
					"Update %s manually if this is expected.\n",
				hostname, knownHostsPath)
			return fmt.Errorf("host key changed for %s", hostname)
		}

		// Unknown host — prompt user to accept (like OpenSSH)
		fingerprint := ssh.FingerprintSHA256(key)
		fmt.Fprintf(os.Stderr,
			"\nThe authenticity of host '%s' can't be established.\n"+
				"%s key fingerprint is %s.\n"+
				"Are you sure you want to continue connecting? (yes/no): ",
			hostname, key.Type(), fingerprint)

		reader := bufio.NewReader(os.Stdin)
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))

		if answer != "yes" && answer != "y" {
			return fmt.Errorf("host key verification rejected by user")
		}

		// Append to known_hosts
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		f, err := os.OpenFile(knownHostsPath, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return fmt.Errorf("save to known_hosts: %w", err)
		}
		defer f.Close()

		if _, err := fmt.Fprintln(f, line); err != nil {
			return fmt.Errorf("write to known_hosts: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Warning: Permanently added '%s' (%s) to the list of known hosts.\n",
			hostname, key.Type())
		return nil
	}, nil
}
