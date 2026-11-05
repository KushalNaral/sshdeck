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
	"golang.org/x/crypto/ssh/agent"
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
	// Close ends the session. A session from Open also closes its connection; a shell of a shared Client leaves the
	// connection to the Client. Safe to call twice.
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

// authMethod builds the login method for srv. For key login, secret is the key's passphrase (empty if it has none).
// With no key file set, keys come from the running ssh-agent.
func authMethod(srv store.Server, secret string) (ssh.AuthMethod, func(), error) {
	none := func() {}
	if srv.Auth != "key" {
		return ssh.Password(secret), none, nil
	}
	if srv.KeyPath == "" {
		c, err := dialAgent(srv.Name)
		if err != nil {
			return nil, none, err
		}
		return ssh.PublicKeysCallback(agent.NewClient(c).Signers), func() { c.Close() }, nil
	}
	path := srv.KeyPath
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, rest)
		}
	}
	pem, err := os.ReadFile(path)
	if err != nil {
		return nil, none, fmt.Errorf("cannot read the key file %s: %w", srv.KeyPath, err)
	}
	signer, err := ssh.ParsePrivateKey(pem)
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) {
		if secret == "" {
			return nil, none, fmt.Errorf("the key %s is protected by a passphrase: enter it in Admin", srv.KeyPath)
		}
		if signer, err = ssh.ParsePrivateKeyWithPassphrase(pem, []byte(secret)); err != nil {
			return nil, none, fmt.Errorf("wrong passphrase for the key %s", srv.KeyPath)
		}
	} else if err != nil {
		return nil, none, fmt.Errorf("cannot use the key file %s: %w", srv.KeyPath, err)
	}
	return ssh.PublicKeys(signer), none, nil
}

func dial(ctx context.Context, srv store.Server, secret string) (*ssh.Client, error) {
	method, closeAuth, err := authMethod(srv, secret)
	if err != nil {
		return nil, err
	}
	defer closeAuth() // signing happens during the handshake, so an agent connection is only needed until then
	hk, err := hostKeyCallback()
	if err != nil {
		return nil, err
	}
	cfg := &ssh.ClientConfig{User: srv.User, Auth: []ssh.AuthMethod{method}, HostKeyCallback: hk, Timeout: 15 * time.Second}
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
		if strings.Contains(err.Error(), "unable to authenticate") && srv.Auth == "key" {
			return nil, fmt.Errorf("login to %s as %s failed: the server did not accept the key: %w", addr, srv.User, err)
		}
		if strings.Contains(err.Error(), "unable to authenticate") {
			return nil, fmt.Errorf("login to %s as %s failed: wrong user or password: %w", addr, srv.User, err)
		}
		return nil, fmt.Errorf("ssh to %s: %w", addr, err)
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// Client is one authenticated SSH connection that many shells and commands share. One login serves a whole
// working session, which is faster than a login per tab or per command and keeps well under a server's login
// rate limits (and fail2ban).
type Client interface {
	// Shell opens another interactive shell (its own PTY) on this connection.
	Shell(ctx context.Context) (Session, error)
	// Run runs one command and returns its stdout. A non-zero exit is an error that still carries what was printed.
	// Cancelling ctx ends only this command, not the connection.
	Run(ctx context.Context, cmd string) (string, error)
	// Done is closed when the connection has ended, whether by Close, a keepalive failure or the server.
	Done() <-chan struct{}
	// Close ends the connection and every shell on it. Safe to call twice.
	Close() error
}

type client struct {
	c    *ssh.Client
	done chan struct{}
	once sync.Once
}

// Dial connects and logs in. ctx covers connecting only.
func Dial(ctx context.Context, srv store.Server, secret string) (Client, error) {
	c, err := dial(ctx, srv, secret)
	if err != nil {
		return nil, err
	}
	k := &client{c: c, done: make(chan struct{})}
	go keepAlive(c, 15*time.Second, 10*time.Second, k.done, func() { k.Close() })
	go func() { c.Wait(); k.Close() }() // the server or the network ended the connection
	return k, nil
}

func (k *client) Done() <-chan struct{} { return k.done }
func (k *client) Close() error {
	k.once.Do(func() { close(k.done); k.c.Close() })
	return nil
}

// Shell requests a PTY and starts a shell on a new channel of this connection.
func (k *client) Shell(ctx context.Context) (Session, error) {
	s, err := k.c.NewSession()
	if err != nil {
		return nil, err
	}
	fail := func(err error) (Session, error) { s.Close(); return nil, err }
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
	x := &session{s: s, in: in, out: make(chan []byte, 16), done: make(chan struct{})}
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

func (k *client) Run(ctx context.Context, cmd string) (string, error) {
	s, err := k.c.NewSession()
	if err != nil {
		return "", err
	}
	defer s.Close()
	stop := context.AfterFunc(ctx, func() { s.Close() }) // ctx ending unblocks a hung command, for this command only
	defer stop()
	out, err := s.Output(cmd)
	if ctx.Err() != nil {
		return string(out), ctx.Err()
	}
	return string(out), err
}

// Open connects to srv with the password (secret), requests a PTY and starts a shell. The returned session owns
// its connection: closing it closes the connection. ctx covers connecting only; it does not end the session later.
func Open(ctx context.Context, srv store.Server, secret string) (Session, error) {
	c, err := Dial(ctx, srv, secret)
	if err != nil {
		return nil, err
	}
	sess, err := c.Shell(ctx)
	if err != nil {
		c.Close()
		return nil, err
	}
	sess.(*session).owner = c
	return sess, nil
}

// requester is the part of *ssh.Client that keepAlive needs.
type requester interface {
	SendRequest(name string, wantReply bool, payload []byte) (bool, []byte, error)
}

// keepAlive probes the server every `every` until done closes. A probe that fails or gets no answer within
// `wait` means the link is dead (sleep, Wi-Fi change, VPN drop): onDead runs once and it stops.
func keepAlive(p requester, every, wait time.Duration, done <-chan struct{}, onDead func()) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
		}
		res := make(chan error, 1)
		go func() { _, _, err := p.SendRequest("keepalive@openssh.com", true, nil); res <- err }()
		select {
		case err := <-res:
			if err == nil {
				continue
			}
		case <-time.After(wait):
		case <-done:
			return
		}
		onDead()
		return
	}
}

type session struct {
	owner Client // set when the session owns its connection (Open); nil for shells of a shared Client
	s     *ssh.Session
	in    interface{ Write([]byte) (int, error) }
	out   chan []byte
	done  chan struct{}
	once  sync.Once
}

func (x *session) Write(p []byte) error  { _, err := x.in.Write(p); return err }
func (x *session) Resize(c, r int) error { return x.s.WindowChange(r, c) }
func (x *session) Output() <-chan []byte { return x.out }
func (x *session) Close() error {
	x.once.Do(func() {
		close(x.done)
		x.s.Close()
		if x.owner != nil {
			x.owner.Close()
		}
	})
	return nil
}

// Run connects, runs one command and returns its stdout, then disconnects. A non-zero exit is an error that
// still carries whatever was printed. For several commands use Dial and Client.Run.
func Run(ctx context.Context, srv store.Server, secret, cmd string) (string, error) {
	c, err := Dial(ctx, srv, secret)
	if err != nil {
		return "", err
	}
	defer c.Close()
	return c.Run(ctx, cmd)
}
