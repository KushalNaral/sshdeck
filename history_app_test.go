package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/history"
)

func kinds(vs []history.Version) []string {
	var k []string
	for i := len(vs) - 1; i >= 0; i-- { // oldest first
		k = append(k, string(vs[i].Kind))
	}
	return k
}

func TestFirstEditCanBeUndone(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	p := filepath.Join(dir, "app.conf")
	write(t, p, "original\n")
	f, _ := a.ReadText("direct", p)
	v, err := a.WriteText("direct", p, "edited\n", f.Version)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(3 * time.Millisecond)
	if _, err := a.WriteText("direct", p, "edited again\n", v); err != nil {
		t.Fatal(err)
	}
	l, _ := a.FileHistory("direct", p)
	if got := strings.Join(kinds(l), ","); got != "initial,save,save" {
		t.Fatalf("kinds = %s", got)
	}
	first, _ := a.HistoryContent("direct", l[2].ID)
	if first != "original\n" {
		t.Errorf("initial = %q", first)
	}
	if !strings.Contains(l[0].Author, "@") || l[0].Login != "u" {
		t.Errorf("who: %+v", l[0])
	}
	if l[2].Author != "" {
		t.Errorf("the initial version was not written by anyone here: %+v", l[2])
	}
}

func TestOutsideChangeIsNoticedOnOpen(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	p := filepath.Join(dir, "a.txt")
	write(t, p, "one")
	a.ReadText("direct", p)
	time.Sleep(3 * time.Millisecond)
	write(t, p, "two, changed in a shell")
	a.ReadText("direct", p)
	l, _ := a.FileHistory("direct", p)
	if got := strings.Join(kinds(l), ","); got != "initial,outside" {
		t.Fatalf("kinds = %s", got)
	}
	if l[0].Author != "" {
		t.Error("an outside change has no known author")
	}
	a.ReadText("direct", p) // unchanged: nothing new
	if l2, _ := a.FileHistory("direct", p); len(l2) != 2 {
		t.Errorf("reading again added a version: %d", len(l2))
	}
}

func TestRestoreIsANewVersion(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	p := filepath.Join(dir, "r.txt")
	write(t, p, "v1")
	f, _ := a.ReadText("direct", p)
	time.Sleep(3 * time.Millisecond)
	a.WriteText("direct", p, "v2", f.Version)
	l, _ := a.FileHistory("direct", p)
	time.Sleep(3 * time.Millisecond)
	if _, err := a.RestoreVersion("direct", p, l[1].ID); err != nil { // l[1] is the initial v1
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "v1" {
		t.Errorf("file = %q", b)
	}
	l, _ = a.FileHistory("direct", p)
	if got := strings.Join(kinds(l), ","); got != "initial,save,restore" {
		t.Errorf("kinds = %s", got)
	}
}

func TestKeyFilesAreNeverRecorded(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	p := filepath.Join(dir, "server.pem")
	write(t, p, "-----BEGIN PRIVATE KEY-----")
	f, _ := a.ReadText("direct", p)
	if !f.NoHistory {
		t.Error("the editor should be told history is off for this file")
	}
	a.WriteText("direct", p, "changed", f.Version)
	if l, _ := a.FileHistory("direct", p); len(l) != 0 {
		t.Errorf("recorded %d versions of a key file", len(l))
	}
}

func TestSavingWorksWhenHistoryCannotBeStored(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	os.WriteFile(filepath.Join(t.TempDir(), "x"), nil, 0o600)
	blocker := filepath.Join(t.TempDir(), "blocker")
	write(t, blocker, "a file where the history folder should be")
	a.histDir = filepath.Join(blocker, "history")
	p := filepath.Join(dir, "a.txt")
	write(t, p, "x")
	f, _ := a.ReadText("direct", p)
	if _, err := a.WriteText("direct", p, "y", f.Version); err != nil {
		t.Fatalf("history trouble must not fail a save: %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "y" {
		t.Error("file not saved")
	}
}

func TestHistoryIsOffWhenSwitchedOff(t *testing.T) {
	a, dir := rigFiles(t), t.TempDir()
	a.settings.NoHistory = true
	p := filepath.Join(dir, "a.txt")
	write(t, p, "x")
	f, _ := a.ReadText("direct", p)
	a.WriteText("direct", p, "y", f.Version)
	if l, _ := a.FileHistory("direct", p); len(l) != 0 {
		t.Error("history is off")
	}
}

func TestServerCopyIsWrittenAndPulled(t *testing.T) {
	a, home := rigFilesHome(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "a.conf")
	write(t, p, "one")
	f, _ := a.ReadText("direct", p)
	a.WriteText("direct", p, "two", f.Version)
	root := filepath.Join(home, ".sshdeck", "history")
	if fi, err := os.Stat(root); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("server copy folder: %v %v", fi, err)
	}
	if m, _ := filepath.Glob(filepath.Join(root, "*", "*.json")); len(m) != 2 { // initial + save
		t.Fatalf("copies on the server: %v", m)
	}

	// A teammate's edit appears on the server; here it shows up in the list and its text is fetched on demand.
	other, _ := history.Open(t.TempDir(), make([]byte, 32))
	v, _, _ := other.Record(history.Version{Server: "direct", Path: p, Author: "sam@desk", Login: "u", Kind: history.Save}, []byte("sam was here"))
	cl, _ := a.ftp("direct")
	history.WriteRemote(cl, v, []byte("sam was here"))
	l, _ := a.FileHistory("direct", p)
	if len(l) != 3 || l[0].Author != "sam@desk" && l[1].Author != "sam@desk" && l[2].Author != "sam@desk" {
		t.Fatalf("%+v", l)
	}
	var theirs history.Version
	for _, x := range l {
		if x.Author == "sam@desk" {
			theirs = x
		}
	}
	if !theirs.Remote {
		t.Error("their text is not here yet")
	}
	if txt, err := a.HistoryContent("direct", theirs.ID); err != nil || txt != "sam was here" {
		t.Fatalf("%q %v", txt, err)
	}
	if l, _ = a.FileHistory("direct", p); func() bool {
		for _, x := range l {
			if x.ID == theirs.ID {
				return x.Remote
			}
		}
		return true
	}() {
		t.Error("after opening it, the text is stored here")
	}

	if err := a.DeleteHistory("direct", p); err != nil {
		t.Fatal(err)
	}
	if m, _ := filepath.Glob(filepath.Join(root, "*", "*.json")); len(m) != 0 {
		t.Errorf("the server copy should be gone: %v", m)
	}
}

func TestProductionServersNeedTheCopyTurnedOn(t *testing.T) {
	a, home := rigFilesHome(t)
	sv, _ := a.server("direct")
	sv.Production = true
	a.st.SaveServer(sv, "")
	p := filepath.Join(t.TempDir(), "a.conf")
	write(t, p, "one")
	f, _ := a.ReadText("direct", p)
	a.WriteText("direct", p, "two", f.Version)
	if _, err := os.Stat(filepath.Join(home, ".sshdeck")); err == nil {
		t.Fatal("nothing may be written to a production server's home before it is turned on")
	}
	if err := a.SetHistoryCopy("direct", "on"); err != nil {
		t.Fatal(err)
	}
	f, _ = a.ReadText("direct", p)
	a.WriteText("direct", p, "three", f.Version)
	if _, err := os.Stat(filepath.Join(home, ".sshdeck", "history")); err != nil {
		t.Errorf("turned on, it should write: %v", err)
	}
	if err := a.SetHistoryCopy("direct", "maybe"); err == nil {
		t.Error("bad value accepted")
	}
}

func TestServerCopyCanBeSwitchedOffEverywhere(t *testing.T) {
	a, home := rigFilesHome(t)
	a.settings.NoSharedHistory = true
	p := filepath.Join(t.TempDir(), "a.conf")
	write(t, p, "one")
	f, _ := a.ReadText("direct", p)
	a.WriteText("direct", p, "two", f.Version)
	if _, err := os.Stat(filepath.Join(home, ".sshdeck")); err == nil {
		t.Error("the server copy is off")
	}
	if l, _ := a.FileHistory("direct", p); len(l) != 2 {
		t.Errorf("local history still works: %d", len(l))
	}
}
