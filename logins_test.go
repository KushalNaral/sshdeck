package main

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/store"
)

type clip struct {
	mu sync.Mutex
	v  string
}

func (c *clip) set(s string) error { c.mu.Lock(); c.v = s; c.mu.Unlock(); return nil }
func (c *clip) get() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.v, nil
}

func loginRig(t *testing.T) (*App, *clip, *trail, store.Login) {
	t.Helper()
	a, _, _ := rig(t, okOpen(newShell()))
	c, tr := &clip{}, &trail{}
	a.clipSet, a.clipGet, a.clipTTL, a.audit = c.set, c.get, 40*time.Millisecond, tr.add
	l, err := a.SaveLogin("direct", store.Login{Label: "Main DB", Kind: "db", User: "root"}, "db-secret-9")
	if err != nil {
		t.Fatal(err)
	}
	return a, c, tr, l
}

func TestCopyLoginPasswordClearsItselfButOnlyIfUnchanged(t *testing.T) {
	a, c, tr, l := loginRig(t)
	if err := a.CopyLogin("direct", l.ID, "secret"); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.get(); v != "db-secret-9" {
		t.Fatalf("clipboard = %q", v)
	}
	time.Sleep(150 * time.Millisecond)
	if v, _ := c.get(); v != "" {
		t.Errorf("clipboard still holds %q after the timeout", v)
	}
	// If the user copied something else meanwhile, leave it alone.
	_ = a.CopyLogin("direct", l.ID, "secret")
	_ = c.set("something else")
	time.Sleep(150 * time.Millisecond)
	if v, _ := c.get(); v != "something else" {
		t.Errorf("clipboard = %q, the timer must not wipe other content", v)
	}
	if strings.Contains(tr.all(), "db-secret-9") {
		t.Error("secret reached the audit trail")
	}
	if tr.find("login.copy") == nil {
		t.Error("copy not audited")
	}
}

func TestCopyLoginUserDoesNotClear(t *testing.T) {
	a, c, _, l := loginRig(t)
	if err := a.CopyLogin("direct", l.ID, "user"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if v, _ := c.get(); v != "root" {
		t.Errorf("clipboard = %q, want the user name to stay", v)
	}
	if err := a.CopyLogin("direct", l.ID, "nonsense"); err == nil {
		t.Error("unknown field accepted")
	}
	if err := a.CopyLogin("direct", "nope", "user"); err == nil {
		t.Error("unknown login accepted")
	}
}

func loginFor(label string) store.Login { return store.Login{Label: label, Kind: "db", User: "root"} }

func TestSSHCommand(t *testing.T) {
	for _, c := range []struct {
		sv   store.Server
		want string
	}{
		{store.Server{User: "deploy", Host: "10.0.0.5", Port: 22}, "ssh deploy@10.0.0.5"},
		{store.Server{User: "deploy", Host: "h.example.org", Port: 2222}, "ssh -p 2222 deploy@h.example.org"},
		{store.Server{User: "u", Host: "h", Port: 22, Auth: "key", KeyPath: "/home/me/.ssh/id ed25519"}, "ssh -i '/home/me/.ssh/id ed25519' u@h"},
		{store.Server{User: "u", Host: "h", Port: 22, Auth: "key", KeyPath: "/k'x"}, `ssh -i '/k'\''x' u@h`},
	} {
		if got := sshCommand(c.sv); got != c.want {
			t.Errorf("sshCommand = %q, want %q", got, c.want)
		}
	}
}

func TestCopySSHCommandPutsItOnTheClipboardWithoutAnySecret(t *testing.T) {
	a, c, _, _ := loginRig(t)
	if err := a.CopySSHCommand("direct"); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.get(); v != "ssh u@h" {
		t.Errorf("clipboard = %q", v)
	}
	if err := a.CopySSHCommand("nope"); err == nil {
		t.Error("unknown server accepted")
	}
}
