package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/KushalNaral/sshdeck/internal/update"
)

// version is set at build time (-ldflags "-X main.version=v0.4.0"). "dev" never looks for updates.
var version = "dev"

const repo = "KushalNaral/sshdeck"

// UpdateStatus is what the UI shows about updating.
type UpdateStatus struct {
	Version   string `json:"version"`
	AutoCheck bool   `json:"autoCheck"`
}

func (a *App) UpdateStatus() UpdateStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return UpdateStatus{Version: a.version, AutoCheck: !a.settings.NoUpdateCheck}
}

// SetAutoUpdateCheck turns the look for a newer release at startup on or off. Checking by hand always works.
func (a *App) SetAutoUpdateCheck(on bool) error {
	a.mu.Lock()
	a.settings.NoUpdateCheck = !on
	s := a.settings
	a.mu.Unlock()
	a.audit("settings.change", "", fmt.Sprintf("autoUpdateCheck=%t", on))
	return saveSettings(a.settingsPath, s)
}

func (a *App) updater() *update.Updater {
	return &update.Updater{Repo: repo, Current: a.version, PublicKey: update.PublicKey()}
}

// exePath is the running program, with symbolic links resolved so the update replaces the real file.
func exePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// CheckUpdate asks GitHub whether a newer release exists.
func (a *App) CheckUpdate() (update.Info, error) {
	if a.version == "dev" {
		return update.Info{}, errors.New("this is a development build, so there is no version to compare")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	return a.updCheck(ctx)
}

// checkUpdateOnStart looks for a newer release a little after startup and tells the UI. It stays silent when the
// check is off, this is a development build, nothing newer exists, or GitHub cannot be reached.
func (a *App) checkUpdateOnStart(delay time.Duration) {
	a.mu.Lock()
	off := a.settings.NoUpdateCheck
	a.mu.Unlock()
	if off || a.version == "dev" {
		return
	}
	go func() {
		time.Sleep(delay)
		ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
		defer cancel()
		if info, err := a.updCheck(ctx); err == nil && info.Available {
			a.emit("update:available", info)
		}
	}()
}

// InstallUpdate downloads, verifies and installs the newest release in place of the running program. It does not
// restart: the UI offers that when the "update:installed" event arrives.
func (a *App) InstallUpdate() error {
	if a.version == "dev" {
		return errors.New("this is a development build")
	}
	ctx, cancel := context.WithTimeout(a.ctx, 10*time.Minute)
	defer cancel()
	info, err := a.updCheck(ctx)
	if err != nil {
		return err
	}
	if !info.Available {
		return errors.New("there is no newer release to install")
	}
	a.emit("update:progress", "Downloading "+info.Latest+" and checking its signature…")
	if err := a.updInstall(ctx, info); err != nil {
		return err
	}
	a.audit("update.install", info.Latest, "from "+a.version)
	a.emit("update:installed", info)
	return nil
}

// realInstall is the production install step: fetch and verify into the program's folder, then swap it in.
func (a *App) realInstall(ctx context.Context, info update.Info) error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	u := a.updater()
	p, err := u.Fetch(ctx, info, filepath.Dir(exe))
	if err != nil {
		return err
	}
	if err := update.Apply(p, exe, runtime.GOOS); err != nil {
		_ = os.Remove(p)
		return fmt.Errorf("could not replace %s: %w", exe, err)
	}
	return nil
}

// RestartApp starts the installed program again shortly after this one has quit, then quits.
func (a *App) RestartApp() error {
	exe, err := exePath()
	if err != nil {
		return err
	}
	if err := relaunch(exe); err != nil {
		return err
	}
	a.audit("app.restart", "", "")
	wr.Quit(a.ctx)
	return nil
}
