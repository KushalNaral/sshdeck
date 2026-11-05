package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KushalNaral/sshdeck/internal/audit"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/vault"
)

// LockInfo is what the UI needs to decide between the lock screen and the app.
type LockInfo struct {
	Enabled     bool `json:"enabled"`
	Locked      bool `json:"locked"`
	IdleMinutes int  `json:"idleMinutes"`
	EndOnLock   bool `json:"endOnLock"`
	// AutoReconnect: a dropped connection is redialled by itself.
	AutoReconnect bool `json:"autoReconnect"`
}

func (a *App) LockState() LockInfo {
	en, lk := a.st.State()
	a.mu.Lock()
	defer a.mu.Unlock()
	return LockInfo{Enabled: en, Locked: lk, IdleMinutes: a.settings.IdleMinutes, EndOnLock: a.settings.EndOnLock, AutoReconnect: !a.settings.NoReconnect}
}

func (a *App) isLocked() bool { _, lk := a.st.State(); return lk }

// guard is called first by every bound method that reaches a server.
func (a *App) guard() error {
	if a.isLocked() {
		return store.ErrLocked
	}
	return nil
}

// EnableLock puts everything under a passphrase (see store.EnableLock). The app stays unlocked afterwards.
// The returned names are keyring entries that could not be deleted.
func (a *App) EnableLock(pass string) ([]string, error) {
	left, err := a.st.EnableLock(a.vaultPath, pass)
	if err == nil {
		a.Touch()
		a.audit("lock.enable", "", "")
	}
	return left, err
}

// DisableLock goes back to the plain setup (passwords in the OS keyring, data in data.json) after checking the passphrase.
func (a *App) DisableLock(pass string) error {
	if err := a.guard(); err != nil {
		return err
	}
	if err := a.st.DisableLock(pass, a.plainPath, a.secrets); err != nil {
		return err
	}
	a.audit("lock.disable", "", "")
	return nil
}

func (a *App) Unlock(pass string) error {
	if err := a.st.Unlock(pass); err != nil {
		reason := err.Error()
		if errors.Is(err, vault.ErrWrongPassphrase) {
			reason = "wrong passphrase"
		}
		a.audit("unlock.fail", "", reason)
		return err
	}
	a.Touch()
	a.audit("unlock", "", "")
	return nil
}

// Lock hides everything and refuses further use until Unlock. Open sessions keep running unless the
// "end sessions on lock" setting is on.
func (a *App) Lock() { a.lockFor("manual") }

func (a *App) lockFor(reason string) {
	if en, lk := a.st.State(); !en || lk {
		return
	}
	a.mu.Lock()
	end := a.settings.EndOnLock
	a.mu.Unlock()
	if end {
		a.disconnectAll()
	}
	a.st.Lock()
	a.audit("lock", "", reason)
	a.emit("app:locked")
}

func (a *App) ChangePassphrase(oldPass, newPass string) error {
	if err := a.guard(); err != nil {
		return err
	}
	err := a.st.ChangePassphrase(oldPass, newPass)
	if err == nil {
		a.audit("passphrase.change", "", "")
	}
	return err
}

func (a *App) SetLockSettings(idleMinutes int, endOnLock bool) error {
	if idleMinutes < 0 || idleMinutes > 24*60 {
		return errors.New("the idle timeout must be between 0 (never) and 1440 minutes")
	}
	a.mu.Lock()
	a.settings.IdleMinutes, a.settings.EndOnLock = idleMinutes, endOnLock
	s := a.settings
	a.mu.Unlock()
	a.audit("settings.change", "", fmt.Sprintf("idle=%d endOnLock=%t", idleMinutes, endOnLock))
	return saveSettings(a.settingsPath, s)
}

// SetAutoReconnect turns the redial of dropped connections on or off.
func (a *App) SetAutoReconnect(on bool) error {
	a.mu.Lock()
	a.settings.NoReconnect = !on
	s := a.settings
	a.mu.Unlock()
	a.audit("settings.change", "", fmt.Sprintf("autoReconnect=%t", on))
	return saveSettings(a.settingsPath, s)
}

// Touch records user activity. The UI calls it on input; terminal typing counts too.
func (a *App) Touch() {
	a.mu.Lock()
	a.lastActive = a.clock()
	a.mu.Unlock()
}

// checkIdle locks the app when it has been idle for longer than the timeout.
func (a *App) checkIdle() {
	if en, lk := a.st.State(); !en || lk {
		return
	}
	a.mu.Lock()
	idle := time.Duration(a.settings.IdleMinutes) * time.Minute
	over := idle > 0 && a.clock().Sub(a.lastActive) >= idle
	a.mu.Unlock()
	if over {
		a.lockFor("idle")
	}
}

func (a *App) idleLoop(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			a.checkIdle()
		}
	}
}

// ListAudit returns the newest audit entries, for the Audit view. It needs an unlocked app.
func (a *App) ListAudit(limit int) ([]audit.Entry, error) {
	if err := a.guard(); err != nil {
		return nil, err
	}
	if a.alog == nil {
		return []audit.Entry{}, nil
	}
	return a.alog.Read(min(max(limit, 1), 1000))
}

// VerifyAudit checks that the audit log's hash chain is intact.
func (a *App) VerifyAudit() audit.Check {
	if a.alog == nil {
		return audit.Check{OK: true}
	}
	return audit.Verify(a.alog.Path())
}

// RecordConfirm notes in the audit trail that the user confirmed an action on a production server.
// It is the only audit entry the UI can write, and only these fixed actions are accepted.
func (a *App) RecordConfirm(serverID, action string) error {
	if err := a.guard(); err != nil {
		return err
	}
	if action != "connect" && action != "container" {
		return errors.New("unknown action to confirm")
	}
	sv, ok := a.server(serverID)
	if !ok {
		return errors.New("that server no longer exists")
	}
	a.audit("production.confirm", sv.Name, action)
	return nil
}
