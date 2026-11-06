package history

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

// ftp starts an SFTP server over a pipe, working in a temp directory used as the login's home.
func ftp(t *testing.T) (*sftp.Client, string) {
	t.Helper()
	home := t.TempDir()
	c1, c2 := net.Pipe()
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
	return cl, home
}

func TestRemoteCopyRoundTrip(t *testing.T) {
	cl, home := ftp(t)
	s, _ := open(t)
	v, _ := rec(t, s, "/etc/app.conf", "port = 1\n", Save)
	if err := WriteRemote(cl, v, []byte("port = 1\n")); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(home, ".sshdeck", "history")
	for _, d := range []string{filepath.Join(home, ".sshdeck"), root, filepath.Join(root, pathKey("/etc/app.conf"))} {
		if fi, err := os.Stat(d); err != nil || fi.Mode().Perm() != 0o700 {
			t.Errorf("%s: %v %v", d, fi, err)
		}
	}
	m, _ := filepath.Glob(filepath.Join(root, pathKey("/etc/app.conf"), "*"))
	for _, f := range m {
		if fi, _ := os.Stat(f); !fi.IsDir() && fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", f, fi.Mode().Perm())
		}
	}
	got, err := ListRemote(cl, "srv", "/etc/app.conf")
	if err != nil || len(got) != 1 || got[0].ID != v.ID || got[0].Author != "rome@laptop" || !got[0].Remote {
		t.Fatalf("%+v %v", got, err)
	}
	b, err := ContentRemote(cl, got[0])
	if err != nil || string(b) != "port = 1\n" {
		t.Fatalf("%q %v", b, err)
	}
	all, _ := RecentRemote(cl, "srv", 10)
	if len(all) != 1 {
		t.Errorf("recent: %+v", all)
	}
}

func TestTeammatesVersionsAreImportedAndFetched(t *testing.T) {
	cl, _ := ftp(t)
	mine, _ := open(t)
	theirs, _ := open(t) // another person's own local store
	v, _ := rec(t, theirs, "/etc/app.conf", "their edit", Save)
	v.Author = "sam@desk"
	WriteRemote(cl, v, []byte("their edit"))

	rv, _ := ListRemote(cl, "srv", "/etc/app.conf")
	if added, err := mine.Import(rv[0]); err != nil || !added {
		t.Fatalf("%v %v", added, err)
	}
	if added, _ := mine.Import(rv[0]); added {
		t.Error("importing twice must not duplicate")
	}
	l := mine.List("srv", "/etc/app.conf")
	if len(l) != 1 || !l[0].Remote || l[0].Author != "sam@desk" {
		t.Fatalf("%+v", l)
	}
	if _, err := mine.Content("srv", l[0].ID); err == nil {
		t.Error("the text is not here yet")
	}
	b, _ := ContentRemote(cl, l[0])
	if err := mine.Fetched("srv", l[0].ID, b); err != nil {
		t.Fatal(err)
	}
	if got, _ := mine.Content("srv", l[0].ID); string(got) != "their edit" {
		t.Errorf("content = %q", got)
	}
	if err := mine.Fetched("srv", l[0].ID, []byte("tampered")); err == nil {
		t.Error("text that does not match the record must be refused")
	}
}

func TestRemotePruneAndDelete(t *testing.T) {
	cl, home := ftp(t)
	s, _ := open(t)
	now := time.Now()
	for i := 0; i < 5; i++ {
		time.Sleep(2 * time.Millisecond)
		v, _ := rec(t, s, "/a", strings.Repeat("x", i+1), Save)
		WriteRemote(cl, v, []byte("x"))
	}
	PruneRemote(cl, "/a", 2, 90, now)
	got, _ := ListRemote(cl, "srv", "/a")
	if len(got) != 2 {
		t.Fatalf("kept %d", len(got))
	}
	if err := DeleteRemote(cl, "/a"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, ".sshdeck", "history", pathKey("/a"))); !os.IsNotExist(err) {
		t.Error("the file's folder should be gone")
	}
	if err := DeleteRemote(cl, ""); err != nil {
		t.Fatal(err)
	}
}
