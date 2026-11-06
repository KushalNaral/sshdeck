package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
)

// throughRecorder stands in for sshx.DialThrough and remembers which client each login went through.
type throughRecorder struct {
	mu   sync.Mutex
	vias map[string]sshx.Client // target host -> the client it was reached through
}

func (r *throughRecorder) dial(_ context.Context, via sshx.Client, sv store.Server, pw string) (sshx.Client, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.vias == nil {
		r.vias = map[string]sshx.Client{}
	}
	r.vias[sv.Host] = via
	return &fakeClient{open: okOpen(newShell()), sv: sv, pw: pw, done: make(chan struct{})}, nil
}

func rigJump(t *testing.T) (*App, *fakeVPN, *throughRecorder, store.Server) {
	t.Helper()
	a, fv, _ := rig(t, okOpen(newShell()))
	rec := &throughRecorder{}
	a.dialThrough = rec.dial
	inner, err := a.st.SaveServer(store.Server{Name: "Inner", Host: "10.1.1.1", Port: 22, User: "u", Auth: "password", JumpID: "a"}, "pw")
	if err != nil {
		t.Fatal(err)
	}
	return a, fv, rec, inner
}

func TestAJumpedServerIsReachedThroughItsJumpHost(t *testing.T) {
	a, _, rec, inner := rigJump(t)
	if r := a.Connect(inner.ID); !r.OK {
		t.Fatalf("%+v", r)
	}
	if rec.vias["10.1.1.1"] == nil {
		t.Fatal("the login should have gone through a jump host")
	}
	if a.conns["a"] != nil {
		t.Error("the jump host is only a hop: it is not a connection of its own")
	}
}

func TestClosingAJumpedServerClosesItsHop(t *testing.T) {
	a, _, rec, inner := rigJump(t)
	if r := a.Connect(inner.ID); !r.OK {
		t.Fatalf("%+v", r)
	}
	hop := rec.vias["10.1.1.1"].(*fakeClient)
	a.Disconnect(inner.ID)
	hop.mu.Lock()
	defer hop.mu.Unlock()
	if !hop.closed {
		t.Error("the hop should be closed with the server")
	}
}

func TestJumpedServerUsesTheJumpHostsVPN(t *testing.T) {
	a, fv, _, inner := rigJump(t) // Inner has no VPN; its jump host "a" uses f
	if r := a.Connect(inner.ID); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(fv.up) != 1 || fv.up[0] != "f" {
		t.Fatalf("vpn up calls %v", fv.up)
	}
}

func TestOwnVPNBeatsTheJumpHostsVPN(t *testing.T) {
	a, fv, _, inner := rigJump(t)
	inner.VPNID = "h"
	if _, err := a.st.SaveServer(inner, ""); err != nil {
		t.Fatal(err)
	}
	go func() { // Hillstone is connected by hand
		for i := 0; i < 200; i++ {
			a.ConfirmManualVPN(inner.ID)
			time.Sleep(10 * time.Millisecond)
		}
	}()
	if r := a.Connect(inner.ID); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(fv.up) != 0 {
		t.Fatalf("only the server's own VPN applies: %v", fv.up)
	}
}

func TestRouteIsCheckedOnTheHostWeActuallyDial(t *testing.T) {
	a, _, _, inner := rigJump(t)
	var probed []string
	a.probe = func(_ context.Context, host string, _ int) error { probed = append(probed, host); return nil }
	if r := a.Connect(inner.ID); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(probed) == 0 || probed[0] != "h" { // the jump host's address, not 10.1.1.1
		t.Fatalf("probed %v", probed)
	}
}
