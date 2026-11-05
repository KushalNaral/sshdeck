// Package vpn brings a VPN up and down by driving the Linux command-line clients:
// openfortivpn for FortiClient and openconnect (--protocol=gp) for GlobalProtect.
// Hillstone has no Linux client, so it is always a manual step.
package vpn

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

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
	stop  []string // for clients that leave a background daemon: the command that ends it
	tun   string   // for a background client: the network interface its tunnel created, so Status can tell when it dies
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
	pinRe  = regexp.MustCompile(`--servercert\s+(pin-sha256:[A-Za-z0-9+/]{43}=)`) // what openconnect suggests
	hexRe  = regexp.MustCompile(`\b([0-9a-fA-F]{64})\b`)
)

// runtimeDir is where the short-lived config file goes: the per-user tmpfs when there is one.
func runtimeDir() string {
	if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" {
		return d
	}
	return os.TempDir()
}

// lookPath and rootOwned are swapped in tests.
var (
	lookPath  = defaultLookPath
	rootOwned = checkRootOwned
	goos      = runtime.GOOS // a variable so tests can build Windows commands on Linux
)

// SweepStale deletes config files left by an earlier run that was killed mid-connect (they hold a VPN password).
// Files younger than olderThan are kept, since a connect may be using them. It returns how many it removed.
func SweepStale(dir string, olderThan time.Duration) int {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range ents {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "sshdeck-vpn-") || !strings.HasSuffix(e.Name(), ".conf") {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) >= olderThan && os.Remove(filepath.Join(dir, e.Name())) == nil {
			n++
		}
	}
	return n
}

// StaleDir is where the config files live; app startup sweeps it.
func StaleDir() string { return runtimeDir() }

// resolve returns the absolute path of a program that will run as root.
func resolve(name string) (string, error) {
	p, err := lookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s is not installed", name)
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("refusing to run %s as root: it was found by a relative path (%s)", name, p)
	}
	if err := rootOwned(p); err != nil {
		return "", fmt.Errorf("refusing to run %s as root: %w", p, err)
	}
	return p, nil
}

// LookPath finds a VPN client the way sshdeck itself will run it.
func LookPath(file string) (string, error) { return lookPath(file) }

// Automatic reports whether sshdeck can start and stop this kind of VPN itself on this system. Where it cannot,
// the connect flow waits for you to connect it by hand, as for Hillstone.
func Automatic(k store.VPNKind) bool {
	return k == store.Forti || k == store.GP
}

// Tool is the program that connects a VPN of this kind. GlobalProtect prefers gpclient (portal and gateway
// sign-in, which plain openconnect fails on some gateways) and falls back to openconnect.
func Tool(k store.VPNKind) string {
	if goos == "windows" && (k == store.Forti || k == store.GP) {
		return "openconnect" // the only client with Windows builds; it speaks both protocols
	}
	switch k {
	case store.Forti:
		return "openfortivpn"
	case store.GP:
		if _, err := lookPath("gpclient"); err == nil {
			return "gpclient"
		}
		return "openconnect"
	}
	return ""
}

// command returns the client's argv, what to write to its stdin, the output line that means "the tunnel is up",
// and a cleanup to run when the attempt is over. openfortivpn does not take a password on stdin, so it gets a
// 0600 config file that is deleted as soon as the attempt ends.
func command(v store.VPN, secret string) (argv []string, stdin, ready string, stop []string, cleanup func(), err error) {
	cleanup = func() {}
	if err := store.CheckVPN(v, secret); err != nil {
		return nil, "", "", nil, cleanup, err
	}
	gw := fmt.Sprintf("%s:%d", v.Gateway, v.Port)
	if v.Kind == store.Hillstone {
		return nil, "", "", nil, cleanup, ErrManual
	}
	if goos == "windows" {
		return windowsCommand(v, secret)
	}
	pk, err := resolve("pkexec")
	if err != nil {
		return nil, "", "", nil, cleanup, err
	}
	var tool string
	if v.Kind == store.Forti || v.Kind == store.GP {
		if tool, err = resolve(Tool(v.Kind)); err != nil {
			return nil, "", "", nil, cleanup, err
		}
	}
	switch v.Kind {
	case store.Forti:
		if strings.HasPrefix(v.TrustedCert, "pin-sha256:") {
			return nil, "", "", nil, cleanup, errors.New("openfortivpn needs the certificate's sha256 digest (64 hex digits), not an openconnect pin: edit the VPN profile")
		}
		f, err := os.CreateTemp(runtimeDir(), "sshdeck-vpn-*.conf")
		if err != nil {
			return nil, "", "", nil, nil, err
		}
		cleanup = func() { os.Remove(f.Name()) }
		if err := f.Chmod(0o600); err != nil {
			f.Close()
			cleanup()
			return nil, "", "", nil, nil, err
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
			return nil, "", "", nil, nil, werr
		}
		return []string{pk, tool, "-c", f.Name()}, "", "Tunnel is up", nil, cleanup, nil
	case store.GP:
		if Tool(store.GP) == "gpclient" {
			// --background: gpclient signs in, then exits 0 and leaves the tunnel running, so no ready line is needed.
			server := v.Gateway
			if v.Port != 443 {
				server = gw
			}
			argv := []string{pk, tool, "--fix-openssl", "connect", server, "-u", v.User, "--auto-gateway", "--passwd-on-stdin", "--background"}
			return argv, secret + "\n", "", []string{pk, tool, "disconnect"}, cleanup, nil
		}
		return []string{pk, tool, "--protocol=gp", "--user", v.User, "--passwd-on-stdin", gw}, secret + "\n", "Connected as", nil, cleanup, nil
	case store.Hillstone:
		return nil, "", "", nil, cleanup, ErrManual
	}
	return nil, "", "", nil, cleanup, fmt.Errorf("vpn: unknown client %q", v.Kind)
}

// windowsCommand runs openconnect for both GlobalProtect and Fortinet. It is started by the elevated helper, so there
// is no pkexec; the helper checks the command again before running it. The password goes on stdin.
func windowsCommand(v store.VPN, secret string) (argv []string, stdin, ready string, stop []string, cleanup func(), err error) {
	cleanup = func() {}
	tool, err := resolve("openconnect")
	if err != nil {
		return nil, "", "", nil, cleanup, err
	}
	proto := "--protocol=gp"
	if v.Kind == store.Forti {
		proto = "--protocol=fortinet"
	}
	server := v.Gateway
	if v.Port != 443 {
		server = fmt.Sprintf("%s:%d", v.Gateway, v.Port)
	}
	argv = []string{tool, proto, "--user", v.User, "--passwd-on-stdin"}
	if c := v.TrustedCert; c != "" {
		if !strings.HasPrefix(c, "pin-sha256:") {
			c = "sha256:" + c
		}
		argv = append(argv, "--servercert", c)
	}
	return append(argv, server), secret + "\n", "Connected as", nil, cleanup, nil
}

// explain turns the client's last output lines into one error: its ERROR lines, or else the last line.
func explain(lines []string) error {
	all := strings.Join(lines, "\n")
	if m := pinRe.FindStringSubmatch(all); m != nil {
		return &CertError{Digest: m[1]}
	}
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

// iface is one network interface. listInterfaces is a seam for tests; tunWait is how long to look for a new one.
type iface struct {
	Name string
	Up   bool
}

var (
	listInterfaces = func() []iface {
		var out []iface
		if all, err := net.Interfaces(); err == nil {
			for _, i := range all {
				out = append(out, iface{i.Name, i.Flags&net.FlagUp != 0})
			}
		}
		return out
	}
	tunWait = 3 * time.Second
)

func interfaces() map[string]bool {
	out := map[string]bool{}
	for _, i := range listInterfaces() {
		out[i.Name] = true
	}
	return out
}

// newTunnel finds the interface that appeared since before: a tunnel device by name, or the only new one.
// Bridges and the like that come and go (docker) are ignored. It returns "" when nothing suitable shows up.
func newTunnel(before map[string]bool, wait time.Duration) string {
	for deadline := time.Now().Add(wait); ; time.Sleep(50 * time.Millisecond) {
		var fresh []string
		for n := range interfaces() {
			if before[n] {
				continue
			}
			for _, p := range []string{"tun", "gpd", "ppp", "utun"} {
				if strings.HasPrefix(n, p) {
					return n
				}
			}
			fresh = append(fresh, n)
		}
		if len(fresh) == 1 && !strings.HasPrefix(fresh[0], "br-") && !strings.HasPrefix(fresh[0], "veth") && !strings.HasPrefix(fresh[0], "docker") {
			return fresh[0]
		}
		if time.Now().After(deadline) {
			return ""
		}
	}
}

// tunnelAlive is false once the interface is gone or down.
func tunnelAlive(name string) bool {
	for _, i := range listInterfaces() {
		if i.Name == name {
			return i.Up
		}
	}
	return false
}

// current is the entry's state, with a dead tunnel counted as Down. m.mu must be held.
func (e *entry) current() State {
	if e.state == Up && e.tun != "" && !tunnelAlive(e.tun) {
		e.state = Down
	}
	return e.state
}

// staleClient is what gpclient says while an earlier background instance (a tunnel that died, or one started by
// hand) is still running. That instance is stopped and the connect tried once more.
const staleClient = "Another instance of the client is already running"

func (m *manager) Up(ctx context.Context, v store.VPN, secret string) error {
	err := m.up(ctx, v, secret)
	if err == nil || !strings.Contains(err.Error(), staleClient) {
		return err
	}
	_, _, _, stop, cleanup, cerr := command(v, secret)
	cleanup()
	if cerr != nil || stop == nil {
		return err
	}
	_ = m.runStop(stop)
	return m.up(ctx, v, secret)
}

func (m *manager) up(ctx context.Context, v store.VPN, secret string) error {
	argv, stdin, ready, stop, cleanup, err := command(v, secret)
	defer cleanup()
	if err != nil {
		return err
	}
	e := &entry{state: Connecting, stop: stop}
	m.mu.Lock()
	if cur := m.m[v.ID]; cur != nil {
		switch cur.current() {
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

	var before map[string]bool
	if ready == "" { // a background client: its tunnel is found by the interface it adds
		before = interfaces()
	}
	p, err := m.s.Start(ctx, argv, stdin)
	if err != nil {
		m.setState(v.ID, e, Failed)
		return fmt.Errorf("start %s: %w", filepath.Base(argv[1]), err)
	}
	m.mu.Lock()
	e.p = p
	m.mu.Unlock()

	var tail []string // the client's recent output, for the error message
	for {
		select {
		case line, ok := <-p.Lines():
			if !ok {
				if ready == "" && p.Wait() == nil { // background client: a clean exit is "signed in, tunnel running"
					tun := newTunnel(before, tunWait)
					m.mu.Lock()
					e.tun = tun
					m.mu.Unlock()
					m.setState(v.ID, e, Up)
					return nil
				}
				m.setState(v.ID, e, Failed)
				if len(tail) == 0 {
					return fmt.Errorf("%s exited before the tunnel came up, without saying why", filepath.Base(argv[1]))
				}
				return fmt.Errorf("%s could not connect: %w", filepath.Base(argv[1]), explain(tail))
			}
			if line = strings.TrimSpace(line); line != "" {
				if tail = append(tail, line); len(tail) > 12 {
					tail = tail[1:]
				}
			}
			if ready != "" && strings.Contains(line, ready) {
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
			return fmt.Errorf("%s did not come up: %w (last output: %s)", filepath.Base(argv[1]), ctx.Err(), last)
		}
	}
}

// runStop runs the command that ends a background client (for gpclient, "disconnect") and waits for it.
func (m *manager) runStop(stop []string) error {
	p, err := m.s.Start(context.Background(), stop, "")
	if err != nil {
		return err
	}
	go func() {
		for range p.Lines() {
		}
	}()
	if err := p.Wait(); err != nil {
		return fmt.Errorf("%s: %w", strings.Join(stop[1:], " "), err)
	}
	return nil
}

func (m *manager) Down(id string) error {
	m.mu.Lock()
	e := m.m[id]
	m.mu.Unlock()
	if e == nil || e.p == nil {
		return nil
	}
	err := e.p.Stop()
	if e.stop != nil {
		if serr := m.runStop(e.stop); serr != nil {
			err = serr
		}
	}
	m.setState(id, e, Down)
	return err
}

func (m *manager) Status(id string) State {
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.m[id]; e != nil {
		return e.current()
	}
	return Down
}
