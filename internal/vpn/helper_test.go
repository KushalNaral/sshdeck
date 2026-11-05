package vpn

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// helperRig wires a HelperStarter to an in-process helper that runs fake clients instead of real ones.
type helperRig struct {
	hs       *HelperStarter
	under    *fakeStarter
	launches atomic.Int32
	allow    func([]string) error
}

func newHelperRig(t *testing.T, script func(*fakeProc)) *helperRig {
	t.Helper()
	r := &helperRig{under: &fakeStarter{script: script}, allow: func([]string) error { return nil }}
	r.hs = &HelperStarter{Launch: func(addr, token string) error {
		r.launches.Add(1)
		go func() { _ = RunHelperWith(addr, token, r.under, func(a []string) error { return r.allow(a) }) }()
		return nil
	}}
	t.Cleanup(func() { _ = r.hs.Close() })
	return r
}

func TestHelperRunsAClientAndStreamsItsOutput(t *testing.T) {
	r := newHelperRig(t, printThenStay("Connected as 10.0.0.2, using SSL"))
	p, err := r.hs.Start(ctx(t), []string{"openconnect", "--x"}, "secret\n")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case l := <-p.Lines():
		if l != "Connected as 10.0.0.2, using SSL" {
			t.Errorf("line = %q", l)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no output from the helper")
	}
	if r.under.stdin != "secret\n" {
		t.Errorf("stdin reached the client as %q", r.under.stdin)
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if !r.under.proc.wasStopped() {
		t.Error("Stop did not reach the client")
	}
}

func TestHelperIsStartedOnceAndReused(t *testing.T) {
	r := newHelperRig(t, printThenStay("up"))
	for i := 0; i < 3; i++ {
		p, err := r.hs.Start(ctx(t), []string{"openconnect"}, "")
		if err != nil {
			t.Fatal(err)
		}
		<-p.Lines()
		_ = p.Stop()
	}
	if n := r.launches.Load(); n != 1 {
		t.Errorf("helper launched %d times, want 1: one UAC prompt per session", n)
	}
}

func TestHelperReportsHowTheClientEnded(t *testing.T) {
	r := newHelperRig(t, printThenExit(errors.New("exit status 1"), "ERROR: Failed to complete authentication"))
	p, err := r.hs.Start(ctx(t), []string{"openconnect"}, "")
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for l := range p.Lines() {
		lines = append(lines, l)
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "Failed to complete") {
		t.Errorf("lines = %v", lines)
	}
	if err := p.Wait(); err == nil || !strings.Contains(err.Error(), "exit status 1") {
		t.Errorf("Wait = %v, want the client's exit error", err)
	}
}

func TestHelperRefusesWhatTheAllowListRefuses(t *testing.T) {
	r := newHelperRig(t, printThenStay("should never run"))
	r.allow = func(argv []string) error {
		if argv[0] != "openconnect" {
			return errors.New("not allowed")
		}
		return nil
	}
	p, err := r.hs.Start(ctx(t), []string{"cmd.exe", "/c", "calc"}, "")
	if err != nil {
		t.Fatal(err)
	}
	for range p.Lines() {
	}
	if err := p.Wait(); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Errorf("Wait = %v", err)
	}
	if r.under.n != 0 {
		t.Error("a refused command was started anyway")
	}
}

func TestHelperRejectsAWrongToken(t *testing.T) {
	var ran atomic.Bool
	hs := &HelperStarter{Launch: func(addr, token string) error {
		go func() {
			_ = RunHelperWith(addr, "not-the-token", &fakeStarter{script: func(*fakeProc) { ran.Store(true) }}, func([]string) error { return nil })
		}()
		return nil
	}}
	defer func() { _ = hs.Close() }()
	c, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := hs.Start(c, []string{"openconnect"}, ""); err == nil {
		t.Fatal("a helper that does not know the token must not be accepted")
	}
	if ran.Load() {
		t.Error("something ran")
	}
}

func TestLostHelperEndsItsClientsWithAnError(t *testing.T) {
	r := newHelperRig(t, printThenStay("up"))
	p, err := r.hs.Start(ctx(t), []string{"openconnect"}, "")
	if err != nil {
		t.Fatal(err)
	}
	<-p.Lines()
	r.hs.dropForTest() // the helper process died
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("want an error when the helper is lost")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Wait never returned after the helper was lost")
	}
}

func TestRefusedElevationIsAPlainError(t *testing.T) {
	hs := &HelperStarter{Launch: func(string, string) error { return errors.New("The operation was canceled by the user.") }}
	_, err := hs.Start(ctx(t), []string{"openconnect"}, "")
	if err == nil || !strings.Contains(err.Error(), "administrator") {
		t.Errorf("err = %v, want a message about administrator approval", err)
	}
}

func TestCancellingWhileWaitingForTheHelper(t *testing.T) {
	var mu sync.Mutex
	hs := &HelperStarter{Launch: func(string, string) error { mu.Lock(); mu.Unlock(); return nil }} // the helper never connects
	c, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := hs.Start(c, []string{"openconnect"}, "")
	if err == nil || time.Since(start) > time.Second {
		t.Errorf("err = %v after %v: cancelling must stop the wait for the UAC prompt", err, time.Since(start))
	}
}

func TestOpenconnectArgumentsAreChecked(t *testing.T) {
	ok := [][]string{
		{"--protocol=gp", "--user", "me", "--passwd-on-stdin", "vpn.example.org"},
		{"--protocol=fortinet", "--user", "me@corp", "--passwd-on-stdin", "--servercert", "pin-sha256:" + strings.Repeat("A", 43) + "=", "vpn.example.org:10443"},
		{"--protocol=gp", "--user", "me", "--passwd-on-stdin", "--servercert", "sha256:" + strings.Repeat("ab", 32), "203.0.113.7"},
	}
	for _, a := range ok {
		if err := validateOpenconnectArgs(a); err != nil {
			t.Errorf("%v refused: %v", a, err)
		}
	}
	bad := [][]string{
		nil,
		{"--protocol=gp", "--passwd-on-stdin", "vpn.example.org"},                                         // no user
		{"--protocol=gp", "--user", "me", "vpn.example.org"},                                              // password would be asked on a console
		{"--protocol=gp", "--user", "me", "--passwd-on-stdin", "--script", "evil.bat", "vpn.example.org"}, // runs a script
		{"--protocol=gp", "--user", "me", "--passwd-on-stdin", "--cafile", "x", "vpn.example.org"},
		{"--protocol=anyconnect", "--user", "me", "--passwd-on-stdin", "vpn.example.org"},
		{"--protocol=gp", "--user", "me", "--passwd-on-stdin", "vpn.example.org; calc"},
		{"--protocol=gp", "--user", "-x", "--passwd-on-stdin", "vpn.example.org"},
		{"--protocol=gp", "--user", "me", "--passwd-on-stdin", "--servercert", "x y", "vpn.example.org"},
		{"--protocol=gp", "--user", "me", "--passwd-on-stdin", "vpn.example.org", "extra"},
	}
	for _, a := range bad {
		if err := validateOpenconnectArgs(a); err == nil {
			t.Errorf("%v accepted", a)
		}
	}
}

func TestUnderDirIsCaseAndSlashInsensitive(t *testing.T) {
	for _, c := range []struct {
		path, dir string
		want      bool
	}{
		{`C:\Program Files\OpenConnect\openconnect.exe`, `C:\Program Files`, true},
		{`c:/program files/openconnect/openconnect.exe`, `C:\Program Files`, true},
		{`C:\Program Files Evil\openconnect.exe`, `C:\Program Files`, false},
		{`C:\Users\me\openconnect.exe`, `C:\Program Files`, false},
		{`C:\Program Files`, `C:\Program Files`, false},
	} {
		if got := underDir(c.path, c.dir); got != c.want {
			t.Errorf("underDir(%q, %q) = %v", c.path, c.dir, got)
		}
	}
}

var _ = store.GP

func TestWindowsVPNGoesThroughTheHelperAndItsAllowList(t *testing.T) {
	asWindows(t)
	r := newHelperRig(t, printThenStay("Connected as 10.0.0.2, using SSL"))
	r.allow = allowOpenconnect // the real check, as the elevated helper applies it
	m := New(r.hs)
	if err := m.Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	if m.Status("gov") != Up {
		t.Fatalf("status %v", m.Status("gov"))
	}
	if r.under.stdin != "pw\n" {
		t.Errorf("the password did not reach the client on stdin: %q", r.under.stdin)
	}
	if err := m.Down("gov"); err != nil {
		t.Fatal(err)
	}
	if !r.under.proc.wasStopped() || m.Status("gov") != Down {
		t.Error("Down must stop the client through the helper")
	}
}
