package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/KushalNaral/sshdeck/internal/audit"
	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
)

type trail struct {
	mu sync.Mutex
	e  []audit.Entry
}

func (tr *trail) add(event, subject, detail string) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.e = append(tr.e, audit.Entry{Event: event, Subject: subject, Detail: detail})
}

func (tr *trail) find(event string) *audit.Entry {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	for i := range tr.e {
		if tr.e[i].Event == event {
			return &tr.e[i]
		}
	}
	return nil
}

func (tr *trail) all() string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	var b strings.Builder
	for _, e := range tr.e {
		b.WriteString(e.Event + "|" + e.Subject + "|" + e.Detail + "\n")
	}
	return b.String()
}

func audited(t *testing.T, open func(context.Context, store.Server, string) (sshx.Session, error)) (*App, *trail) {
	t.Helper()
	a, _, _ := rig(t, open)
	tr := &trail{}
	a.audit = tr.add
	return a, tr
}

func TestConnectAndVPNAreRecorded(t *testing.T) {
	a, tr := audited(t, okOpen(newShell()))
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	if e := tr.find("vpn.up"); e == nil || e.Subject != "Forti" {
		t.Errorf("vpn.up: %+v", e)
	}
	if e := tr.find("connect"); e == nil || e.Subject != "A" || !strings.Contains(e.Detail, "Forti") {
		t.Errorf("connect: %+v", e)
	}
	a.Disconnect("a")
	if tr.find("disconnect") == nil || tr.find("vpn.down") == nil {
		t.Errorf("disconnect/vpn.down missing:\n%s", tr.all())
	}
}

func TestFailedConnectIsRecordedWithoutAnySecret(t *testing.T) {
	const secret = "S3cr3t-Pa55-xyz"
	a, tr := audited(t, func(_ context.Context, _ store.Server, pw string) (sshx.Session, error) {
		return nil, errors.New("login to h:22 as u failed: wrong user or password (tried " + "x" + ")")
	})
	if _, err := a.SaveServer(store.Server{ID: "sec", Name: "Sec", Host: "h", Port: 22, User: "u", Auth: "password"}, secret); err != nil {
		t.Fatal(err)
	}
	if r := a.Connect("sec"); r.OK {
		t.Fatal("should fail")
	}
	if e := tr.find("connect.fail"); e == nil || e.Subject != "Sec" || !strings.Contains(e.Detail, "wrong user or password") {
		t.Errorf("connect.fail: %+v", e)
	}
	if strings.Contains(tr.all(), secret) {
		t.Fatalf("a password leaked into the audit trail:\n%s", tr.all())
	}
}

func TestCancelledConnectAndContainerShellAreRecorded(t *testing.T) {
	started := make(chan struct{})
	a, tr := audited(t, func(ctx context.Context, _ store.Server, _ string) (sshx.Session, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("direct") }()
	waitFor(t, started)
	a.Disconnect("direct")
	<-done
	if tr.find("connect.cancel") == nil {
		t.Errorf("cancel not recorded:\n%s", tr.all())
	}

	a2, tr2 := audited(t, okOpen(newShell()))
	if r := a2.Connect("direct"); !r.OK {
		t.Fatal(r)
	}
	if err := a2.OpenContainerShell("direct", 2, "app", "root", true); err != nil {
		t.Fatal(err)
	}
	if e := tr2.find("container.exec"); e == nil || e.Subject != "Direct" || e.Detail != "container=app user=root sudo=true" {
		t.Errorf("container.exec: %+v", e)
	}
}

func TestConfigChangesAreRecordedByName(t *testing.T) {
	a, tr := audited(t, okOpen(newShell()))
	sv, err := a.SaveServer(store.Server{Name: "Fresh", Host: "h", Port: 22, User: "u", Auth: "password"}, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if e := tr.find("server.save"); e == nil || e.Subject != "Fresh" {
		t.Errorf("server.save: %+v", e)
	}
	if err := a.DeleteServer(sv.ID); err != nil {
		t.Fatal(err)
	}
	if e := tr.find("server.delete"); e == nil || e.Subject != "Fresh" {
		t.Errorf("server.delete: %+v", e)
	}
	if _, err := a.SaveVPN(store.VPN{Name: "NewVPN", Kind: store.Forti, Gateway: "g", Port: 1, User: "u"}, "p"); err != nil {
		t.Fatal(err)
	}
	tk, err := a.SaveTask(store.Task{Title: "Patch Atlas"})
	if err != nil {
		t.Fatal(err)
	}
	if err := a.DeleteTask(tk.ID); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"vpn.save", "task.save", "task.delete"} {
		if tr.find(ev) == nil {
			t.Errorf("%s missing:\n%s", ev, tr.all())
		}
	}
}

func TestLockEventsAreRecordedAndNeverCarryThePassphrase(t *testing.T) {
	a, _, _, _ := lockRig(t, okOpen(newShell()))
	tr := &trail{}
	a.audit = tr.add
	a.Lock()
	if err := a.Unlock("the wrong passphrase"); err == nil {
		t.Fatal("wrong passphrase unlocked")
	}
	if err := a.Unlock("pass"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetLockSettings(30, true); err != nil {
		t.Fatal(err)
	}
	if err := a.ChangePassphrase("pass", "another one"); err != nil {
		t.Fatal(err)
	}
	for _, ev := range []string{"lock", "unlock.fail", "unlock", "settings.change", "passphrase.change"} {
		if tr.find(ev) == nil {
			t.Errorf("%s missing:\n%s", ev, tr.all())
		}
	}
	for _, leak := range []string{"the wrong passphrase", "another one"} {
		if strings.Contains(tr.all(), leak) {
			t.Errorf("passphrase %q leaked:\n%s", leak, tr.all())
		}
	}
	if e := tr.find("lock"); e.Detail != "manual" {
		t.Errorf("lock reason %q", e.Detail)
	}
}

func TestIdleLockSaysWhy(t *testing.T) {
	a, _, _, clock := lockRig(t, okOpen(newShell()))
	tr := &trail{}
	a.audit = tr.add
	if err := a.SetLockSettings(5, false); err != nil {
		t.Fatal(err)
	}
	*clock = clock.Add(6 * 60 * 1e9)
	a.checkIdle()
	if e := tr.find("lock"); e == nil || e.Detail != "idle" {
		t.Errorf("%+v", e)
	}
}

func TestAuditViewNeedsAnUnlockedApp(t *testing.T) {
	a, _, _, _ := lockRig(t, okOpen(newShell()))
	l, err := audit.Open(filepath.Join(t.TempDir(), "audit.log"), "me")
	if err != nil {
		t.Fatal(err)
	}
	a.alog = l
	a.audit = func(e, s, d string) { _ = l.Add(e, s, d) }
	a.Connect("direct")
	got, err := a.ListAudit(10)
	if err != nil || len(got) == 0 {
		t.Fatalf("%v %v", got, err)
	}
	if c := a.VerifyAudit(); !c.OK {
		t.Fatalf("%+v", c)
	}
	a.Lock()
	if _, err := a.ListAudit(10); !errors.Is(err, store.ErrLocked) {
		t.Errorf("a locked app showed its audit log: %v", err)
	}
}

func TestProductionConfirmationIsRecorded(t *testing.T) {
	a, tr := audited(t, okOpen(newShell()))
	if err := a.RecordConfirm("direct", "connect"); err != nil {
		t.Fatal(err)
	}
	if e := tr.find("production.confirm"); e == nil || e.Subject != "Direct" || e.Detail != "connect" {
		t.Errorf("%+v", e)
	}
	if err := a.RecordConfirm("direct", "rm -rf /"); err == nil {
		t.Error("an arbitrary action was accepted")
	}
	if err := a.RecordConfirm("no-such-server", "connect"); err == nil {
		t.Error("unknown server accepted")
	}
	tr2 := &trail{}
	a.audit = tr2.add
	a.vaultPath = filepath.Join(t.TempDir(), "vault.enc")
	if _, err := a.EnableLock("pw"); err != nil {
		t.Fatal(err)
	}
	a.Lock()
	if err := a.RecordConfirm("direct", "connect"); !errors.Is(err, store.ErrLocked) {
		t.Errorf("locked: %v", err)
	}
}
