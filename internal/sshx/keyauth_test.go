package sshx

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// newKey makes a key pair; the private half is written to a file, encrypted when pass is not empty.
func newKey(t *testing.T, pass string) (ed25519.PrivateKey, ssh.PublicKey, string) {
	t.Helper()
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	pub, _ := ssh.NewPublicKey(priv.Public())
	var blk *pem.Block
	var err error
	if pass == "" {
		blk, err = ssh.MarshalPrivateKey(priv, "")
	} else {
		blk, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "", []byte(pass))
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(blk), 0o600); err != nil {
		t.Fatal(err)
	}
	return priv, pub, path
}

func TestKeyFileLogin(t *testing.T) {
	ts, srv := startServer(t)
	_, pub, path := newKey(t, "")
	ts.authorize(pub)
	srv.Auth, srv.KeyPath = "key", path
	if _, err := Run(context.Background(), srv, "", "uname"); err != nil && !strings.Contains(err.Error(), "exit") {
		t.Fatalf("key login failed: %v", err)
	}
}

func TestEncryptedKeyNeedsItsPassphrase(t *testing.T) {
	if testing.Short() {
		t.Skip("decrypting a key is deliberately slow, and slower still under -race")
	}
	ts, srv := startServer(t)
	_, pub, path := newKey(t, "open sesame")
	ts.authorize(pub)
	srv.Auth, srv.KeyPath = "key", path
	if _, err := Run(context.Background(), srv, "open sesame", "uname"); err != nil && !strings.Contains(err.Error(), "exit") {
		t.Fatalf("right passphrase: %v", err)
	}
	for _, pass := range []string{"", "wrong"} {
		_, err := Run(context.Background(), srv, pass, "uname")
		if err == nil || !strings.Contains(err.Error(), "passphrase") {
			t.Errorf("passphrase %q: want a passphrase error, got %v", pass, err)
		}
	}
}

func TestAKeyTheServerDoesNotKnowIsRefusedClearly(t *testing.T) {
	_, srv := startServer(t)
	_, _, path := newKey(t, "")
	srv.Auth, srv.KeyPath = "key", path
	_, err := Run(context.Background(), srv, "", "uname")
	if err == nil || !strings.Contains(err.Error(), "did not accept") {
		t.Fatalf("got %v", err)
	}
}

func TestMissingKeyFileNamesThePath(t *testing.T) {
	_, srv := startServer(t)
	srv.Auth, srv.KeyPath = "key", "/nonexistent/id_x"
	_, err := Run(context.Background(), srv, "", "uname")
	if err == nil || !strings.Contains(err.Error(), "/nonexistent/id_x") {
		t.Fatalf("got %v", err)
	}
}

func TestKeyFromTheSSHAgent(t *testing.T) {
	ts, srv := startServer(t)
	priv, pub, _ := newKey(t, "")
	ts.authorize(pub)
	kr := agent.NewKeyring()
	if err := kr.Add(agent.AddedKey{PrivateKey: priv}); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(t.TempDir(), "agent.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go agent.ServeAgent(kr, c)
		}
	}()
	t.Setenv("SSH_AUTH_SOCK", sock)
	srv.Auth, srv.KeyPath = "key", ""
	if _, err := Run(context.Background(), srv, "", "uname"); err != nil && !strings.Contains(err.Error(), "exit") {
		t.Fatalf("agent login failed: %v", err)
	}
}

func TestNoAgentAndNoKeyFileExplainsWhatToDo(t *testing.T) {
	_, srv := startServer(t)
	t.Setenv("SSH_AUTH_SOCK", "")
	srv.Auth, srv.KeyPath = "key", ""
	_, err := Run(context.Background(), srv, "", "uname")
	if err == nil || !strings.Contains(err.Error(), "ssh-agent") {
		t.Fatalf("got %v", err)
	}
}
