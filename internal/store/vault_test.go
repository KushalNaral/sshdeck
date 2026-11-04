package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/vault"
)

func init() { vault.Default = vault.Params{Time: 1, Memory: 8, Threads: 1} }

// plainWithData is a classic store (keyring-backed) holding one server, one VPN and one task.
func plainWithData(t *testing.T) (*Store, memSecrets, string, string) {
	t.Helper()
	s, sec, path := open(t)
	v, err := s.SaveVPN(VPN{Name: "Gate", Kind: Forti, Gateway: "gw.example.org", Port: 443, User: "vu"}, "vpn-pw")
	if err != nil {
		t.Fatal(err)
	}
	sv := good
	sv.VPNID = v.ID
	sv, err = s.SaveServer(sv, "server-pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveTask(Task{Title: "Patch it", ServerIDs: []string{sv.ID}}); err != nil {
		t.Fatal(err)
	}
	return s, sec, path, filepath.Join(filepath.Dir(path), "vault.enc")
}

func TestEnableLockMovesDataAndSecretsIntoTheVault(t *testing.T) {
	s, sec, plainPath, vpath := plainWithData(t)
	left, err := s.EnableLock(vpath, "pass phrase")
	if err != nil || len(left) != 0 {
		t.Fatalf("enable: %v leftover %v", err, left)
	}
	if _, err := os.Stat(plainPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("plaintext data.json still exists")
	}
	if len(sec) != 0 {
		t.Errorf("keyring still holds %d secrets", len(sec))
	}
	raw, _ := os.ReadFile(vpath)
	for _, leak := range []string{"Atlas", "10.0.0.5", "server-pw", "vpn-pw", "gw.example.org"} {
		if bytes.Contains(raw, []byte(leak)) {
			t.Errorf("vault file contains %q in the clear", leak)
		}
	}
	if st, _ := os.Stat(vpath); st.Mode().Perm() != 0o600 {
		t.Errorf("vault mode %v", st.Mode().Perm())
	}
	sv := s.Servers()[0]
	if pw, err := s.Password("server", sv.ID); err != nil || pw != "server-pw" {
		t.Fatalf("password after enabling: %q %v", pw, err)
	}

	// A fresh start finds it locked, and the right passphrase brings everything back.
	s2, err := OpenLocked(vpath)
	if err != nil {
		t.Fatal(err)
	}
	if enabled, locked := s2.State(); !enabled || !locked {
		t.Fatalf("state %v %v", enabled, locked)
	}
	if len(s2.Servers()) != 0 || len(s2.Tasks()) != 0 {
		t.Error("a locked store must show nothing")
	}
	if err := s2.Unlock("pass phrase"); err != nil {
		t.Fatal(err)
	}
	if len(s2.Servers()) != 1 || len(s2.VPNs()) != 1 || len(s2.Tasks()) != 1 {
		t.Fatalf("data lost: %d %d %d", len(s2.Servers()), len(s2.VPNs()), len(s2.Tasks()))
	}
	if pw, err := s2.Password("vpn", s2.VPNs()[0].ID); err != nil || pw != "vpn-pw" {
		t.Fatalf("vpn password: %q %v", pw, err)
	}
}

func TestLockedStoreRefusesEverything(t *testing.T) {
	s, _, _, vpath := plainWithData(t)
	if _, err := s.EnableLock(vpath, "p"); err != nil {
		t.Fatal(err)
	}
	id := s.Servers()[0].ID
	s.Lock()
	if _, err := s.Password("server", id); !errors.Is(err, ErrLocked) {
		t.Errorf("password: %v", err)
	}
	if _, err := s.SaveServer(good, "x"); !errors.Is(err, ErrLocked) {
		t.Errorf("save server: %v", err)
	}
	if err := s.DeleteServer(id); !errors.Is(err, ErrLocked) {
		t.Errorf("delete server: %v", err)
	}
	if _, err := s.SaveVPN(VPN{Name: "V", Kind: Forti, Gateway: "g", Port: 1, User: "u"}, ""); !errors.Is(err, ErrLocked) {
		t.Errorf("save vpn: %v", err)
	}
	if _, err := s.SaveTask(Task{Title: "t"}); !errors.Is(err, ErrLocked) {
		t.Errorf("save task: %v", err)
	}
	if len(s.Servers()) != 0 {
		t.Error("locked store still lists servers")
	}
}

func TestChangesInVaultModeStayEncrypted(t *testing.T) {
	s, sec, _, vpath := plainWithData(t)
	if _, err := s.EnableLock(vpath, "p"); err != nil {
		t.Fatal(err)
	}
	sv := good
	sv.Name, sv.Host = "Another", "10.77.66.55"
	saved, err := s.SaveServer(sv, "brand-new-pw")
	if err != nil {
		t.Fatal(err)
	}
	if len(sec) != 0 {
		t.Error("a new password leaked into the keyring")
	}
	raw, _ := os.ReadFile(vpath)
	if bytes.Contains(raw, []byte("10.77.66.55")) || bytes.Contains(raw, []byte("brand-new-pw")) {
		t.Error("new data written in the clear")
	}
	s2, _ := OpenLocked(vpath)
	if err := s2.Unlock("p"); err != nil {
		t.Fatal(err)
	}
	if pw, _ := s2.Password("server", saved.ID); pw != "brand-new-pw" {
		t.Errorf("password not persisted: %q", pw)
	}
	if err := s2.DeleteServer(saved.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Password("server", saved.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted server's password survived: %v", err)
	}
}

func TestWrongPassphraseIsSlowedDown(t *testing.T) {
	s, _, _, vpath := plainWithData(t)
	if _, err := s.EnableLock(vpath, "right"); err != nil {
		t.Fatal(err)
	}
	clock := time.Now()
	old := now
	now = func() time.Time { return clock }
	defer func() { now = old }()

	s2, _ := OpenLocked(vpath)
	for i := 0; i < 3; i++ {
		if err := s2.Unlock("wrong"); !errors.Is(err, vault.ErrWrongPassphrase) {
			t.Fatalf("attempt %d: %v", i, err)
		}
	}
	var soon *TooSoonError
	if err := s2.Unlock("right"); !errors.As(err, &soon) || soon.Wait <= 0 {
		t.Fatalf("even the right passphrase must wait after repeated failures: %v", err)
	}
	clock = clock.Add(time.Minute)
	if err := s2.Unlock("right"); err != nil {
		t.Fatalf("after the wait: %v", err)
	}
	s2.Lock()
	if err := s2.Unlock("wrong"); !errors.Is(err, vault.ErrWrongPassphrase) {
		t.Fatalf("counter should have reset: %v", err)
	}
}

func TestChangePassphrase(t *testing.T) {
	s, _, _, vpath := plainWithData(t)
	if _, err := s.EnableLock(vpath, "old"); err != nil {
		t.Fatal(err)
	}
	if err := s.ChangePassphrase("not-old", "new"); !errors.Is(err, vault.ErrWrongPassphrase) {
		t.Fatalf("wrong old passphrase: %v", err)
	}
	if err := s.ChangePassphrase("old", "new"); err != nil {
		t.Fatal(err)
	}
	s2, _ := OpenLocked(vpath)
	if err := s2.Unlock("old"); !errors.Is(err, vault.ErrWrongPassphrase) {
		t.Errorf("old passphrase still works: %v", err)
	}
	if err := s2.Unlock("new"); err != nil {
		t.Errorf("new passphrase: %v", err)
	}
}

func TestEnableLockTwiceAndEmptyPassphraseAreRefused(t *testing.T) {
	s, _, _, vpath := plainWithData(t)
	if _, err := s.EnableLock(vpath, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty passphrase: %v", err)
	}
	if _, err := s.EnableLock(vpath, "p"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnableLock(vpath, "p"); !errors.Is(err, ErrInvalid) {
		t.Errorf("second enable: %v", err)
	}
}

type failingSecrets struct{ memSecrets }

func (f failingSecrets) Set(string, string) error { return errors.New("keyring is locked") }

func TestDisableLockReturnsEverythingToTheKeyringSetup(t *testing.T) {
	s, _, plainPath, vpath := plainWithData(t)
	if _, err := s.EnableLock(vpath, "p"); err != nil {
		t.Fatal(err)
	}
	ring := memSecrets{}
	if err := s.DisableLock("p", plainPath, ring); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(vpath); !errors.Is(err, os.ErrNotExist) {
		t.Error("vault file still exists")
	}
	raw, err := os.ReadFile(plainPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("server-pw")) || bytes.Contains(raw, []byte("secrets")) {
		t.Error("passwords were written into data.json")
	}
	if st, _ := os.Stat(plainPath); st.Mode().Perm() != 0o600 {
		t.Errorf("data.json mode %v", st.Mode().Perm())
	}
	if enabled, _ := s.State(); enabled {
		t.Error("still reports the lock as enabled")
	}
	sv := s.Servers()[0]
	if pw, err := s.Password("server", sv.ID); err != nil || pw != "server-pw" {
		t.Fatalf("password after disabling: %q %v", pw, err)
	}
	// A fresh start sees an ordinary store.
	s2, err := Open(plainPath, ring)
	if err != nil || len(s2.Servers()) != 1 || len(s2.Tasks()) != 1 {
		t.Fatalf("reopen: %v", err)
	}
	if pw, _ := s2.Password("vpn", s2.VPNs()[0].ID); pw != "vpn-pw" {
		t.Errorf("vpn password %q", pw)
	}
}

func TestDisableLockNeedsTheRightPassphraseAndKeepsTheVaultOnFailure(t *testing.T) {
	s, _, plainPath, vpath := plainWithData(t)
	if _, err := s.EnableLock(vpath, "p"); err != nil {
		t.Fatal(err)
	}
	if err := s.DisableLock("wrong", plainPath, memSecrets{}); !errors.Is(err, vault.ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	if err := s.DisableLock("p", plainPath, failingSecrets{memSecrets{}}); err == nil {
		t.Fatal("a failing keyring went unnoticed")
	}
	if _, err := os.Stat(vpath); err != nil {
		t.Error("the vault was removed although disabling failed")
	}
	if _, err := os.Stat(plainPath); !errors.Is(err, os.ErrNotExist) {
		t.Error("data.json was written although disabling failed")
	}
	if enabled, _ := s.State(); !enabled {
		t.Error("lock state changed after a failed disable")
	}
	s.Lock()
	if err := s.DisableLock("p", plainPath, memSecrets{}); !errors.Is(err, ErrLocked) {
		t.Errorf("locked store: %v", err)
	}
}
