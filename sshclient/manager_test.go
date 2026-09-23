package sshclient

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"makoterm/database"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// ── In-process SSH server ───────────────────────────────────────────
//
// These tests run a real SSH server on a loopback port and drive the client
// against it. That covers the code path that used to be untested entirely:
// authentication, host key verification and trust-on-first-use.

// testServer is a minimal SSH server good enough for one interactive session.
// It runs over an in-memory pipe, so tests need no ports and cannot collide
// with anything else on the machine.
type testServer struct {
	hostKey  ssh.PublicKey
	config   *ssh.ServerConfig
	listener net.Listener

	wg sync.WaitGroup
}

type serverOptions struct {
	// rejectAuth makes every authentication attempt fail, so the dial
	// returns an authentication error.
	rejectAuth bool
	// banner is sent before authentication when non-empty.
	banner string
	// shellMsg is written to the session's stdout when the shell starts.
	shellMsg string
}

// startTestServer starts an SSH server reachable through the returned dialer.
func startTestServer(t *testing.T, opts serverOptions) (*testServer, func(string, string) (net.Conn, error)) {
	t.Helper()

	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	cfg := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if opts.rejectAuth {
				return nil, fmt.Errorf("denied")
			}
			return nil, nil
		},
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if opts.rejectAuth {
				return nil, fmt.Errorf("denied")
			}
			return nil, nil
		},
	}
	if opts.banner != "" {
		cfg.BannerCallback = func(ssh.ConnMetadata) string { return opts.banner }
	}
	cfg.AddHostKey(signer)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}

	srv := &testServer{
		hostKey:  signer.PublicKey(),
		config:   cfg,
		listener: listener,
	}

	// Accept connections until the test is over.
	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			srv.wg.Add(1)
			go func() {
				defer srv.wg.Done()
				srv.handleConn(conn, opts)
			}()
		}
	}()

	// The dialer is a plain TCP dial; the listener port is fixed for the
	// lifetime of the test, so host records and known_hosts entries agree.
	dial := func(network, address string) (net.Conn, error) {
		return net.Dial(network, address)
	}

	t.Cleanup(func() {
		listener.Close()
		srv.wg.Wait()
	})
	return srv, dial
}

// addr is the address clients dial, used for known_hosts entries.
func (s *testServer) addr() string {
	return net.JoinHostPort("127.0.0.1", fmt.Sprint(s.port()))
}

func (s *testServer) handleConn(conn net.Conn, opts serverOptions) {
	defer conn.Close()
	// Never let a stuck peer hang the whole test run.
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))

	sshConn, chans, reqs, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return // authentication failed, as some tests intend
	}
	defer sshConn.Close()

	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "only sessions are supported")
			continue
		}
		channel, requests, err := newChan.Accept()
		if err != nil {
			return
		}
		go func() {
			defer channel.Close()
			shellStarted := false
			for req := range requests {
				ok := true
				switch req.Type {
				case "pty-req":
					// pty-req payload: term string, width, height, pixels, modes
					var payload struct {
						Term          string
						Width, Height uint32
						PixelWidth    uint32
						PixelHeight   uint32
						Modes         string
					}
					_ = ssh.Unmarshal(req.Payload, &payload)
				case "window-change", "env":
					// accepted, nothing to do
				case "shell":
					if opts.shellMsg != "" {
						io.WriteString(channel, opts.shellMsg)
					}
					if req.WantReply {
						req.Reply(true, nil)
					}
					// A shell request also means "run until exit-status".
					// Without that message session.Wait() blocks forever.
					shellStarted = true
				default:
					ok = false
				}
				if req.WantReply {
					req.Reply(ok, nil)
				}
				if shellStarted {
					// exit-status 0, then EOF.
					channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
					channel.CloseWrite()
					return
				}
			}
		}()
	}
}

// knownHostsLine is the entry OpenSSH would write for this server.
func (s *testServer) knownHostsLine(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(knownhosts.Line([]string{s.addr()}, s.hostKey))
}

// hostFor returns a host record pointing at the test server.
func (s *testServer) hostFor() database.Host {
	return database.Host{
		Name:     "test",
		Address:  "127.0.0.1",
		Port:     s.port(),
		User:     "tester",
		Password: "secret",
	}
}

// port is the actual listening port.
func (s *testServer) port() int {
	return s.listener.Addr().(*net.TCPAddr).Port
}

// ── Test harness ────────────────────────────────────────────────────

// testConfig builds a hermetic client configuration: no agent, no real keys,
// prompts answered from the provided input.
func testConfig(t *testing.T, input string) (Config, *bytes.Buffer) {
	t.Helper()
	out := &bytes.Buffer{}
	cfg := Config{
		In:          strings.NewReader(input),
		Out:         out,
		HomeDir:     t.TempDir(), // no ~/.ssh keys to pick up
		NoAgent:     true,
		SkipRawMode: true, // no TTY in tests
		KeepAlive:   -1,
	}
	return cfg, out
}

// ── Tests ───────────────────────────────────────────────────────────

// TestConnect_TOFUAddsHostKey walks the first-connection flow: the key is
// unknown, the user accepts it, and it is written to known_hosts.
func TestConnect_TOFUAddsHostKey(t *testing.T) {
	srv, dial := startTestServer(t, serverOptions{shellMsg: "hello\r\n"})
	cfg, out := testConfig(t, "yes\n")
	cfg.Dial = dial

	err := ConnectWithConfig(srv.hostFor(), cfg)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if !strings.Contains(out.String(), "can't be established") {
		t.Errorf("expected a trust prompt, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Permanently added") {
		t.Errorf("expected a confirmation, got: %q", out.String())
	}

	// The entry must be present and readable by OpenSSH-compatible code.
	path := filepath.Join(cfg.HomeDir, ".ssh", "known_hosts")
	checker, err := knownhosts.New(path)
	if err != nil {
		t.Fatalf("known_hosts is not parseable: %v", err)
	}
	if err := checker(srv.addr(), fakeAddr(srv.addr()), srv.hostKey); err != nil {
		t.Errorf("host key was not stored correctly: %v", err)
	}
}

// TestConnect_KnownHostSkipsPrompt verifies a known key connects silently.
func TestConnect_KnownHostSkipsPrompt(t *testing.T) {
	srv, dial := startTestServer(t, serverOptions{shellMsg: "hello\r\n"})

	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"),
		[]byte(srv.knownHostsLine(t)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, out := testConfig(t, "")
	cfg.HomeDir = home
	cfg.Dial = dial

	if err := ConnectWithConfig(srv.hostFor(), cfg); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	if strings.Contains(out.String(), "can't be established") {
		t.Errorf("prompted for a host key that is already known: %q", out.String())
	}
}

// TestConnect_RefusesChangedHostKey verifies a different key for a known host
// aborts the connection instead of silently trusting it.
func TestConnect_RefusesChangedHostKey(t *testing.T) {
	srv, dial := startTestServer(t, serverOptions{})

	// Store a key that does not belong to this server.
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherPriv)
	if err != nil {
		t.Fatal(err)
	}

	home := t.TempDir()
	sshDir := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(knownhosts.Line([]string{srv.addr()}, otherSigner.PublicKey()))
	if err := os.WriteFile(filepath.Join(sshDir, "known_hosts"), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, out := testConfig(t, "yes\n")
	cfg.Dial = dial // agreeing must not help
	cfg.HomeDir = home

	err = ConnectWithConfig(srv.hostFor(), cfg)
	if err == nil {
		t.Fatal("expected the connection to be refused")
	}
	if !strings.Contains(err.Error(), "changed") {
		t.Errorf("error = %v, want a 'host key changed' error", err)
	}
	if !strings.Contains(out.String(), "REMOTE HOST IDENTIFICATION HAS CHANGED") {
		t.Errorf("expected the warning banner, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Offered key") {
		t.Errorf("expected both fingerprints for comparison, got: %q", out.String())
	}
}

// TestConnect_RejectsHostKeyOnNo verifies that declining the prompt aborts the
// connection and writes nothing.
func TestConnect_RejectsHostKeyOnNo(t *testing.T) {
	srv, dial := startTestServer(t, serverOptions{})
	cfg, _ := testConfig(t, "no\n")
	cfg.Dial = dial

	err := ConnectWithConfig(srv.hostFor(), cfg)
	if err == nil {
		t.Fatal("expected the connection to be refused")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Errorf("error = %v, want a rejection", err)
	}

	path := filepath.Join(cfg.HomeDir, ".ssh", "known_hosts")
	data, err := os.ReadFile(path)
	if err == nil && len(bytes.TrimSpace(data)) != 0 {
		t.Errorf("known_hosts must stay empty, got %q", data)
	}
}

// TestConnect_AuthenticationFailure checks the error is wrapped usefully.
func TestConnect_AuthenticationFailure(t *testing.T) {
	srv, dial := startTestServer(t, serverOptions{rejectAuth: true})
	cfg, _ := testConfig(t, "yes\n")
	cfg.Dial = dial

	err := ConnectWithConfig(srv.hostFor(), cfg)
	if err == nil {
		t.Fatal("expected an authentication error")
	}
	if !strings.Contains(err.Error(), "connect to") {
		t.Errorf("error = %v, want it to mention the address", err)
	}
}

// TestConnect_SendsNonexistentAddress is a cheap check that dial errors are
// reported with the address.
func TestConnect_SendsNonexistentAddress(t *testing.T) {
	cfg, _ := testConfig(t, "")
	host := database.Host{Name: "nothing", Address: "127.0.0.1", Port: 1, User: "u"}

	err := ConnectWithConfig(host, cfg)
	if err == nil {
		t.Fatal("expected a dial error")
	}
	if !strings.Contains(err.Error(), "127.0.0.1:1") {
		t.Errorf("error = %v, want it to mention 127.0.0.1:1", err)
	}
}

// fakeAddr implements net.Addr for the known_hosts checker.
type fakeAddr string

func (a fakeAddr) Network() string { return "tcp" }
func (a fakeAddr) String() string  { return string(a) }

// ── Unit tests for helpers ──────────────────────────────────────────

func TestExpandHome(t *testing.T) {
	home := "/home/tester"
	tests := []struct{ in, want string }{
		{"~/.ssh/id_rsa", filepath.Join(home, ".ssh/id_rsa")},
		{"~", home},
		{"/absolute/key", "/absolute/key"},
		{"relative/key", "relative/key"},
		{"~/", home},
	}
	for _, tt := range tests {
		if got := expandHome(tt.in, home); got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNormalizeKnownHostsEntry(t *testing.T) {
	// Non-standard ports must be bracketed, port 22 bare — the same format
	// OpenSSH writes, so both tools share one known_hosts file.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct{ addr, wantHost string }{
		{"example.com:22", "example.com"},
		{"example.com:2222", "[example.com]:2222"},
		{"10.0.0.1:2200", "[10.0.0.1]:2200"},
	}
	for _, tc := range cases {
		line := knownhosts.Line([]string{tc.addr}, signer.PublicKey())
		if !strings.HasPrefix(line, tc.wantHost+" ") {
			t.Errorf("knownhosts.Line(%q) = %q, want prefix %q", tc.addr, line, tc.wantHost)
		}
	}
}

// testPassphrase matches sshclient/testdata/id_ed25519_passphrase.
const testPassphrase = "makoterm-test-passphrase"

// TestLoadKeyInteractive_EncryptedKeyUsedToBeSkipped is the regression test for
// silently ignoring passphrase-protected keys, which surfaced to the user only
// as an unexplained authentication failure.
func TestLoadKeyInteractive_EncryptedKeyUsedToBeSkipped(t *testing.T) {
	path := filepath.Join("testdata", "id_ed25519_passphrase")

	// 1. An explicit passphrase is used.
	cfg := Config{In: strings.NewReader(""), Out: io.Discard, KeyPassphrase: testPassphrase}
	signer, err := loadKeyInteractive(path, cfg, func(string, ...any) {})
	if err != nil {
		t.Fatalf("loadKeyInteractive with passphrase failed: %v", err)
	}
	if signer == nil {
		t.Fatal("expected a signer")
	}

	// 2. Supplying it interactively works too.
	out := &bytes.Buffer{}
	cfg = Config{In: strings.NewReader(testPassphrase + "\n"), Out: out}
	if _, err := loadKeyInteractive(path, cfg, func(string, ...any) {}); err != nil {
		t.Fatalf("interactive passphrase failed: %v", err)
	}
	if !strings.Contains(out.String(), "Enter passphrase for key") {
		t.Errorf("expected a passphrase prompt, got %q", out.String())
	}

	// 3. A wrong passphrase is reported, not swallowed.
	cfg = Config{In: strings.NewReader("wrong\n"), Out: io.Discard}
	if _, err := loadKeyInteractive(path, cfg, func(string, ...any) {}); err == nil {
		t.Error("expected an error for a wrong passphrase")
	}

	// 4. With prompting disabled the key is skipped with a clear reason.
	no := false
	cfg = Config{In: strings.NewReader(""), Out: io.Discard, PromptPassphrase: &no}
	_, err = loadKeyInteractive(path, cfg, func(string, ...any) {})
	if err == nil || !strings.Contains(err.Error(), "passphrase-protected") {
		t.Errorf("error = %v, want a 'passphrase-protected' explanation", err)
	}
}

func TestLoadKeyInteractive_UnparseableKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, []byte("not a key"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := Config{In: strings.NewReader(""), Out: io.Discard}
	if _, err := loadKeyInteractive(path, cfg, func(string, ...any) {}); err == nil {
		t.Error("expected an error for an unparseable key")
	}
}

func TestLoadKeyInteractive_MissingFile(t *testing.T) {
	cfg := Config{In: strings.NewReader(""), Out: io.Discard}
	if _, err := loadKeyInteractive(filepath.Join(t.TempDir(), "nope"), cfg, func(string, ...any) {}); err == nil {
		t.Error("expected an error for a missing key")
	}
}
