package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPermissionsAreTightened(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "sshdeck")
	os.MkdirAll(dir, 0o755)
	os.Chmod(dir, 0o755)
	for _, n := range []string{"data.json", "vault.enc", "known_hosts", "audit.log", "settings.json"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "unrelated.txt"), []byte("x"), 0o644)
	if err := checkConfigDir(dir); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(dir); st.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", st.Mode().Perm())
	}
	for _, n := range []string{"data.json", "vault.enc", "known_hosts", "audit.log", "settings.json"} {
		if st, _ := os.Stat(filepath.Join(dir, n)); st.Mode().Perm() != 0o600 {
			t.Errorf("%s mode %v", n, st.Mode().Perm())
		}
	}
	if st, _ := os.Stat(filepath.Join(dir, "unrelated.txt")); st.Mode().Perm() != 0o644 {
		t.Error("a file that is not ours was touched")
	}
}

func TestMissingConfigDirIsCreatedPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "sshdeck")
	if err := checkConfigDir(dir); err != nil {
		t.Fatal(err)
	}
	if st, err := os.Stat(dir); err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("%v %v", st, err)
	}
}

func TestASymlinkInPlaceOfAConfigFileIsRefused(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	os.WriteFile(target, []byte("x"), 0o600)
	if err := os.Symlink(target, filepath.Join(dir, "data.json")); err != nil {
		t.Skip("no symlinks here")
	}
	if err := checkConfigDir(dir); err == nil {
		t.Fatal("a symlinked data.json was accepted")
	}
}
