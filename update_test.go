package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/update"
)

func updRig(t *testing.T, version string) (*App, *events, *trail) {
	t.Helper()
	a, _, ev := rig(t, okOpen(newShell()))
	tr := &trail{}
	a.audit = tr.add
	a.version = version
	return a, ev, tr
}

func TestStartupCheckAnnouncesANewerRelease(t *testing.T) {
	a, ev, _ := updRig(t, "v0.4.0")
	a.updCheck = func(context.Context) (update.Info, error) {
		return update.Info{Current: "v0.4.0", Latest: "v0.5.0", Available: true, CanInstall: true}, nil
	}
	a.checkUpdateOnStart(0)
	until(t, "update event", func() bool { return ev.get("update:available") != nil })
	if got := ev.get("update:available").(update.Info); got.Latest != "v0.5.0" {
		t.Errorf("event = %+v", got)
	}
}

func TestStartupCheckStaysQuietWhenCurrentOffOrDev(t *testing.T) {
	for name, set := range map[string]func(*App){
		"up to date": func(a *App) {
			a.updCheck = func(context.Context) (update.Info, error) { return update.Info{Latest: "v0.4.0"}, nil }
		},
		"switched off": func(a *App) {
			a.settings.NoUpdateCheck = true
			a.updCheck = func(context.Context) (update.Info, error) { return update.Info{Available: true}, nil }
		},
		"development build": func(a *App) {
			a.version = "dev"
			a.updCheck = func(context.Context) (update.Info, error) { return update.Info{Available: true}, nil }
		},
		"network error": func(a *App) {
			a.updCheck = func(context.Context) (update.Info, error) { return update.Info{}, errors.New("offline") }
		},
	} {
		a, ev, _ := updRig(t, "v0.4.0")
		set(a)
		a.checkUpdateOnStart(0)
		time.Sleep(60 * time.Millisecond)
		if ev.get("update:available") != nil {
			t.Errorf("%s: announced an update", name)
		}
	}
}

func TestManualCheckWorksEvenWhenTheStartupCheckIsOff(t *testing.T) {
	a, _, _ := updRig(t, "v0.4.0")
	a.settings.NoUpdateCheck = true
	a.updCheck = func(context.Context) (update.Info, error) {
		return update.Info{Latest: "v0.5.0", Available: true}, nil
	}
	info, err := a.CheckUpdate()
	if err != nil || !info.Available {
		t.Fatalf("%+v %v", info, err)
	}
	a.version = "dev"
	if _, err := a.CheckUpdate(); err == nil {
		t.Error("a development build has no version to compare")
	}
}

func TestUpdateSettingIsRememberedAndAudited(t *testing.T) {
	a, _, tr := updRig(t, "v0.4.0")
	if !a.UpdateStatus().AutoCheck {
		t.Fatal("the startup check should default to on")
	}
	if err := a.SetAutoUpdateCheck(false); err != nil {
		t.Fatal(err)
	}
	if a.UpdateStatus().AutoCheck || tr.find("settings.change") == nil {
		t.Error("the setting was not saved and recorded")
	}
}

func TestInstallRefusesWhenThereIsNothingNewer(t *testing.T) {
	a, _, _ := updRig(t, "v0.4.0")
	a.updCheck = func(context.Context) (update.Info, error) { return update.Info{Latest: "v0.4.0"}, nil }
	if err := a.InstallUpdate(); err == nil {
		t.Error("installing with no newer release should fail")
	}
}

func TestInstallFetchesAppliesAndRecordsIt(t *testing.T) {
	a, ev, tr := updRig(t, "v0.4.0")
	a.updCheck = func(context.Context) (update.Info, error) {
		return update.Info{Latest: "v0.5.0", Available: true, CanInstall: true}, nil
	}
	var applied string
	a.updInstall = func(_ context.Context, info update.Info) error { applied = info.Latest; return nil }
	if err := a.InstallUpdate(); err != nil {
		t.Fatal(err)
	}
	if applied != "v0.5.0" || tr.find("update.install") == nil {
		t.Errorf("applied %q, audit %v", applied, tr.find("update.install"))
	}
	if !ev.has("update:installed") {
		t.Error("the UI was not told to offer a restart")
	}
}
