package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/vault"
)

func init() { vault.Default = vault.Params{Time: 1, Memory: 8, Threads: 1} }

// lockRig is the usual rig with the passphrase lock enabled and a controllable clock.
func lockRig(t *testing.T, open func(context.Context, store.Server, string) (sshx.Session, error)) (*App, *fakeVPN, *events, *time.Time) {
	t.Helper()
	a, fv, ev := rig(t, open)
	dir := t.TempDir()
	a.vaultPath, a.settingsPath = filepath.Join(dir, "vault.enc"), filepath.Join(dir, "settings.json")
	clock := time.Now()
	a.clock = func() time.Time { return clock }
	if _, err := a.EnableLock("pass"); err != nil {
		t.Fatal(err)
	}
	return a, fv, ev, &clock
}

func TestLockedAppRefusesToConnectOrTouchShells(t *testing.T) {
	opened := 0
	sh := newShell()
	a, _, ev, _ := lockRig(t, func(context.Context, store.Server, string) (sshx.Session, error) { opened++; return sh, nil })
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	a.Lock()
	if !ev.has("app:locked") {
		t.Error("the UI is not told the app locked")
	}
	if got := a.ListServers(); len(got) != 0 {
		t.Errorf("locked app lists %d servers", len(got))
	}
	n := opened
	if r := a.Connect("a"); r.OK || !strings.Contains(r.Problem, "locked") || opened != n {
		t.Fatalf("connect while locked: %+v (opens %d->%d)", r, n, opened)
	}
	if err := a.AttachShell("direct", 1, 80, 24); !errors.Is(err, store.ErrLocked) {
		t.Errorf("attach: %v", err)
	}
	if _, err := a.Analyze("direct"); !errors.Is(err, store.ErrLocked) {
		t.Errorf("analyze: %v", err)
	}
	if err := a.OpenContainerShell("direct", 2, "app", "", false); !errors.Is(err, store.ErrLocked) {
		t.Errorf("container shell: %v", err)
	}
	a.TermWrite("direct", 1, "rm -rf /\r")
	if strings.Contains(sh.got.String(), "rm -rf") {
		t.Error("typing reached a shell while the app was locked")
	}
}

func TestLockKeepsSessionsAliveByDefaultAndUnlockResumesThem(t *testing.T) {
	sh := newShell()
	a, _, _, _ := lockRig(t, okOpen(sh))
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	a.Lock()
	sh.mu.Lock()
	closed := sh.closed
	sh.mu.Unlock()
	if closed {
		t.Fatal("locking must not end a running session")
	}
	if err := a.Unlock("wrong"); err == nil {
		t.Fatal("wrong passphrase unlocked")
	}
	if err := a.Unlock("pass"); err != nil {
		t.Fatal(err)
	}
	a.TermWrite("direct", 1, "ls\r")
	if !strings.Contains(sh.got.String(), "ls") {
		t.Error("the session did not resume after unlocking")
	}
}

func TestEndSessionsOnLockSetting(t *testing.T) {
	sh := newShell()
	a, fv, _, _ := lockRig(t, okOpen(sh))
	if err := a.SetLockSettings(15, true); err != nil {
		t.Fatal(err)
	}
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	a.Lock()
	sh.mu.Lock()
	closed := sh.closed
	sh.mu.Unlock()
	if !closed || len(fv.down) != 1 {
		t.Fatalf("shell closed=%v vpn down=%v", closed, fv.down)
	}
}

func TestIdleTimeoutLocksAndActivityResetsIt(t *testing.T) {
	a, _, _, clock := lockRig(t, okOpen(newShell()))
	if err := a.SetLockSettings(10, false); err != nil {
		t.Fatal(err)
	}
	a.Touch()
	*clock = clock.Add(9 * time.Minute)
	a.checkIdle()
	if a.LockState().Locked {
		t.Fatal("locked too early")
	}
	a.Touch()
	*clock = clock.Add(9 * time.Minute)
	a.checkIdle()
	if a.LockState().Locked {
		t.Fatal("activity did not reset the timer")
	}
	*clock = clock.Add(2 * time.Minute)
	a.checkIdle()
	if !a.LockState().Locked {
		t.Fatal("did not lock after 11 idle minutes")
	}
}

func TestIdleTimeoutZeroNeverLocks(t *testing.T) {
	a, _, _, clock := lockRig(t, okOpen(newShell()))
	if err := a.SetLockSettings(0, false); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(48 * time.Hour)
	a.checkIdle()
	if a.LockState().Locked {
		t.Fatal("timeout 0 means never")
	}
}

func TestLockSettingsPersistWithOwnerOnlyPermissions(t *testing.T) {
	a, _, _, _ := lockRig(t, okOpen(newShell()))
	if err := a.SetLockSettings(30, true); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(a.settingsPath)
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("settings file: %v %v", st, err)
	}
	got := loadSettings(a.settingsPath)
	if got.IdleMinutes != 30 || !got.EndOnLock {
		t.Fatalf("%+v", got)
	}
	if def := loadSettings(filepath.Join(t.TempDir(), "none.json")); def.IdleMinutes != 15 || def.EndOnLock {
		t.Fatalf("defaults %+v", def)
	}
	if err := a.SetLockSettings(-5, false); err == nil {
		t.Error("negative timeout accepted")
	}
}

func TestEnablingTheLockStartsUnlockedAndLockStateIsReported(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	if s := a.LockState(); s.Enabled || s.Locked {
		t.Fatalf("fresh app: %+v", s)
	}
	a.vaultPath = filepath.Join(t.TempDir(), "vault.enc")
	if _, err := a.EnableLock("pw"); err != nil {
		t.Fatal(err)
	}
	if s := a.LockState(); !s.Enabled || s.Locked || s.IdleMinutes != 15 {
		t.Fatalf("after enable: %+v", s)
	}
	if err := a.ChangePassphrase("pw", "pw2"); err != nil {
		t.Fatal(err)
	}
	a.Lock()
	if err := a.Unlock("pw"); err == nil {
		t.Error("old passphrase still works")
	}
	if err := a.Unlock("pw2"); err != nil {
		t.Error(err)
	}
}
