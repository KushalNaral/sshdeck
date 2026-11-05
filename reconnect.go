package main

import (
	"context"
	"strings"
	"time"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/vpn"
)

// Link is sent to the UI as "link:<serverId>" while a dropped connection is being redialled.
type Link struct {
	State    string `json:"state"` // reconnecting, live or lost
	Attempt  int    `json:"attempt,omitempty"`
	Max      int    `json:"max,omitempty"`
	Problem  string `json:"problem,omitempty"`
	Recovery string `json:"recovery,omitempty"`
}

// watch waits for the connection's SSH link to end and, unless the user disconnected, brings it back.
// It runs until Disconnect cancels ctx.
func (a *App) watch(ctx context.Context, serverID string, c *conn, vp *store.VPN) {
	if vp != nil && vp.Kind != store.Hillstone {
		go a.watchTunnel(ctx, c, vp)
	}
	for {
		a.mu.Lock()
		cl := c.client
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-cl.Done():
		}
		if ctx.Err() != nil {
			return
		}
		a.reconnect(ctx, serverID, c, vp)
		a.waitReplaced(ctx, c, cl)
	}
}

// watchTunnel notices a VPN tunnel that died before the SSH keepalive does. It ends the SSH link, and the watcher
// above then restarts the VPN and redials as for any other drop.
func (a *App) watchTunnel(ctx context.Context, c *conn, vp *store.VPN) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(a.vpnPoll):
		}
		if a.vpn.Status(vp.ID) != vpn.Down {
			continue
		}
		a.mu.Lock()
		cl := c.client
		a.mu.Unlock()
		select {
		case <-cl.Done(): // already dropped: the watcher is on it
		default:
			if ctx.Err() == nil {
				a.audit("vpn.lost", vp.Name, "")
				_ = cl.Close()
			}
		}
	}
}

// waitReplaced returns once c has a different client than cl (someone reconnected) or ctx ends.
func (a *App) waitReplaced(ctx context.Context, c *conn, cl sshx.Client) {
	for {
		a.mu.Lock()
		cur := c.client
		a.mu.Unlock()
		if cur != cl {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// retryable is false for failures that more attempts cannot fix, and that could lock the account if repeated.
func retryable(r ConnectResult) bool {
	p := r.Problem + " " + r.Recovery
	return !strings.Contains(p, "wrong user or password") && !strings.Contains(p, "did not accept the key") && !strings.Contains(p, "host key")
}

func (a *App) reconnect(ctx context.Context, serverID string, c *conn, vp *store.VPN) {
	name := a.serverName(serverID)
	a.mu.Lock()
	on := !a.settings.NoReconnect
	a.mu.Unlock()
	if !on {
		a.emit("link:"+serverID, Link{State: "lost", Problem: "The connection to " + name + " dropped.", Recovery: "Press Reconnect in the terminal, or connect again."})
		return
	}
	for a.isLocked() { // the vault is locked, so the password is out of reach: wait for the unlock
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
	last := fail("The connection to "+name+" dropped.", "Connect again.")
	max := len(a.backoff)
	for i := 1; i <= max; i++ {
		a.emit("link:"+serverID, Link{State: "reconnecting", Attempt: i, Max: max})
		select {
		case <-ctx.Done():
			return
		case <-time.After(a.backoff[i-1]):
		}
		sv, ok := a.server(serverID)
		if !ok {
			return
		}
		if vp != nil && vp.Kind != store.Hillstone && a.vpn.Status(vp.ID) != vpn.Up { // Hillstone is connected by hand: nothing to restart
			if res, bad := a.vpnUp(ctx, *vp, func(string, func(*Step)) {}); bad {
				last = res
				continue
			}
		}
		pw, err := a.serverSecret(sv)
		if err != nil {
			last = fail("No password is stored for "+sv.Name+".", "Open Admin, edit the server and enter its password.")
			break
		}
		dctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = a.liveClient(dctx, c, sv, pw)
		cancel()
		if err == nil {
			a.emit("link:"+serverID, Link{State: "live"})
			a.audit("reconnect", name, "")
			return
		}
		if ctx.Err() != nil {
			return
		}
		if last = sshFailure(err); !retryable(last) {
			break
		}
	}
	a.emit("link:"+serverID, Link{State: "lost", Problem: last.Problem, Recovery: last.Recovery})
	a.audit("reconnect.fail", name, last.Problem)
}
