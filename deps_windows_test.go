package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/KushalNaral/sshdeck/internal/vpn"
)

// jobStarter stands in for the elevated helper: it records the request and plays back lines and an exit status.
type jobStarter struct {
	argv  []string
	lines []string
	err   error
}

type jobProc struct {
	lines chan string
	err   error
	once  sync.Once
}

func (p *jobProc) Lines() <-chan string { return p.lines }
func (p *jobProc) Wait() error          { return p.err }
func (p *jobProc) Stop() error          { p.once.Do(func() {}); return nil }

func (s *jobStarter) Start(_ context.Context, argv []string, _ string) (vpn.Proc, error) {
	s.argv = argv
	p := &jobProc{lines: make(chan string, len(s.lines)), err: s.err}
	for _, l := range s.lines {
		p.lines <- l
	}
	close(p.lines)
	return p, nil
}

func windowsRig(t *testing.T) (*App, *events, *jobStarter) {
	t.Helper()
	a, _, ev := rig(t, okOpen(newShell()))
	old := hostOS
	hostOS = "windows"
	t.Cleanup(func() { hostOS = old })
	js := &jobStarter{lines: []string{"Downloading the OpenConnect installer…", "OpenConnect is installed."}}
	a.starter = js
	return a, ev, js
}

func TestWindowsInstallOfOpenconnectRunsTheHelperJobAndStreamsItsLog(t *testing.T) {
	a, ev, js := windowsRig(t)
	if err := a.InstallDependencies([]string{"openconnect"}); err != nil {
		t.Fatal(err)
	}
	if len(js.argv) != 1 || js.argv[0] != vpn.InstallOpenconnectMarker {
		t.Errorf("request = %v, want only the install job", js.argv)
	}
	if got := ev.all["deps:log"]; len(got) != 2 || got[1] != "OpenConnect is installed." {
		t.Errorf("log = %v", got)
	}
}

func TestWindowsInstallRefusesAnythingElseAndReportsFailures(t *testing.T) {
	a, _, js := windowsRig(t)
	if err := a.InstallDependencies([]string{"openconnect", "calc"}); err == nil {
		t.Error("an unknown program was accepted")
	}
	if err := a.InstallDependencies([]string{"pkexec"}); err == nil {
		t.Error("a Linux program was accepted on Windows")
	}
	js.err = errors.New("the downloaded installer does not match the checksum")
	err := a.InstallDependencies([]string{"openconnect"})
	if err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("err = %v", err)
	}
}

func TestWindowsListsOpenconnectAsInstallable(t *testing.T) {
	a, _, _ := windowsRig(t)
	a.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	d := a.CheckDependencies()
	if len(d) != 1 || d[0].Name != "openconnect" || d[0].Installed || !d[0].Installable {
		t.Errorf("deps = %+v", d)
	}
}
