package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
)

// TunnelState is what the UI shows for one saved tunnel.
type TunnelState struct {
	ID    string `json:"id"`
	State string `json:"state"` // "running" or "stopped"
	Error string `json:"error,omitempty"`
	Conns int    `json:"conns"` // open connections right now
}

type tunnelRun struct {
	cancel context.CancelFunc
	mu     sync.Mutex
	ln     net.Listener
	open   map[net.Conn]struct{}
	err    string
}

func tunnelKey(serverID, id string) string { return serverID + "/" + id }

// clientOf is the server's current SSH client. It changes when the link is redialled, so tunnels ask each time.
func (a *App) clientOf(serverID string) sshx.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.conns[serverID]; c != nil {
		return c.client
	}
	return nil
}

func (a *App) findTunnel(serverID, id string) (store.Tunnel, error) {
	if sv, ok := a.server(serverID); ok {
		for _, t := range sv.Tunnels {
			if t.ID == id {
				return t, nil
			}
		}
	}
	return store.Tunnel{}, errors.New("that tunnel no longer exists")
}

func listenErr(err error, addr string) error {
	if errors.Is(err, syscall.EADDRINUSE) || strings.Contains(err.Error(), "address already in use") {
		return fmt.Errorf("%s is already in use. Pick another port.", addr)
	}
	return err
}

// StartTunnel opens a saved tunnel on a connected server. A busy port is reported here, not later.
func (a *App) StartTunnel(serverID, id string) error {
	t, err := a.findTunnel(serverID, id)
	if err != nil {
		return err
	}
	cl := a.clientOf(serverID)
	if cl == nil {
		return errors.New("Connect to the server first.")
	}
	key := tunnelKey(serverID, id)
	a.tmu.Lock()
	if a.tuns[key] != nil {
		a.tmu.Unlock()
		return nil
	}
	a.tmu.Unlock()
	var ln net.Listener
	if t.Kind == "local" {
		ln, err = net.Listen("tcp", t.Listen)
	} else {
		ln, err = cl.Listen("tcp", t.Listen)
	}
	if err != nil {
		return listenErr(err, t.Listen)
	}
	ctx, cancel := context.WithCancel(a.ctx)
	r := &tunnelRun{cancel: cancel, ln: ln, open: map[net.Conn]struct{}{}}
	a.tmu.Lock()
	a.tuns[key] = r
	a.tmu.Unlock()
	go a.serveTunnel(ctx, serverID, t, r)
	a.audit("tunnel.start", a.serverName(serverID), t.Name+" "+t.Listen+" -> "+t.Target)
	a.tunnelsChanged(serverID)
	return nil
}

func (a *App) serveTunnel(ctx context.Context, serverID string, t store.Tunnel, r *tunnelRun) {
	for {
		r.mu.Lock()
		ln := r.ln
		r.mu.Unlock()
		for {
			c, err := ln.Accept()
			if err != nil {
				break
			}
			go a.carry(ctx, serverID, t, r, c)
		}
		if ctx.Err() != nil || t.Kind == "local" {
			return
		}
		// A remote forward lives on the SSH link: when the link is redialled, listen again on the new one.
		for ctx.Err() == nil {
			time.Sleep(time.Second)
			cl := a.clientOf(serverID)
			if cl == nil {
				continue
			}
			nl, err := cl.Listen("tcp", t.Listen)
			if err != nil {
				continue
			}
			r.mu.Lock()
			r.ln = nl
			r.mu.Unlock()
			break
		}
	}
}

// carry joins one accepted connection to the other end of the tunnel.
func (a *App) carry(ctx context.Context, serverID string, t store.Tunnel, r *tunnelRun, c net.Conn) {
	var far net.Conn
	var err error
	if t.Kind == "local" {
		cl := a.clientOf(serverID)
		if cl == nil {
			c.Close()
			return
		}
		far, err = cl.Dial("tcp", t.Target)
	} else {
		far, err = net.DialTimeout("tcp", t.Target, 10*time.Second)
	}
	if err != nil {
		r.mu.Lock()
		r.err = err.Error()
		r.mu.Unlock()
		c.Close()
		a.tunnelsChanged(serverID)
		return
	}
	r.mu.Lock()
	if ctx.Err() != nil { // stopped while this connection was being dialled
		r.mu.Unlock()
		c.Close()
		far.Close()
		return
	}
	r.err = ""
	r.open[c], r.open[far] = struct{}{}, struct{}{}
	r.mu.Unlock()
	a.tunnelsChanged(serverID)
	done := make(chan struct{}, 2)
	for _, p := range [][2]net.Conn{{c, far}, {far, c}} {
		go func() { io.Copy(p[0], p[1]); done <- struct{}{} }()
	}
	<-done
	c.Close()
	far.Close()
	<-done
	r.mu.Lock()
	delete(r.open, c)
	delete(r.open, far)
	r.mu.Unlock()
	a.tunnelsChanged(serverID)
}

// StopTunnel closes a tunnel and every connection through it.
func (a *App) StopTunnel(serverID, id string) {
	key := tunnelKey(serverID, id)
	a.tmu.Lock()
	r := a.tuns[key]
	delete(a.tuns, key)
	a.tmu.Unlock()
	if r == nil {
		return
	}
	r.cancel()
	r.mu.Lock()
	r.ln.Close()
	for c := range r.open {
		c.Close()
	}
	r.mu.Unlock()
	if t, err := a.findTunnel(serverID, id); err == nil {
		a.audit("tunnel.stop", a.serverName(serverID), t.Name)
	}
	a.tunnelsChanged(serverID)
}

func (a *App) stopTunnels(serverID string) {
	a.tmu.Lock()
	var ids []string
	for k := range a.tuns {
		if id, ok := strings.CutPrefix(k, serverID+"/"); ok {
			ids = append(ids, id)
		}
	}
	a.tmu.Unlock()
	for _, id := range ids {
		a.StopTunnel(serverID, id)
	}
}

// startAutoTunnels runs once a server is connected.
func (a *App) startAutoTunnels(serverID string) {
	sv, ok := a.server(serverID)
	if !ok {
		return
	}
	for _, t := range sv.Tunnels {
		if t.Auto {
			if err := a.StartTunnel(serverID, t.ID); err != nil {
				a.audit("tunnel.fail", sv.Name, t.Name+": "+err.Error())
			}
		}
	}
}

// Tunnels reports every saved tunnel of a server.
func (a *App) Tunnels(serverID string) []TunnelState {
	sv, _ := a.server(serverID)
	out := make([]TunnelState, 0, len(sv.Tunnels))
	a.tmu.Lock()
	defer a.tmu.Unlock()
	for _, t := range sv.Tunnels {
		s := TunnelState{ID: t.ID, State: "stopped"}
		if r := a.tuns[tunnelKey(serverID, t.ID)]; r != nil {
			r.mu.Lock()
			s.State, s.Error, s.Conns = "running", r.err, len(r.open)/2
			r.mu.Unlock()
		}
		out = append(out, s)
	}
	return out
}

func (a *App) tunnelsChanged(serverID string) { a.emit("tunnels:"+serverID, a.Tunnels(serverID)) }
