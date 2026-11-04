// Package vpn brings a VPN up and down by driving the Linux command-line clients:
// openfortivpn for FortiClient and openconnect (--protocol=gp) for GlobalProtect.
// Hillstone has no Linux client, so it is always a manual step.
package vpn

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/KushalNaral/sshdeck/internal/store"
)

type State string

const (
	Down       State = "down"
	Connecting State = "connecting"
	Up         State = "up"
	Failed     State = "failed"
)

// ErrManual is returned by Up for VPNs the app cannot start itself (Hillstone).
var ErrManual = errors.New("vpn: this VPN must be connected manually")

// Proc is a running client process.
type Proc interface {
	// Lines streams the client's output, one line per element, closed when the process ends.
	Lines() <-chan string
	// Wait blocks until the process exits and returns its exit error.
	Wait() error
	// Stop asks the process to exit (SIGTERM, then kill) and waits for it.
	Stop() error
}

// Starter launches a process. The real one wraps os/exec; tests use a fake.
// stdin is written to the process once and then closed, so secrets never appear in argv.
type Starter interface {
	Start(ctx context.Context, argv []string, stdin string) (Proc, error)
}

// Manager runs at most one client per VPN profile.
type Manager interface {
	// Up starts the client and returns once the tunnel is up. If the client exits first, or ctx ends,
	// it returns an error that carries the client's last output line, and the state is Failed.
	Up(ctx context.Context, v store.VPN, secret string) error
	// Down stops the client for this profile. Down on a VPN that is not running is not an error.
	Down(id string) error
	// Status is Down for a VPN that was never started.
	Status(id string) State
}

// New returns a Manager that launches clients through s.
func New(s Starter) Manager { return &manager{s: s, m: map[string]*entry{}} }

type entry struct {
	p     Proc
	state State
}

type manager struct {
	s  Starter
	mu sync.Mutex
	m  map[string]*entry
}

func (m *manager) setState(id string, e *entry, st State) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m[id] == e { // a newer Up for the same profile owns the state now
		e.state = st
	}
}

// CertError means the gateway's certificate was not trusted. Digest is what to put in the VPN profile
// ("Trusted certificate") if the user trusts this gateway.
type CertError struct{ Digest string }

func (e *CertError) Error() string {
	return "the gateway's certificate is not trusted (sha256 " + e.Digest + ")"
}

var (
	certRe = regexp.MustCompile(`--trusted-cert\s+([0-9a-fA-F]{64})`)
	hexRe  = regexp.MustCompile(`\b([0-9a-fA-F]{64})\b`)
)

// runtimeDir is where the short-lived config file goes: the per-user tmpfs when there is one.
func runtimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}

// command returns the client's argv, what to write to its stdin, the output line that means "the tunnel is up",
// and a cleanup to run when the attempt is over. openfortivpn does not take a password on stdin, so it gets a
// 0600 config file that is deleted as soon as the attempt ends.
func command(v store.VPN, secret string) (argv []string, stdin, ready string, cleanup func(), err error) {
	cleanup = func() {}
	gw := fmt.Sprintf("%s:%d", v.Gateway, v.Port)
	switch v.Kind {
	case store.Forti:
		f, err := os.CreateTemp(runtimeDir(), "sshdeck-vpn-*.conf")
		if err != nil {
			return nil, "", "", nil, err
		}
		cleanup = func() { os.Remove(f.Name()) }
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			cleanup()
			return nil, "", "", nil, err
		}
		conf := fmt.Sprintf("host = %s\nport = %d\nusername = %s\npassword = %s\n", v.Gateway, v.Port, v.User, secret)
		if v.TrustedCert != "" {
			conf += "trusted-cert = " + v.TrustedCert + "\n"
		}
		_, werr := f.WriteString(conf)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			cleanup()
			return nil, "", "", nil, werr
		}
		return []string{"pkexec", "openfortivpn", "-c", f.Name()}, "", "Tunnel is up", cleanup, nil
	case store.GP:
		return []string{"pkexec", "openconnect", "--protocol=gp", "--user", v.User, "--passwd-on-stdin", gw}, secret + "\n", "Connected as", cleanup, nil
	case store.Hillstone:
		return nil, "", "", cleanup, ErrManual
	}
	return nil, "", "", cleanup, fmt.Errorf("vpn: unknown client %q", v.Kind)
}

// explain turns the client's last output lines into one error: its ERROR lines, or else the last line.
func explain(lines []string) error {
	all := strings.Join(lines, "\n")
	if m := certRe.FindStringSubmatch(all); m != nil {
		return &CertError{Digest: m[1]}
	}
	// Other versions print "sha256 digest:" and then the hash, with no --trusted-cert hint.
	if strings.Contains(strings.ToLower(all), "certificate") {
		if m := hexRe.FindStringSubmatch(all); m != nil {
			return &CertError{Digest: strings.ToLower(m[1])}
		}
	}
	var errs []string
	for _, l := range lines {
		if strings.Contains(strings.ToUpper(l), "ERROR") {
			errs = append(errs, strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "ERROR:")))
		}
	}
	if len(errs) == 0 && len(lines) > 0 {
		errs = lines[len(lines)-1:]
	}
	return errors.New(strings.Join(errs, "; "))
}

func (m *manager) Up(ctx context.Context, v store.VPN, secret string) error {
	argv, stdin, ready, cleanup, err := command(v, secret)
	defer cleanup()
	if err != nil {
		return err
	}
	e := &entry{state: Connecting}
	m.mu.Lock()
	if cur := m.m[v.ID]; cur != nil {
		switch cur.state {
		case Up:
			m.mu.Unlock()
			return nil
		case Connecting:
			m.mu.Unlock()
			return fmt.Errorf("vpn %s is already connecting", v.Name)
		}
	}
	m.m[v.ID] = e
	m.mu.Unlock()

	p, err := m.s.Start(ctx, argv, stdin)
	if err != nil {
		m.setState(v.ID, e, Failed)
		return fmt.Errorf("start %s: %w", argv[1], err)
	}
	m.mu.Lock()
	e.p = p
	m.mu.Unlock()

	var tail []string // the client's recent output, for the error message
	for {
		select {
		case line, ok := <-p.Lines():
			if !ok {
				m.setState(v.ID, e, Failed)
				if len(tail) == 0 {
					return fmt.Errorf("%s exited before the tunnel came up, without saying why", argv[1])
				}
				return fmt.Errorf("%s could not connect: %w", argv[1], explain(tail))
			}
			if line = strings.TrimSpace(line); line != "" {
				if tail = append(tail, line); len(tail) > 12 {
					tail = tail[1:]
				}
			}
			if strings.Contains(line, ready) {
				m.setState(v.ID, e, Up)
				go func() { // keep draining so the client never blocks on a full pipe; note when it dies
					for range p.Lines() {
					}
					m.mu.Lock()
					if m.m[v.ID] == e && e.state == Up {
						e.state = Down
					}
					m.mu.Unlock()
				}()
				return nil
			}
		case <-ctx.Done():
			_ = p.Stop()
			m.setState(v.ID, e, Failed)
			last := ""
			if len(tail) > 0 {
				last = tail[len(tail)-1]
			}
			return fmt.Errorf("%s did not come up: %w (last output: %s)", argv[1], ctx.Err(), last)
		}
	}
}

func (m *manager) Down(id string) error {
	m.mu.Lock()
	e := m.m[id]
	m.mu.Unlock()
	if e == nil || e.p == nil {
		return nil
	}
	err := e.p.Stop()
	m.setState(id, e, Down)
	return err
}

func (m *manager) Status(id string) State {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.m[id]; e != nil {
		return e.state
	}
	return Down
}
