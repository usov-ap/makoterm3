package sshclient

import (
	"fmt"
	"io"
	"os"

	"makoterm/database"

	"golang.org/x/crypto/ssh"
	"golang.org/x/term"
)

// Session represents an active SSH session wrapper
type Session struct {
	Host       database.Host
	Client     *ssh.Client
	SSHSession *ssh.Session
}

// Connect starts a new full-screen SSH session
func Connect(host database.Host) error {
	// For simplicity, we just use the password from the DB. 
	// In a real app, this should try ssh-agent, ~/.ssh/id_rsa, then password.
	
	config := &ssh.ClientConfig{
		User: host.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(host.Password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Insecure for demo, should verify in production
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
