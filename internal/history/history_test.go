package history

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	s, err := Open(dir, bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	return s, dir
}

func rec(t *testing.T, s *Store, path, body string, kind Kind) (Version, bool) {
	t.Helper()
	v, added, err := s.Record(Version{Server: "srv", Path: path, Author: "rome@laptop", Login: "deploy", Kind: kind}, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return v, added
}

func TestRecordAndReadBack(t *testing.T) {
	s, _ := open(t)
	v, added := rec(t, s, "/etc/app.conf", "one\n", Save)
	if !added || v.ID == "" || v.Size != 4 || v.Hash == "" {
		t.Fatalf("%+v %v", v, added)
	}
	got, err := s.Content("srv", v.ID)
	if err != nil || string(got) != "one\n" {
		t.Fatalf("%q %v", got, err)
	}
	rec(t, s, "/etc/app.conf", "two\n", Save)
	l := s.List("srv", "/etc/app.conf")
	if len(l) != 2 || l[0].Size != 4 || l[0].Time < l[1].Time {
		t.Fatalf("newest first: %+v", l)
	}
	if lv, ok := s.Latest("srv", "/etc/app.conf"); !ok || lv.ID != l[0].ID {
		t.Errorf("latest = %+v", lv)
	}
}

func TestSameContentIsNotAVersionTwice(t *testing.T) {
	s, dir := open(t)
	rec(t, s, "/a", "same", Save)
	if _, added := rec(t, s, "/a", "same", Save); added {
		t.Error("saving unchanged content must not add a version")
	}
	// Another file with the same content shares one blob.
	rec(t, s, "/b", "same", Save)
	n := 0
	filepath.WalkDir(filepath.Join(dir, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			n++
		}
		return nil
	})
	if n != 1 {
		t.Errorf("blobs = %d, want 1", n)
	}
}

func TestNothingReadableOnDisk(t *testing.T) {
	s, dir := open(t)
	rec(t, s, "/home/deploy/.env", "DB_PASSWORD=hunter2\n", Save)
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, secret := range []string{"hunter2", ".env", "deploy", "rome"} {
			if strings.Contains(string(b), secret) || strings.Contains(p, "hunter2") {
				t.Errorf("%s leaks %q", p, secret)
			}
		}
		if fi, _ := d.Info(); fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", p, fi.Mode().Perm())
		}
		return nil
	})
	if _, err := Open(dir, bytes.Repeat([]byte{9}, 32)); err != nil {
		t.Fatal(err)
	}
	other, _ := Open(dir, bytes.Repeat([]byte{9}, 32))
	if len(other.List("srv", "/home/deploy/.env")) != 0 {
		t.Error("a different key must not read the history")
	}
}

func TestPruneByCountAndAge(t *testing.T) {
	s, _ := open(t)
	s.Keep, s.Days = 3, 30
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	// an old one, then five recent ones
	s.now = func() time.Time { return now.AddDate(0, 0, -60) }
	old, _ := rec(t, s, "/a", "ancient", Save)
	s.now = func() time.Time { return now }
	for _, b := range []string{"1", "2", "3", "4", "5"} {
		now = now.Add(time.Minute)
		rec(t, s, "/a", b, Save)
	}
	l := s.List("srv", "/a")
	if len(l) != 3 {
		t.Fatalf("kept %d, want 3: %+v", len(l), l)
	}
	if _, err := s.Content("srv", old.ID); err == nil {
		t.Error("the old version's content should be gone")
	}
}

func TestPinnedAndNewestSurvivePruning(t *testing.T) {
	s, _ := open(t)
	s.Keep, s.Days = 2, 30
	first, _ := rec(t, s, "/a", "first", Save)
	if err := s.Pin("srv", first.ID, true, "before the upgrade"); err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{"2", "3", "4"} {
		time.Sleep(2 * time.Millisecond)
		rec(t, s, "/a", b, Save)
	}
	l := s.List("srv", "/a")
	var pinned bool
	for _, v := range l {
		if v.ID == first.ID && v.Pinned && v.Note == "before the upgrade" {
			pinned = true
		}
	}
	if !pinned || len(l) != 3 { // newest 2 plus the pinned one
		t.Fatalf("%d versions, pinned kept = %v", len(l), pinned)
	}
	// Even with everything expired the newest stays.
	s.Keep, s.Days = 1, 1
	s.now = func() time.Time { return time.Now().AddDate(1, 0, 0) }
	rec(t, s, "/a", "5", Save)
	if len(s.List("srv", "/a")) < 2 {
		t.Error("newest and pinned must stay")
	}
}

func TestRecentAcrossFilesAndDelete(t *testing.T) {
	s, dir := open(t)
	rec(t, s, "/a", "a1", Save)
	time.Sleep(2 * time.Millisecond)
	rec(t, s, "/b", "b1", Outside)
	r := s.Recent("srv", 10)
	if len(r) != 2 || r[0].Path != "/b" || r[0].Kind != Outside {
		t.Fatalf("%+v", r)
	}
	if err := s.Delete("srv", "/a"); err != nil {
		t.Fatal(err)
	}
	if len(s.List("srv", "/a")) != 0 || len(s.List("srv", "/b")) != 1 {
		t.Error("delete removed the wrong file")
	}
	s.Delete("srv", "")
	if len(s.Recent("srv", 10)) != 0 {
		t.Error("deleting the server's history must empty it")
	}
	left := 0
	filepath.WalkDir(filepath.Join(dir, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			left++
		}
		return nil
	})
	if left != 0 {
		t.Errorf("%d blobs left behind", left)
	}
}

func TestDenied(t *testing.T) {
	for _, p := range []string{"/home/u/.ssh/id_rsa", "/home/u/.ssh/id_ed25519", "/etc/ssl/key.pem", "/etc/shadow", "/x/server.KEY", "/x/vault.kdbx"} {
		if !Denied(p, nil) {
			t.Errorf("%s should be skipped", p)
		}
	}
	for _, p := range []string{"/etc/nginx/nginx.conf", "/var/www/.env", "/home/u/keys.md"} {
		if Denied(p, nil) {
			t.Errorf("%s should be kept", p)
		}
	}
	if !Denied("/var/www/.env", []string{".env"}) {
		t.Error("custom patterns apply")
	}
}
