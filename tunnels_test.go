package main

import (
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// echo listens on a free local port and sends back whatever it is given.
func echo(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { io.Copy(c, c); c.Close() }()
		}
	}()
	return l.Addr().String()
}

func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

// rigTunnel connects "direct" with one tunnel of the given kind and returns the app and the tunnel's id.
func rigTunnel(t *testing.T, kind, listen, target string) (*App, string) {
	t.Helper()
	a, _, _ := rig(t, okOpen(newShell()))
	sv, _ := a.server("direct")
	sv.Tunnels = []store.Tunnel{{Name: "t", Kind: kind, Listen: listen, Target: target}}
	saved, err := a.st.SaveServer(sv, "")
	if err != nil {
		t.Fatal(err)
	}
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	return a, saved.Tunnels[0].ID
}

func ping(t *testing.T, addr string) string {
	t.Helper()
	c, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return "dial: " + err.Error()
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := c.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(c, buf); err != nil {
		return "read: " + err.Error()
	}
	return string(buf)
}

func TestLocalTunnelCarriesTraffic(t *testing.T) {
	target, listen := echo(t), freeAddr(t)
	a, id := rigTunnel(t, "local", listen, target)
	if err := a.StartTunnel("direct", id); err != nil {
		t.Fatal(err)
	}
	if got := ping(t, listen); got != "ping" {
		t.Fatalf("got %q", got)
	}
	if got := a.liveClientFor("direct").dials(); len(got) != 1 || got[0] != target {
		t.Fatalf("should reach the target through the SSH client: %v", got)
	}
	if st := a.Tunnels("direct"); len(st) != 1 || st[0].State != "running" {
		t.Fatalf("%+v", st)
	}
}

func TestTunnelNeedsAConnectionAndAFreePort(t *testing.T) {
	target := echo(t)
	taken, _ := net.Listen("tcp", "127.0.0.1:0")
	defer taken.Close()
	a, id := rigTunnel(t, "local", taken.Addr().String(), target)
	err := a.StartTunnel("direct", id)
	if err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("a busy port should say so: %v", err)
	}
	a.Disconnect("direct")
	if err := a.StartTunnel("direct", id); err == nil || !strings.Contains(err.Error(), "Connect") {
		t.Fatalf("not connected: %v", err)
	}
}

func TestStopClosesOpenConnections(t *testing.T) {
	target, listen := echo(t), freeAddr(t)
	a, id := rigTunnel(t, "local", listen, target)
	if err := a.StartTunnel("direct", id); err != nil {
		t.Fatal(err)
	}
	c, err := net.Dial("tcp", listen)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.Write([]byte("x"))
	for i := 0; i < 200 && a.Tunnels("direct")[0].Conns == 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	a.StopTunnel("direct", id)
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 8)
	for {
		if _, err := c.Read(buf); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				t.Fatal("the open connection should have been closed")
			}
			break
		}
	}
	if _, err := net.DialTimeout("tcp", listen, 200*time.Millisecond); err == nil {
		t.Error("the port should be free again")
	}
}

func TestDisconnectStopsTunnels(t *testing.T) {
	target, listen := echo(t), freeAddr(t)
	a, id := rigTunnel(t, "local", listen, target)
	if err := a.StartTunnel("direct", id); err != nil {
		t.Fatal(err)
	}
	a.Disconnect("direct")
	if _, err := net.DialTimeout("tcp", listen, 200*time.Millisecond); err == nil {
		t.Error("the listener should be gone after Disconnect")
	}
	if st := a.Tunnels("direct"); st[0].State == "running" {
		t.Fatalf("%+v", st)
	}
}

func TestTunnelFollowsTheReconnectedClient(t *testing.T) {
	target, listen := echo(t), freeAddr(t)
	a, id := rigTunnel(t, "local", listen, target)
	if err := a.StartTunnel("direct", id); err != nil {
		t.Fatal(err)
	}
	fresh := &fakeClient{open: okOpen(newShell()), done: make(chan struct{})}
	a.mu.Lock()
	a.conns["direct"].client = fresh
	a.mu.Unlock()
	if got := ping(t, listen); got != "ping" {
		t.Fatalf("got %q", got)
	}
	if len(fresh.dials()) != 1 {
		t.Fatalf("traffic should use the new client: %v", fresh.dials())
	}
}

func TestRemoteTunnelReachesALocalTarget(t *testing.T) {
	target, listen := echo(t), freeAddr(t)
	a, id := rigTunnel(t, "remote", listen, target)
	if err := a.StartTunnel("direct", id); err != nil {
		t.Fatal(err)
	}
	if got := ping(t, listen); got != "ping" {
		t.Fatalf("got %q", got)
	}
}

func TestAutoTunnelsStartWithTheConnection(t *testing.T) {
	target, listen := echo(t), freeAddr(t)
	a, _, _ := rig(t, okOpen(newShell()))
	sv, _ := a.server("direct")
	sv.Tunnels = []store.Tunnel{{Name: "t", Kind: "local", Listen: listen, Target: target, Auto: true}}
	if _, err := a.st.SaveServer(sv, ""); err != nil {
		t.Fatal(err)
	}
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ping(t, listen) == "ping" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the auto tunnel never came up")
}
