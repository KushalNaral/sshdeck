package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func searchTree(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "it's a dir")
	write(t, filepath.Join(dir, "conf", "nginx.conf"), "server {\n  listen 80;\n  # the needle is here\n}\n")
	write(t, filepath.Join(dir, "app", "Readme.md"), "nothing\n")
	write(t, filepath.Join(dir, "app", "binary.bin"), "needle\x00\x01")
	return dir
}

func TestSearchByName(t *testing.T) {
	a := rigFiles(t)
	a.liveClientFor("direct").exec = shellExec
	dir := searchTree(t)
	hits, err := a.SearchFiles("direct", dir, "README", false)
	if err != nil || len(hits) != 1 || !strings.HasSuffix(hits[0].Path, "app/Readme.md") {
		t.Fatalf("names are matched without regard to case: %+v %v", hits, err)
	}
}

func TestSearchByContentGivesLines(t *testing.T) {
	a := rigFiles(t)
	a.liveClientFor("direct").exec = shellExec
	dir := searchTree(t)
	hits, err := a.SearchFiles("direct", dir, "needle", true)
	if err != nil || len(hits) != 1 {
		t.Fatalf("a binary file must not match: %+v %v", hits, err)
	}
	if h := hits[0]; !strings.HasSuffix(h.Path, "conf/nginx.conf") || h.Line != 3 || !strings.Contains(h.Text, "the needle is here") {
		t.Fatalf("%+v", h)
	}
}

func TestSearchTermsAreNeverShellSyntax(t *testing.T) {
	a := rigFiles(t)
	a.liveClientFor("direct").exec = shellExec
	dir := searchTree(t)
	mark := filepath.Join(t.TempDir(), "pwned")
	for _, q := range []string{"$(touch " + mark + ")", "`touch " + mark + "`", "'; touch " + mark + "; '", "a b"} {
		if _, err := a.SearchFiles("direct", dir, q, true); err != nil {
			t.Fatalf("%q: %v", q, err)
		}
		if _, err := a.SearchFiles("direct", dir, q, false); err != nil {
			t.Fatalf("%q: %v", q, err)
		}
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("a search term ran as a command")
	}
}

func TestSearchInsideAContainer(t *testing.T) {
	a := rigFiles(t)
	a.liveClientFor("direct").exec = fakeDocker
	dir := searchTree(t)
	hits, err := a.SearchFiles("direct@web", dir, "needle", true)
	if err != nil || len(hits) != 1 {
		t.Fatalf("%+v %v", hits, err)
	}
}

func TestSearchRefusesNothingToLookFor(t *testing.T) {
	a := rigFiles(t)
	if _, err := a.SearchFiles("direct", "/tmp", "  ", true); err == nil {
		t.Error("an empty query should be refused")
	}
	if _, err := a.SearchFiles("direct", "", "x", true); err == nil {
		t.Error("a folder is needed")
	}
}

func TestSearchStopsAtTheCap(t *testing.T) {
	a := rigFiles(t)
	a.liveClientFor("direct").exec = shellExec
	dir := t.TempDir()
	for i := 0; i < searchCap+50; i++ {
		write(t, filepath.Join(dir, strings.Repeat("a", 3)+string(rune('a'+i%26))+strings.Repeat("b", i/26)+".log"), "x")
	}
	hits, err := a.SearchFiles("direct", dir, ".log", false)
	if err != nil || len(hits) != searchCap {
		t.Fatalf("%d hits, %v", len(hits), err)
	}
}
