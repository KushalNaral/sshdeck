// Package sshx opens SSH sessions to a server: an interactive shell with a PTY, and one-shot commands.
package sshx

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// KnownHosts is the file that remembers server host keys. Empty means <user config dir>/sshdeck/known_hosts.
var KnownHosts string

// Session is one interactive shell on a server.
type Session interface {
	// Write sends keystrokes to the shell.
	Write(p []byte) error
	// Resize tells the remote PTY the terminal is now cols x rows.
	Resize(cols, rows int) error
	// Output streams what the shell prints (stdout and stderr merged by the PTY).
	// The channel is closed when the session ends, whether by Close or by the remote side.
	Output() <-chan []byte
	// Close ends the session and releases the connection. Safe to call twice.
	Close() error
}

var hostsMu sync.Mutex

func knownHostsPath() (string, error) {
	if KnownHosts != "" {
		return KnownHosts, nil
	}
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "sshdeck", "known_hosts"), nil
}

// hostKeyCallback trusts a host the first time it is seen and remembers its key; a changed key is refused.
func hostKeyCallback() (ssh.HostKeyCallback, error) {
	path, err := knownHostsPath()
	if err != nil {
		return nil, err
	}
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		hostsMu.Lock()
		defer hostsMu.Unlock()
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
		if err != nil {
			return err
		}
		f.Close()
		check, err := knownhosts.New(path)
		if err != nil {
			return err
		}
		err = check(hostname, remote, key)
		var ke *knownhosts.KeyError
		switch {
		case err == nil:
			return nil
		case errors.As(err, &ke) && len(ke.Want) == 0: // never seen: trust on first use
			f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				return err
			}
			defer f.Close()
			_, err = fmt.Fprintln(f, knownhosts.Line([]string{hostname}, key))
			return err
		case errors.As(err, &ke):
			return fmt.Errorf("the host key for %s changed since it was first saved; if the server was rebuilt, remove its line from %s: %w", hostname, path, err)
		}
		return err
	}, nil
}

func dial(ctx context.Context, srv store.Server, secret string) (*ssh.Client, error) {
	if srv.Auth != "password" {
		return nil, fmt.Errorf("%s uses key login, which is not supported yet; switch it to password in Admin", srv.Name)
	}
	hk, err := hostKeyCallback()
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{User: srv.User, Auth: []ssh.AuthMethod{ssh.Password(secret)}, HostKeyCallback: hk, Timeout: 15 * time.Second}
	addr := net.JoinHostPort(srv.Host, strconv.Itoa(srv.Port))
	nc, err := (&net.Dialer{Timeout: cfg.Timeout}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", addr, err)
	}
	// Bound the handshake by ctx too: closing the conn unblocks NewClientConn.
	stop := context.AfterFunc(ctx, func() { nc.Close() })
	defer stop()
	c, chans, reqs, err := ssh.NewClientConn(nc, addr, cfg)
	if err != nil {
		nc.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, fmt.Errorf("login to %s as %s failed: wrong user or password: %w", addr, srv.User, err)
		}
		return nil, fmt.Errorf("ssh to %s: %w", addr, err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// Open connects to srv with the password (secret), requests a PTY and starts a shell.
// ctx covers connecting only; it does not end the session later.
func Open(ctx context.Context, srv store.Server, secret string) (Session, error) {
	c, err := dial(ctx, srv, secret)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (Session, error) { c.Close(); return nil, err }
	s, err := c.NewSession()
	if err != nil {
		return fail(err)
	}
	if err := s.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{ssh.ECHO: 1}); err != nil {
		return fail(err)
	}
	in, err := s.StdinPipe()
	if err != nil {
		return fail(err)
	}
	o, err := s.StdoutPipe()
	if err != nil {
		return fail(err)
	}
	if err := s.Shell(); err != nil {
		return fail(err)
	}
	x := &session{c: c, s: s, in: in, out: make(chan []byte, 16), done: make(chan struct{})}
	go func() {
		defer close(x.out)
		buf := make([]byte, 32*1024)
		for {
			n, err := o.Read(buf)
			if n > 0 {
				select {
				case x.out <- append([]byte(nil), buf[:n]...):
				case <-x.done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return x, nil
}

type session struct {
	c    *ssh.Client
	s    *ssh.Session
	in   interface{ Write([]byte) (int, error) }
	out  chan []byte
	done chan struct{}
	once sync.Once
}

func (x *session) Write(p []byte) error  { _, err := x.in.Write(p); return err }
func (x *session) Resize(c, r int) error { return x.s.WindowChange(r, c) }
func (x *session) Output() <-chan []byte { return x.out }
func (x *session) Close() error {
	x.once.Do(func() { close(x.done); x.s.Close(); x.c.Close() })
	return nil
}

// Run connects, runs one command and returns its stdout. A non-zero exit is an error that still
// carries whatever was printed. Used by probe for read-only analysis.
func Run(ctx context.Context, srv store.Server, secret, cmd string) (string, error) {
	c, err := dial(ctx, srv, secret)
	if err != nil {
		return "", err
	}
	defer c.Close()
	s, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	stop := context.AfterFunc(ctx, func() { c.Close() }) // ctx ending unblocks a hung command
	defer stop()
	out, err := s.Output(cmd)
	if ctx.Err() != nil {
		return string(out), ctx.Err()
	}
	return string(out), err
}
