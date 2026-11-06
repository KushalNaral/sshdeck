package store

import (
	"net"
	"strconv"
)

// Tunnel is a saved port forward over a server's SSH connection.
// Kind "local" listens here (Listen) and reaches Target from the server; "remote" listens on the server and reaches
// Target from here. Listen may be a bare port, which means 127.0.0.1:port.
type Tunnel struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Listen string `json:"listen"`
	Target string `json:"target"`
	Auto   bool   `json:"auto,omitempty"` // start when the server connects
}

// hostPort splits "host:port" (or a bare port, using def as the host) and checks both halves.
func hostPort(s, def string) (string, error) {
	if _, err := strconv.Atoi(s); err == nil && def != "" {
		s = net.JoinHostPort(def, s)
	}
	h, p, err := net.SplitHostPort(s)
	n, perr := strconv.Atoi(p)
	if err != nil || perr != nil || !validPort(n) || h == "" || checkHost(h) != nil {
		return "", invalid("%q is not a host:port address", s)
	}
	return net.JoinHostPort(h, p), nil
}

func checkTunnels(ts []Tunnel) error {
	if len(ts) > 30 {
		return invalid("at most 30 tunnels per server")
	}
	seen := map[string]bool{}
	for _, t := range ts {
		if t.Name == "" || hasControl(t.Name) || hasControl(t.Listen) || hasControl(t.Target) {
			return invalid("tunnel needs a name, and no control characters")
		}
		if t.Kind != "local" && t.Kind != "remote" {
			return invalid("tunnel %q: kind must be local or remote", t.Name)
		}
		l, err := hostPort(t.Listen, "127.0.0.1")
		if err != nil {
			return err
		}
		if _, err := hostPort(t.Target, ""); err != nil {
			return err
		}
		if k := t.Kind + l; seen[k] {
			return invalid("tunnel %q: %s is already used by another tunnel", t.Name, l)
		} else {
			seen[k] = true
		}
	}
	return nil
}

// cleanTunnels gives new tunnels an id and spells every listen address out.
func cleanTunnels(ts []Tunnel) []Tunnel {
	out := make([]Tunnel, len(ts))
	for i, t := range ts {
		if t.ID == "" {
			t.ID = newID(t.Name)
		}
		t.Listen, _ = hostPort(t.Listen, "127.0.0.1")
		out[i] = t
	}
	return out
}

const maxHops = 3

// checkJump refuses a jump host that does not exist, that loops back, or that makes the chain longer than maxHops.
func (s *Store) checkJump(sv Server) error {
	hops, at := 0, sv.JumpID
	for at != "" {
		if at == sv.ID {
			return invalid("%s cannot be reached through itself", sv.Name)
		}
		i := s.serverIndex(at)
		if i < 0 {
			return invalid("unknown jump host %q", at)
		}
		if hops++; hops > maxHops {
			return invalid("at most %d jump hosts in a row", maxHops)
		}
		at = s.d.Servers[i].JumpID
	}
	return nil
}
