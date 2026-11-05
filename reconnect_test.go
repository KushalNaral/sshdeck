package main

import (
	"errors"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/vpn"
	"strings"
	"testing"
	"time"
)

func linkStates(ev *events, id string) []string {
	ev.mu.Lock()
	defer ev.mu.Unlock()
	var out []string
	for _, x := range ev.all["link:"+id] {
		out = append(out, x.(Link).State)
	}
	return out
}

func lastLink(ev *events, id string) Link {
	ev.mu.Lock()
	defer ev.mu.Unlock()
	if l, ok := ev.all["link:"+id]; ok && len(l) > 0 {
		return l[len(l)-1].(Link)
	}
	return Link{}
}

func until(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 400; i++ {
		if ok() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func fast(a *App) {
	a.backoff = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond}
}

func TestDroppedLinkReconnectsOnItsOwn(t *testing.T) {
	a, _, ev, d := rigDial(t, okOpen(newShell()))
	fast(a)
	tr := &trail{}
	a.audit = tr.add
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	d.last().drop()
	until(t, "live again", func() bool { return lastLink(ev, "direct").State == "live" })
	if d.count() != 2 {
		t.Errorf("dials = %d, want 2", d.count())
	}
	if st := linkStates(ev, "direct"); st[0] != "reconnecting" {
		t.Errorf("states = %v", st)
	}
	if tr.find("reconnect") == nil {
		t.Error("reconnect not audited")
	}
	// A second drop is noticed too: the watcher keeps going.
	d.last().drop()
	until(t, "third dial", func() bool { return d.count() == 3 })
}

func TestVPNIsBroughtBackBeforeSSH(t *testing.T) {
	a, fv, ev, d := rigDial(t, okOpen(newShell()))
	fast(a)
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	fv.mu.Lock()
	fv.dead = true // the tunnel died along with the SSH link
	fv.mu.Unlock()
	d.last().drop()
	until(t, "live again", func() bool { return lastLink(ev, "a").State == "live" })
	fv.mu.Lock()
	defer fv.mu.Unlock()
	if len(fv.up) != 2 {
		t.Errorf("vpn up calls = %v, want the VPN started again", fv.up)
	}
}

func TestGivesUpAfterFiveFailures(t *testing.T) {
	a, _, ev, d := rigDial(t, okOpen(newShell()))
	fast(a)
	tr := &trail{}
	a.audit = tr.add
	a.Connect("direct")
	d.mu.Lock()
	d.err = errors.New("cannot reach h:22: connection refused")
	d.mu.Unlock()
	d.last().drop()
	until(t, "lost", func() bool { return lastLink(ev, "direct").State == "lost" })
	n := 0
	for _, s := range linkStates(ev, "direct") {
		if s == "reconnecting" {
			n++
		}
	}
	if n != 5 {
		t.Errorf("attempts = %d, want 5", n)
	}
	if l := lastLink(ev, "direct"); l.Problem == "" || l.Recovery == "" {
		t.Errorf("lost without a reason: %+v", l)
	}
	if tr.find("reconnect.fail") == nil {
		t.Error("failure not audited")
	}
}

func TestWrongPasswordIsNotRetried(t *testing.T) {
	a, _, ev, d := rigDial(t, okOpen(newShell()))
	fast(a)
	a.Connect("direct")
	d.mu.Lock()
	d.err = errors.New("wrong user or password")
	d.mu.Unlock()
	d.last().drop()
	until(t, "lost", func() bool { return lastLink(ev, "direct").State == "lost" })
	n := 0
	for _, s := range linkStates(ev, "direct") {
		if s == "reconnecting" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("attempts = %d, want 1: retrying a refused login can lock the account", n)
	}
}

func TestDisconnectDuringBackoffStopsTheWatcher(t *testing.T) {
	a, _, ev, d := rigDial(t, okOpen(newShell()))
	a.backoff = []time.Duration{150 * time.Millisecond}
	a.Connect("direct")
	d.last().drop()
	until(t, "reconnecting", func() bool { return lastLink(ev, "direct").State == "reconnecting" })
	a.Disconnect("direct")
	time.Sleep(300 * time.Millisecond)
	if d.count() != 1 {
		t.Errorf("dials = %d: a disconnected server must not be redialled", d.count())
	}
	if lastLink(ev, "direct").State == "live" {
		t.Error("reported live after disconnect")
	}
}

func TestAutoReconnectCanBeTurnedOff(t *testing.T) {
	a, _, ev, d := rigDial(t, okOpen(newShell()))
	fast(a)
	if err := a.SetAutoReconnect(false); err != nil {
		t.Fatal(err)
	}
	a.Connect("direct")
	d.last().drop()
	until(t, "lost", func() bool { return lastLink(ev, "direct").State == "lost" })
	time.Sleep(50 * time.Millisecond)
	if d.count() != 1 {
		t.Errorf("dials = %d, want no redial when automatic reconnect is off", d.count())
	}
	if a.LockState().AutoReconnect {
		t.Error("LockState.AutoReconnect should be false")
	}
}

func TestADeadTunnelIsNoticedWithoutWaitingForSSH(t *testing.T) {
	a, fv, ev, d := rigDial(t, okOpen(newShell()))
	fast(a)
	a.vpnPoll = 5 * time.Millisecond
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	fv.mu.Lock()
	fv.dead = true // the tunnel is gone; the SSH client has not noticed yet
	fv.mu.Unlock()
	until(t, "live again", func() bool { return lastLink(ev, "a").State == "live" })
	fv.mu.Lock()
	ups := len(fv.up)
	fv.mu.Unlock()
	if ups != 2 || d.count() != 2 {
		t.Errorf("vpn up calls %d, dials %d: want the VPN and SSH both restarted once", ups, d.count())
	}
}

func TestVPNPollStopsOnDisconnect(t *testing.T) {
	a, fv, _, d := rigDial(t, okOpen(newShell()))
	fast(a)
	a.vpnPoll = 5 * time.Millisecond
	a.Connect("a")
	a.Disconnect("a")
	fv.mu.Lock()
	fv.dead = true
	fv.mu.Unlock()
	time.Sleep(60 * time.Millisecond)
	if d.count() != 1 {
		t.Errorf("dials = %d: a disconnected server must not be reconnected because its VPN died", d.count())
	}
}

func TestVPNStepShowsWhatTheClientSaysWhileConnecting(t *testing.T) {
	a, _, ev := rig(t, okOpen(newShell()))
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	var details []string
	ev.mu.Lock()
	for _, x := range ev.all["connect:a"] {
		for _, s := range x.([]Step) {
			if s.Key == "vpn" {
				details = append(details, s.Detail)
			}
		}
	}
	ev.mu.Unlock()
	var saw bool
	for _, d := range details {
		if d == "Forti · Attempting to connect to server" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("details = %v, want the client's line shown next to the VPN name", details)
	}
	if last := details[len(details)-1]; last != "Forti" {
		t.Errorf("the finished step should show only the VPN name, got %q", last)
	}
}

func TestShortenKeepsLongLinesReadable(t *testing.T) {
	if got := shorten("abcdef", 4); got != "abc…" {
		t.Errorf("got %q", got)
	}
	if got := shorten("abc", 4); got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestFailedVPNConnectGivesTheClientOutputAsDetails(t *testing.T) {
	a, fv, _ := rig(t, okOpen(newShell()))
	fv.err = &vpn.ClientError{Err: errors.New("Failed to complete authentication"), Command: "openconnect --protocol=gp gw", Output: []string{"POST https://gw/login", "Failed to complete authentication"}}
	r := a.Connect("a")
	if r.OK || !strings.Contains(r.Details, "$ openconnect --protocol=gp gw") || !strings.Contains(r.Details, "POST https://gw/login") {
		t.Fatalf("result = %+v", r)
	}
	if !strings.Contains(r.Recovery, "user") || !strings.Contains(r.Recovery, "official") {
		t.Errorf("recovery = %q, want a hint about credentials and the official client", r.Recovery)
	}
}

func TestAVPNMarkedManualWaitsForYouLikeHillstone(t *testing.T) {
	a, fv, ev := rig(t, okOpen(newShell()))
	v, err := a.st.SaveVPN(store.VPN{Name: "Official GP", Kind: store.GP, Gateway: "gp.example.org", Port: 443, User: "u", Manual: true}, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.st.SaveServer(store.Server{ID: "m", Name: "M", Host: "h", Port: 22, User: "u", Auth: "password", VPNID: v.ID}, "pw"); err != nil {
		t.Fatal(err)
	}
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("m") }()
	until(t, "waiting for the user", func() bool {
		steps, _ := ev.get("connect:m").([]Step)
		return len(steps) > 0 && steps[0].Status == "manual"
	})
	a.ConfirmManualVPN("m")
	if r := <-done; !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(fv.up) != 0 {
		t.Errorf("sshdeck started the client for a VPN you connect yourself: %v", fv.up)
	}
}

func TestPortal512IsExplainedAsARefusedLogin(t *testing.T) {
	a, fv, _ := rig(t, okOpen(newShell()))
	fv.err = &vpn.ClientError{Err: errors.New("Failed to complete authentication"), Command: "openconnect --protocol=gp gw", Output: []string{
		"POST https://gw/global-protect/getconfig.esp", "Got HTTP response: HTTP/1.1 512 Custom error", "Unexpected 512 result from server", "Failed to complete authentication"}}
	r := a.Connect("a")
	for _, want := range []string{"512", "password", "Sign in at", "myself"} {
		if !strings.Contains(r.Recovery, want) {
			t.Errorf("recovery %q should mention %q", r.Recovery, want)
		}
	}
}
