package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
)

// chain is sv followed by each jump host in turn. The last one is the host this machine connects to.
// The store refuses loops and long chains, so this ends.
func (a *App) chain(sv store.Server) []store.Server {
	out := []store.Server{sv}
	for sv.JumpID != "" {
		next, ok := a.server(sv.JumpID)
		if !ok {
			break
		}
		out = append(out, next)
		sv = next
	}
	return out
}

// entry is the host this machine dials. Without a jump host that is sv itself.
func (a *App) entry(sv store.Server) store.Server {
	c := a.chain(sv)
	return c[len(c)-1]
}

// vpnFor is the VPN a server needs: its own, otherwise the nearest jump host's.
func (a *App) vpnFor(sv store.Server) string {
	for _, s := range a.chain(sv) {
		if s.VPNID != "" {
			return s.VPNID
		}
	}
	return ""
}

// jumped is a login that went through another: closing it closes the hop too.
type jumped struct {
	sshx.Client
	via sshx.Client
}

func (j *jumped) Close() error {
	err := j.Client.Close()
	j.via.Close()
	return err
}

// dialServer logs in to sv, through its jump hosts if it has any.
func (a *App) dialServer(ctx context.Context, sv store.Server, pw string) (sshx.Client, error) {
	if sv.JumpID == "" {
		return a.dial(ctx, sv, pw)
	}
	hop, ok := a.server(sv.JumpID)
	if !ok {
		return nil, errors.New("the jump host for " + sv.Name + " no longer exists")
	}
	hpw, err := a.serverSecret(hop)
	if err != nil {
		return nil, errors.New("no password is stored for the jump host " + hop.Name)
	}
	via, err := a.dialServer(ctx, hop, hpw)
	if err != nil {
		return nil, errors.New("jump host " + hop.Name + ": " + err.Error())
	}
	c, err := a.dialThrough(ctx, via, sv, pw)
	if err != nil {
		via.Close()
		return nil, err
	}
	return &jumped{Client: c, via: via}, nil
}

// sshDetail is the line under the SSH step: who and where, and through which jump host.
func (a *App) sshDetail(sv store.Server) string {
	d := fmt.Sprintf("%s@%s:%d", sv.User, sv.Host, sv.Port)
	if hop, ok := a.server(sv.JumpID); ok {
		d += " via " + hop.Name
	}
	return d
}
