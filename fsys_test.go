package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"

	"github.com/KushalNaral/sshdeck/internal/history"
)

// fakeDocker answers the docker commands sshdeck sends by running the part after "docker exec NAME" in a local
// shell, so the quoting, find/stat parsing and in-place writes are exercised for real. Paths are the temp
// directory's own, standing in for the container's filesystem.
func fakeDocker(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer) error {
	switch {
	case strings.HasPrefix(cmd, "docker inspect"):
		stdout.Write([]byte("true 4242\n")) // a pid whose /proc/<pid>/root this machine cannot give us
		return nil
	case strings.HasPrefix(cmd, "docker ps"):
		stdout.Write([]byte("web\ndb\n"))
		return nil
	}
	rest, ok := strings.CutPrefix(cmd, "docker exec ")
	if !ok {
		return errors.New("unexpected command " + cmd)
	}
	rest = strings.TrimPrefix(rest, "-i ")
	_, rest, _ = strings.Cut(rest, " ") // the container name
	c := exec.CommandContext(ctx, "sh", "-c", rest)
	c.Stdin, c.Stdout = stdin, stdout
	var eb bytes.Buffer
	c.Stderr = &eb
	if err := c.Run(); err != nil {
		if m := strings.TrimSpace(eb.String()); m != "" {
			return errors.New(m)
		}
		return err
	}
	return nil
}

type execFn func(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer) error

func (f execFn) Exec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer) error {
	return f(ctx, cmd, stdin, stdout)
}

func TestDockerFSListsReadsAndWritesInPlace(t *testing.T) {
	d := dockerFS{ex: execFn(fakeDocker), name: "web"}
	dir := t.TempDir()
	odd := "it's a \"file\" $HOME.txt"
	write(t, filepath.Join(dir, odd), "hello")
	write(t, filepath.Join(dir, "sub", "x"), "x")
	os.Symlink("sub", filepath.Join(dir, "link"))

	infos, err := d.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]os.FileInfo{}
	for _, i := range infos {
		got[i.Name()] = i
	}
	if f := got[odd]; f == nil || f.Size() != 5 || f.IsDir() {
		t.Errorf("odd name: %+v", f)
	}
	if !got["sub"].IsDir() || got["link"].Mode()&os.ModeSymlink == 0 {
		t.Errorf("sub/link: %v %v", got["sub"], got["link"])
	}
	if fi, err := d.Stat(filepath.Join(dir, "link")); err != nil || !fi.IsDir() {
		t.Errorf("stat follows links: %v %v", fi, err)
	}
	if _, err := d.Stat(filepath.Join(dir, "nope")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing file: %v", err)
	}

	r, err := d.Open(filepath.Join(dir, odd))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(r)
	r.Close()
	if string(b) != "hello" {
		t.Errorf("read %q", b)
	}

	// Writing keeps the same file (owner, mode and inode), which is the point of in-place.
	p := filepath.Join(dir, "conf")
	write(t, p, "old")
	os.Chmod(p, 0o640)
	before, _ := os.Stat(p)
	w, _ := d.Create(p)
	w.Write([]byte("new text"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(p)
	if !os.SameFile(before, after) || after.Mode().Perm() != 0o640 {
		t.Error("the file was replaced instead of written in place")
	}
	if b, _ := os.ReadFile(p); string(b) != "new text" {
		t.Errorf("content %q", b)
	}
	if w, _ := d.CreateNew(p); w != nil {
		w.Write([]byte("clobber"))
		if err := w.Close(); err == nil {
			t.Error("CreateNew must not write over an existing file")
		}
	}
	if b, _ := os.ReadFile(p); string(b) != "new text" {
		t.Errorf("the existing file was changed: %q", b)
	}
	if w, _ := d.Create(filepath.Join(dir, "nodir", "f")); w != nil {
		w.Write([]byte("x"))
		if err := w.Close(); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("writing into a missing folder: %v", err)
		}
	}
}

func TestDockerFSChangesTheTree(t *testing.T) {
	d := dockerFS{ex: execFn(fakeDocker), name: "web"}
	dir := t.TempDir()
	if err := d.MkdirAll(filepath.Join(dir, "a", "b")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "a", "f"), "1")
	if err := d.Rename(filepath.Join(dir, "a", "f"), filepath.Join(dir, "a", "g")); err != nil {
		t.Fatal(err)
	}
	if err := d.Chmod(filepath.Join(dir, "a", "g"), 0o600); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "a", "g")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if err := d.Remove(filepath.Join(dir, "a", "g")); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove(filepath.Join(dir, "a", "b")); err != nil { // an empty folder
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "a", "c", "d"), "1")
	if err := d.RemoveAll(filepath.Join(dir, "a")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a")); !os.IsNotExist(err) {
		t.Error("RemoveAll left it")
	}
}

func TestProcFSStaysInsideTheContainerRoot(t *testing.T) {
	c1, c2 := net.Pipe()
	srv, _ := sftp.NewServer(c1)
	go srv.Serve()
	cl, _ := sftp.NewClientPipe(c2, c2)
	t.Cleanup(func() { cl.Close(); srv.Close() })
	root := t.TempDir() // stands for /proc/<pid>/root
	write(t, filepath.Join(root, "etc", "app.conf"), "in the container")
	write(t, filepath.Join(filepath.Dir(root), "outside.txt"), "host file")
	f := procFS{in: sftpFS{cl}, root: root}

	r, err := f.Open("/etc/app.conf")
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := io.ReadAll(r); string(b) != "in the container" {
		t.Errorf("read %q", b)
	}
	r.Close()
	if _, err := f.Stat("/../outside.txt"); err == nil {
		t.Error(".. must not climb out of the container root")
	}
	if p, err := f.RealPath("/etc/../etc"); err != nil || p != "/etc" {
		t.Errorf("RealPath = %q %v", p, err)
	}
	if !f.InPlace() {
		t.Error("container writes are in place")
	}
}

func TestEditingAFileInsideAContainer(t *testing.T) {
	a, _ := rigFilesHome(t)
	a.liveClientFor("direct").exec = fakeDocker
	dir := t.TempDir()
	p := filepath.Join(dir, "app.env")
	write(t, p, "A=1\n")
	os.Chmod(p, 0o600)
	before, _ := os.Stat(p)

	names, err := a.Containers("direct")
	if err != nil || strings.Join(names, ",") != "db,web" {
		t.Fatalf("%v %v", names, err)
	}
	l, err := a.ListDir("direct@web", dir)
	if err != nil || len(l.Entries) != 1 || l.Entries[0].Name != "app.env" {
		t.Fatalf("%+v %v", l, err)
	}
	f, err := a.ReadText("direct@web", p)
	if err != nil || f.Content != "A=1\n" {
		t.Fatalf("%+v %v", f, err)
	}
	if _, err := a.WriteText("direct@web", p, "A=2\n", f.Version); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(p)
	if b, _ := os.ReadFile(p); string(b) != "A=2\n" || !os.SameFile(before, after) || after.Mode().Perm() != 0o600 {
		t.Errorf("content %q, same file %v, mode %v", b, os.SameFile(before, after), after.Mode().Perm())
	}
	if m, _ := filepath.Glob(p + "*"); len(m) != 1 {
		t.Errorf("a neighbour file was left: %v", m)
	}

	// History is kept apart from the server's own file of the same path.
	vs, _ := a.FileHistory("direct@web", p)
	if len(vs) != 2 || vs[0].Path != "docker:web:"+p || vs[0].Server != "direct" {
		t.Fatalf("%+v", vs)
	}
	if own, _ := a.FileHistory("direct", p); len(own) != 0 {
		t.Error("the container file's history must not show up for the server's file")
	}
	if _, err := a.RestoreVersion("direct@web", p, vs[1].ID); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "A=1\n" {
		t.Errorf("restored %q", b)
	}

	// Upload and download go through the same path.
	up := filepath.Join(t.TempDir(), "up.txt")
	write(t, up, "uploaded")
	if _, err := a.Upload("direct@web", dir, []string{up}, "ask"); err != nil {
		t.Fatal(err)
	}
	for _, x := range settle(t, a) {
		if x.State != "done" {
			t.Errorf("%+v", x)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "up.txt")); string(b) != "uploaded" {
		t.Errorf("uploaded %q", b)
	}
	back := t.TempDir()
	a.Download("direct@web", []string{filepath.Join(dir, "up.txt")}, back, "ask")
	settle(t, a)
	if b, _ := os.ReadFile(filepath.Join(back, "up.txt")); string(b) != "uploaded" {
		t.Errorf("downloaded %q", b)
	}
}

func TestContainerNamesAreChecked(t *testing.T) {
	a := rigFiles(t)
	a.liveClientFor("direct").exec = fakeDocker
	for _, bad := range []string{"a;b", "../x", "a b", "$(id)"} {
		if _, err := a.ListDir("direct@"+bad, "/"); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
	_ = history.Save
}
