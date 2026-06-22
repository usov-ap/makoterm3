package sshclient

import (
	"fmt"
	"io"
	"os"

	"makoterm/database"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
	"net"
	"path/filepath"
)

// Session represents an active SSH session wrapper
type Session struct {
	Host       database.Host
	Client     *ssh.Client
	SSHSession *ssh.Session
}

// Connect starts a new full-screen SSH session
func Connect(host database.Host) error {
	var authMethods []ssh.AuthMethod

	// 1. Try SSH Agent
	if authSock := os.Getenv("SSH_AUTH_SOCK"); authSock != "" {
		if conn, err := net.Dial("unix", authSock); err == nil {
			agentClient := agent.NewClient(conn)
			authMethods = append(authMethods, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	// 2. Try standard keys if they exist
	homeDir, _ := os.UserHomeDir()
	keyPaths := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519"),
		filepath.Join(homeDir, ".ssh", "id_rsa"),
	}
	
	for _, p := range keyPaths {
		if key, err := os.ReadFile(p); err == nil {
			if signer, err := ssh.ParsePrivateKey(key); err == nil {
				authMethods = append(authMethods, ssh.PublicKeys(signer))
			}
		}
	}

	// 3. Fallback to password
	if host.Password != "" {
		authMethods = append(authMethods, ssh.Password(host.Password))
	}

	config := &ssh.ClientConfig{
		User:            host.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Insecure for demo, should verify in production
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
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer client.Close()
	
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	defer session.Close()

	// Setup standard IO
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	// TODO: We need to intercept Stdin to watch for Ctrl+X to detach.
	// For now we pass Stdin directly.
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
		return err
	}
	defer term.Restore(fd, oldState)

	termWidth, termHeight, err := term.GetSize(fd)
	if err != nil {
		termWidth, termHeight = 80, 24
	}

	if err := session.RequestPty("xterm-256color", termHeight, termWidth, modes); err != nil {
		return fmt.Errorf("request for pseudo terminal failed: %w", err)
	}

	// Start remote shell
	if err := session.Shell(); err != nil {
		return fmt.Errorf("failed to start shell: %w", err)
	}

	// Wait for session to finish
	err = session.Wait()
	if err != nil {
		// ExitError means remote command exited non-zero, this is normal
		if _, ok := err.(*ssh.ExitError); !ok && err != io.EOF {
			return err
		}
	}

	return nil
}
