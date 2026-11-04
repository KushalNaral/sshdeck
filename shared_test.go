package main

import (
	"context"
	"errors"
	"testing"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
)

func TestOneLoginServesConnectTabsAndAnalysis(t *testing.T) {
	a, _, _, d := rigDial(t, func(context.Context, store.Server, string) (sshx.Session, error) { return newShell(), nil })
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	for tab := 2; tab <= 3; tab++ {
		if err := a.AttachShell("direct", tab, 80, 24); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Analyze("direct"); err != nil {
		t.Fatal(err)
	}
	if n := d.count(); n != 1 {
		t.Fatalf("%d logins for one connected server, want 1", n)
	}
	c := d.last()
	if c.shells != 3 || len(c.runs) < 2 {
		t.Fatalf("shells=%d runs=%v", c.shells, c.runs)
	}
}

func TestDisconnectClosesTheSharedConnection(t *testing.T) {
	a, _, _, d := rigDial(t, okOpen(newShell()))
	a.Connect("direct")
	a.Disconnect("direct")
	if !d.last().closed {
		t.Fatal("the connection outlived Disconnect")
	}
}

func TestADroppedConnectionIsRedialedForTheNextTab(t *testing.T) {
	a, _, _, d := rigDial(t, func(context.Context, store.Server, string) (sshx.Session, error) { return newShell(), nil })
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	d.last().drop()
	if err := a.AttachShell("direct", 2, 80, 24); err != nil {
		t.Fatalf("a new tab after the link dropped: %v", err)
	}
	if d.count() != 2 {
		t.Fatalf("%d logins, want a fresh one after the drop", d.count())
	}
	// And the new connection is the one used from now on.
	if err := a.AttachShell("direct", 3, 80, 24); err != nil || d.count() != 2 {
		t.Fatalf("third tab: %v, logins %d", err, d.count())
	}
}

func TestAnalysingAnUnconnectedServerLogsInOnceNotPerCommand(t *testing.T) {
	a, _, _, d := rigDial(t, okOpen(newShell()))
	if _, err := a.Analyze("direct"); err != nil {
		t.Fatal(err)
	}
	if d.count() != 1 {
		t.Fatalf("%d logins for one analysis, want 1", d.count())
	}
	c := d.last()
	if len(c.runs) < 5 || !c.closed {
		t.Fatalf("runs=%d closed=%v: the one-off connection must carry every command and then close", len(c.runs), c.closed)
	}
}

func TestNothingLeaksWhenTheShellCannotStart(t *testing.T) {
	a, _, _, d := rigDial(t, func(context.Context, store.Server, string) (sshx.Session, error) {
		return nil, errors.New("login to h:22 as u failed: wrong user or password")
	})
	if r := a.Connect("direct"); r.OK {
		t.Fatal("should fail")
	}
	if !d.last().closed {
		t.Fatal("the connection stayed open after a failed connect")
	}
}

func TestACancelledConnectClosesItsConnection(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	a, _, _, d := rigDial(t, func(context.Context, store.Server, string) (sshx.Session, error) {
		close(started)
		<-release
		return newShell(), nil
	})
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("direct") }()
	waitFor(t, started)
	a.Disconnect("direct")
	close(release)
	if r := <-done; r.OK {
		t.Fatalf("%+v", r)
	}
	if !d.last().closed {
		t.Fatal("a cancelled connect left its connection open")
	}
}
