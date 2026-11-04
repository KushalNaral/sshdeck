package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/KushalNaral/sshdeck/internal/store"
)

const goodPassword = "s3cret"

// TestMain keeps the tests' remembered host keys out of the real config directory.
func TestMain(m *testing.M) {
	d, _ := os.MkdirTemp("", "sshx")
	KnownHosts = filepath.Join(d, "known_hosts")
	code := m.Run()
	os.RemoveAll(d)
	os.Exit(code)
}

// testServer is an in-process SSH server: password auth, "exec" answers from a table, "shell" echoes
// input back in upper case, and "window-change" requests are recorded.
type testServer struct {
	addr     string
	keys     []ssh.PublicKey // accepted for public-key login
	accepted atomic.Int32    // TCP connections the server has taken
	live     []net.Conn      // open connections, so a test can cut them
	mu       sync.Mutex
	windows  [][2]uint32 // cols, rows from pty-req and window-change
}

// dropAll cuts every open connection, like a network failure.
func (ts *testServer) dropAll() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for _, c := range ts.live {
		c.Close()
	}
}

func (ts *testServer) authorize(k ssh.PublicKey) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.keys = append(ts.keys, k)
}

func (ts *testServer) lastWindow() [2]uint32 {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if len(ts.windows) == 0 {
		return [2]uint32{}
	}
	return ts.windows[len(ts.windows)-1]
}

func startServer(t *testing.T) (*testServer, store.Server) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	ts := &testServer{}
	cfg := &ssh.ServerConfig{PublicKeyCallback: func(c ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
		ts.mu.Lock()
		defer ts.mu.Unlock()
		for _, ok := range ts.keys {
			if c.User() == "deploy" && bytes.Equal(ok.Marshal(), k.Marshal()) {
				return nil, nil
			}
		}
		return nil, ssh.ErrNoAuth
	}, PasswordCallback: func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
		if c.User() == "deploy" && string(pw) == goodPassword {
			return nil, nil
		}
		return nil, ssh.ErrNoAuth
	}}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	ts.addr = ln.Addr().String()
	go func() {
		for {
			nc, err := ln.Accept()
			if err != nil {
				return
			}
			ts.accepted.Add(1)
			ts.mu.Lock()
			ts.live = append(ts.live, nc)
			ts.mu.Unlock()
			go ts.serve(nc, cfg)
		}
	}()
	host, port, _ := net.SplitHostPort(ts.addr)
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	return ts, store.Server{ID: "t", Name: "test", Host: host, Port: p, User: "deploy", Auth: "password"}
}

func (ts *testServer) serve(nc net.Conn, cfg *ssh.ServerConfig) {
	conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(reqs)
	for nch := range chans {
		if nch.ChannelType() != "session" {
			nch.Reject(ssh.UnknownChannelType, "no")
			continue
		}
		ch, creqs, _ := nch.Accept()
		go ts.session(ch, creqs)
	}
}

func (ts *testServer) session(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	for r := range reqs {
		switch r.Type {
		case "pty-req":
			ts.setWindow(r.Payload, true)
			r.Reply(true, nil)
		case "window-change":
			ts.setWindow(r.Payload, false)
			r.Reply(true, nil)
		case "shell":
			r.Reply(true, nil)
			go func() { // echo input upper-cased until the client goes away
				buf := make([]byte, 256)
				for {
					n, err := ch.Read(buf)
					if n > 0 {
						ch.Write(bytes.ToUpper(buf[:n]))
					}
					if err != nil {
						return
					}
				}
			}()
		case "exec":
			r.Reply(true, nil)
			cmd := string(r.Payload[4:])
			code := 0
			switch cmd {
			case "echo hi":
				ch.Write([]byte("hi\n"))
			case "fail":
				ch.Stderr().Write([]byte("boom\n"))
				code = 3
			}
			ch.SendRequest("exit-status", false, []byte{0, 0, 0, byte(code)})
			return
		default:
			r.Reply(false, nil)
		}
	}
}

// setWindow records cols and rows. A pty-req payload starts with the terminal-type string; window-change does not.
func (ts *testServer) setWindow(p []byte, pty bool) {
	u32 := func(b []byte) uint32 { return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3]) }
	if pty {
		p = p[4+u32(p):]
	}
	ts.mu.Lock()
	ts.windows = append(ts.windows, [2]uint32{u32(p), u32(p[4:])})
	ts.mu.Unlock()
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func TestRunReturnsStdout(t *testing.T) {
	_, srv := startServer(t)
	out, err := Run(ctx(t), srv, goodPassword, "echo hi")
	if err != nil || out != "hi\n" {
		t.Fatalf("got %q, %v", out, err)
	}
}

func TestRunNonZeroExitIsAnError(t *testing.T) {
	_, srv := startServer(t)
	_, err := Run(ctx(t), srv, goodPassword, "fail")
	if err == nil {
		t.Fatal("want an error for exit status 3")
	}
}

func TestWrongPasswordIsAReadableError(t *testing.T) {
	_, srv := startServer(t)
	for name, fn := range map[string]func() error{
		"Run":  func() error { _, err := Run(ctx(t), srv, "nope", "echo hi"); return err },
		"Open": func() error { _, err := Open(ctx(t), srv, "nope"); return err },
	} {
		err := fn()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "password") {
			t.Errorf("%s: want an error that mentions the password, got %v", name, err)
		}
	}
}

func TestUnreachableHostFailsFast(t *testing.T) {
	srv := store.Server{Host: "127.0.0.1", Port: 1, User: "deploy", Auth: "password"} // nothing listens on port 1
	if _, err := Run(ctx(t), srv, goodPassword, "echo hi"); err == nil {
		t.Fatal("want a connection error")
	}
}

func TestShellEchoesInput(t *testing.T) {
	_, srv := startServer(t)
	s, err := Open(ctx(t), srv, goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Write([]byte("ls\n")); err != nil {
		t.Fatal(err)
	}
	select {
	case b := <-s.Output():
		if !strings.Contains(string(b), "LS") {
			t.Fatalf("got %q", b)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no output")
	}
}

func TestResizeReachesTheServer(t *testing.T) {
	ts, srv := startServer(t)
	s, err := Open(ctx(t), srv, goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Resize(132, 43); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if ts.lastWindow() == [2]uint32{132, 43} {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("server saw %v, want [132 43]", ts.lastWindow())
}

func TestCloseEndsOutput(t *testing.T) {
	_, srv := startServer(t)
	s, err := Open(ctx(t), srv, goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	select {
	case _, open := <-s.Output():
		if open {
			// drain anything buffered, then it must close
			for range s.Output() {
			}
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Output was not closed after Close")
	}
}

func TestFirstContactIsRememberedAndAChangedKeyIsRefused(t *testing.T) {
	ts, srv := startServer(t)
	if _, err := Run(ctx(t), srv, goodPassword, "echo hi"); err != nil { // first contact: trusted and remembered
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(KnownHosts)
	if !strings.Contains(string(saved), "ssh-ed25519") {
		t.Fatalf("the host key was not remembered: %q", saved)
	}
	// A second server with a different key, reached under the first one's address, must be refused.
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	wrong, _ := ssh.NewPublicKey(pub)
	other := filepath.Join(t.TempDir(), "kh")
	os.WriteFile(other, []byte(knownhosts.Line([]string{knownhosts.Normalize(ts.addr)}, wrong)+"\n"), 0o600)
	old := KnownHosts
	KnownHosts = other
	defer func() { KnownHosts = old }()
	_, err := Run(ctx(t), srv, goodPassword, "echo hi")
	if err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("want a host-key-changed error, got %v", err)
	}
}
