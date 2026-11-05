package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestSSHWaitsUntilTheServerIsReachableAfterAVPNComesUp(t *testing.T) {
	a, _, ev, d := rigDial(t, okOpen(newShell()))
	var probes atomic.Int32
	a.routePoll = time.Millisecond
	a.probe = func(context.Context, string, int) error {
		if probes.Add(1) < 4 {
			return errors.New("i/o timeout") // the tunnel is up but its route is not in place yet
		}
		return nil
	}
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	if probes.Load() != 4 {
		t.Errorf("probes = %d, want it to keep trying until the route appears", probes.Load())
	}
	if d.count() != 1 {
		t.Errorf("dials = %d, want SSH tried once, after the route was ready", d.count())
	}
	var waited bool
	ev.mu.Lock()
	for _, x := range ev.all["connect:a"] {
		for _, s := range x.([]Step) {
			if s.Key == "ssh" && s.Detail != "" && len(s.Detail) > 20 && s.Detail[len(s.Detail)-5:] == "route" {
				waited = true
			}
		}
	}
	ev.mu.Unlock()
	if !waited {
		t.Error("the SSH step never said it was waiting for the VPN route")
	}
}

func TestNoRouteWaitForAServerWithoutAVPN(t *testing.T) {
	a, _, _, _ := rigDial(t, okOpen(newShell()))
	var probes atomic.Int32
	a.probe = func(context.Context, string, int) error { probes.Add(1); return errors.New("x") }
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	if probes.Load() != 0 {
		t.Errorf("probes = %d: a direct server needs no route check", probes.Load())
	}
}

func TestSSHIsStillTriedWhenTheRouteNeverAppears(t *testing.T) {
	a, _, _, d := rigDial(t, okOpen(newShell()))
	a.routeWait, a.routePoll = 30*time.Millisecond, time.Millisecond
	a.probe = func(context.Context, string, int) error { return errors.New("i/o timeout") }
	r := a.Connect("a")
	if d.count() != 1 {
		t.Errorf("dials = %d: after the wait SSH must still be tried, so its own error reaches the user", d.count())
	}
	if !r.OK {
		t.Errorf("%+v", r)
	}
}

func TestCancellingDuringTheRouteWaitStopsAtOnce(t *testing.T) {
	a, fv, _, d := rigDial(t, okOpen(newShell()))
	a.routeWait, a.routePoll = 10*time.Second, time.Millisecond
	started := make(chan struct{}, 1)
	a.probe = func(context.Context, string, int) error {
		select {
		case started <- struct{}{}:
		default:
		}
		return errors.New("i/o timeout")
	}
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("a") }()
	<-started
	a.Disconnect("a")
	select {
	case r := <-done:
		if r.OK || r.Problem != "Cancelled." {
			t.Errorf("%+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("cancel did not stop the wait")
	}
	if d.count() != 0 {
		t.Error("SSH was tried after the connect was cancelled")
	}
	fv.mu.Lock()
	defer fv.mu.Unlock()
	if len(fv.down) == 0 {
		t.Error("the VPN started for this connect was left running")
	}
}
