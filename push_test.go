package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPushCopiesToConnectedServersAndSkipsTheRest(t *testing.T) {
	a, remote := rigFiles(t), t.TempDir()
	src := t.TempDir()
	write(t, filepath.Join(src, "app.conf"), "v2")

	res, err := a.PushTo([]string{"direct", "ghost"}, remote, []string{filepath.Join(src, "app.conf")}, "skip")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].Started != 1 || res[0].Error != "" {
		t.Fatalf("connected server: %+v", res)
	}
	if res[1].Started != 0 || res[1].Error == "" {
		t.Fatalf("a server that is not connected must be reported, not dialled: %+v", res[1])
	}
	settle(t, a)
	if b, _ := os.ReadFile(filepath.Join(remote, "app.conf")); string(b) != "v2" {
		t.Errorf("file not copied: %q", b)
	}
}

func TestPushPoliciesAreChosenUpFront(t *testing.T) {
	a, remote := rigFiles(t), t.TempDir()
	src := t.TempDir()
	write(t, filepath.Join(src, "f.txt"), "new")
	write(t, filepath.Join(remote, "f.txt"), "old")
	l := []string{filepath.Join(src, "f.txt")}

	if _, err := a.PushTo([]string{"direct"}, remote, l, "ask"); err == nil {
		t.Error("ask cannot work across many servers and must be refused")
	}
	res, _ := a.PushTo([]string{"direct"}, remote, l, "skip")
	if res[0].Started != 0 || len(res[0].Conflicts) != 1 || res[0].Conflicts[0] != "f.txt" {
		t.Errorf("skip should report what it left alone: %+v", res[0])
	}
	if b, _ := os.ReadFile(filepath.Join(remote, "f.txt")); string(b) != "old" {
		t.Error("skip must not touch the file")
	}
	a.PushTo([]string{"direct"}, remote, l, "replace")
	settle(t, a)
	if b, _ := os.ReadFile(filepath.Join(remote, "f.txt")); string(b) != "new" {
		t.Error("replace should overwrite")
	}
}

func TestPushRejectsBadInput(t *testing.T) {
	a := rigFiles(t)
	f := []string{"/etc/hostname"}
	for name, c := range map[string]struct {
		ids  []string
		dir  string
		l    []string
		want string
	}{
		"no servers": {nil, "/tmp", f, "server"},
		"no files":   {[]string{"direct"}, "/tmp", nil, "file"},
		"empty dir":  {[]string{"direct"}, "", f, ""},
		"nul in dir": {[]string{"direct"}, "/tmp/a\x00b", f, ""},
	} {
		if _, err := a.PushTo(c.ids, c.dir, c.l, "skip"); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}

func TestPushIsAudited(t *testing.T) {
	a, remote := rigFiles(t), t.TempDir()
	tr := &trail{}
	a.audit = tr.add
	src := t.TempDir()
	write(t, filepath.Join(src, "x"), "1")
	a.PushTo([]string{"direct"}, remote, []string{filepath.Join(src, "x")}, "skip")
	settle(t, a)
	if tr.find("push") == nil {
		t.Error("expected a push audit entry")
	}
}
