package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

// rigFiles connects the "direct" server and gives its client an in-process SFTP server that works on the real
// disk, so remote paths are ordinary temp-dir paths.
func rigFiles(t *testing.T) *App {
	a, _ := rigFilesHome(t)
	return a
}

// rigFilesHome is rigFiles that also says which folder is the login's home on the fake server.
func rigFilesHome(t *testing.T) (*App, string) {
	t.Helper()
	a, _, _, d := rigDial(t, okOpen(newShell()))
	a.ctx = context.Background()
	a.histDir = filepath.Join(t.TempDir(), "history")
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	c1, c2 := net.Pipe()
	home := t.TempDir()
	srv, err := sftp.NewServer(c1, sftp.WithServerWorkingDirectory(home))
	if err != nil {
		t.Fatal(err)
	}
	go srv.Serve()
	cl, err := sftp.NewClientPipe(c2, c2)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cl.Close(); srv.Close() })
	d.last().ftp = cl
	return a, home
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func settle(t *testing.T, a *App) []Transfer {
	t.Helper()
	for i := 0; i < 400; i++ {
		ts := a.Transfers()
		busy := false
		for _, x := range ts {
			if x.State == "queued" || x.State == "running" {
				busy = true
			}
		}
		if !busy {
			return ts
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("transfers never finished")
	return nil
}

func TestListDirSortsFoldersFirst(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	write(t, filepath.Join(dir, "b.txt"), "x")
	write(t, filepath.Join(dir, "A.txt"), "x")
	write(t, filepath.Join(dir, "sub", "c"), "x")
	l, err := a.ListDir("direct", dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range l.Entries {
		names = append(names, e.Name)
	}
	if got := strings.Join(names, ","); got != "sub,A.txt,b.txt" {
		t.Errorf("order = %s", got)
	}
	if _, err := a.ListDir("direct", filepath.Join(dir, "nope")); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("missing folder: %v", err)
	}
	if _, err := a.ListDir("a", dir); err == nil {
		t.Error("a server that is not connected must not list")
	}
}

func TestEditSavesAndNoticesOutsideChanges(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	p := filepath.Join(dir, "app.conf")
	write(t, p, "one\n")
	os.Chmod(p, 0o600)
	f, err := a.ReadText("direct", p)
	if err != nil || f.Content != "one\n" {
		t.Fatalf("%+v %v", f, err)
	}
	v, err := a.WriteText("direct", p, "two\n", f.Version)
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "two\n" {
		t.Errorf("file = %q", b)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("permissions changed to %v", fi.Mode().Perm())
	}
	if m, _ := filepath.Glob(p + "*"); len(m) != 1 {
		t.Errorf("temp file left behind: %v", m)
	}
	// someone else edits it, then we save with the old version
	time.Sleep(5 * time.Millisecond)
	write(t, p, "theirs, longer\n")
	if _, err := a.WriteText("direct", p, "mine\n", v); err == nil || !strings.Contains(err.Error(), ErrChanged) {
		t.Fatalf("expected a conflict, got %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "theirs, longer\n" {
		t.Errorf("a refused save must not write, file = %q", b)
	}
}

func TestReadTextRefusesBinaryAndBig(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	write(t, filepath.Join(dir, "bin"), "ab\x00cd")
	write(t, filepath.Join(dir, "big"), strings.Repeat("x", maxEdit+1))
	for _, n := range []string{"bin", "big"} {
		if _, err := a.ReadText("direct", filepath.Join(dir, n)); err == nil {
			t.Errorf("%s should be refused", n)
		}
	}
}

func TestRenameNeverReplacesAndDeleteGuardsRoot(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	write(t, filepath.Join(dir, "a"), "A")
	write(t, filepath.Join(dir, "b"), "B")
	if err := a.Rename("direct", filepath.Join(dir, "a"), filepath.Join(dir, "b")); err == nil {
		t.Error("rename over an existing file must fail")
	}
	if err := a.Rename("direct", filepath.Join(dir, "a"), filepath.Join(dir, "c")); err != nil {
		t.Fatal(err)
	}
	if err := a.Delete("direct", []string{"/"}); err == nil {
		t.Error("deleting / must be refused")
	}
	write(t, filepath.Join(dir, "d", "x", "y"), "1")
	if err := a.Delete("direct", []string{filepath.Join(dir, "d")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "d")); !os.IsNotExist(err) {
		t.Error("folder should be gone")
	}
	if err := a.NewFile("direct", filepath.Join(dir, "b")); err == nil {
		t.Error("NewFile must not truncate an existing file")
	}
}

func TestUploadFolderAndDownloadItBack(t *testing.T) {
	a := rigFiles(t)
	src, remote, back := t.TempDir(), t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "site", "index.php"), "<?php")
	write(t, filepath.Join(src, "site", "css", "a.css"), "body{}")
	r, err := a.Upload("direct", remote, []string{filepath.Join(src, "site")}, "ask")
	if err != nil || r.Started != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	for _, x := range settle(t, a) {
		if x.State != "done" || x.Total != 11 || x.Done != 11 {
			t.Errorf("%+v", x)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(remote, "site", "css", "a.css")); string(b) != "body{}" {
		t.Errorf("uploaded file = %q", b)
	}
	if m, _ := filepath.Glob(filepath.Join(remote, "site", "*.sshdeck-part")); len(m) != 0 {
		t.Errorf("temp files left: %v", m)
	}
	if _, err := a.Download("direct", []string{filepath.Join(remote, "site")}, back, "ask"); err != nil {
		t.Fatal(err)
	}
	settle(t, a)
	if b, _ := os.ReadFile(filepath.Join(back, "site", "index.php")); string(b) != "<?php" {
		t.Errorf("downloaded file = %q", b)
	}
}

func TestConflictPolicies(t *testing.T) {
	a, remote := rigFiles(t), t.TempDir()
	src := t.TempDir()
	write(t, filepath.Join(src, "f.txt"), "new")
	write(t, filepath.Join(remote, "f.txt"), "old")
	l := []string{filepath.Join(src, "f.txt")}
	r, _ := a.Upload("direct", remote, l, "ask")
	if r.Started != 0 || len(r.Conflicts) != 1 || r.Conflicts[0] != "f.txt" {
		t.Fatalf("ask: %+v", r)
	}
	if b, _ := os.ReadFile(filepath.Join(remote, "f.txt")); string(b) != "old" {
		t.Fatal("ask must not touch anything")
	}
	if r, _ = a.Upload("direct", remote, l, "skip"); r.Started != 0 {
		t.Errorf("skip: %+v", r)
	}
	a.Upload("direct", remote, l, "keep")
	settle(t, a)
	if b, _ := os.ReadFile(filepath.Join(remote, "f (2).txt")); string(b) != "new" {
		t.Error("keep should write f (2).txt")
	}
	a.Upload("direct", remote, l, "replace")
	settle(t, a)
	if b, _ := os.ReadFile(filepath.Join(remote, "f.txt")); string(b) != "new" {
		t.Error("replace should overwrite")
	}
}

func TestHostileNamesAreNotDownloaded(t *testing.T) {
	for _, n := range []string{"", ".", "..", "a/b", `a\b`, "x\x00y"} {
		if safeName(n) {
			t.Errorf("%q must not be accepted", n)
		}
	}
	if !safeName("report (1).pdf") {
		t.Error("ordinary names must pass")
	}
}

func TestCancelStopsATransfer(t *testing.T) {
	a := rigFiles(t)
	a.xsem = make(chan struct{}, 1)
	a.xsem <- struct{}{} // hold the only slot so the transfer stays queued
	src, remote := t.TempDir(), t.TempDir()
	write(t, filepath.Join(src, "big"), "data")
	a.Upload("direct", remote, []string{filepath.Join(src, "big")}, "ask")
	id := a.Transfers()[0].ID
	a.CancelTransfer(id)
	<-a.xsem
	ts := settle(t, a)
	if ts[0].State != "cancelled" {
		t.Errorf("state = %s", ts[0].State)
	}
	if _, err := os.Stat(filepath.Join(remote, "big")); err == nil {
		t.Error("a cancelled upload must not leave a file")
	}
	a.RetryTransfer(id)
	if ts = settle(t, a); len(ts) != 2 || ts[1].State != "done" {
		t.Errorf("retry: %+v", ts)
	}
}
