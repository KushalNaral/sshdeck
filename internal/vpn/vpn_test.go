package vpn

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// fakeProc plays back lines, then (optionally) exits on its own.
type fakeProc struct {
	lines   chan string
	done    chan struct{}
	stopped bool
	mu      sync.Mutex
	exit    error
}

func (p *fakeProc) Lines() <-chan string { return p.lines }
func (p *fakeProc) Wait() error          { <-p.done; return p.exit }
func (p *fakeProc) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stopped {
		p.stopped = true
		close(p.done)
		close(p.lines)
	}
	return nil
}
func (p *fakeProc) wasStopped() bool { p.mu.Lock(); defer p.mu.Unlock(); return p.stopped }

type fakeStarter struct {
	argv     []string
	conf     string // the -c config file as it was when the client started
	confMode os.FileMode
	stdin    string
	proc     *fakeProc
	n        int
	script   func(p *fakeProc) // what the client prints
}

func (f *fakeStarter) Start(_ context.Context, argv []string, stdin string) (Proc, error) {
	f.argv, f.stdin, f.n = argv, stdin, f.n+1
	if i := slices.Index(argv, "-c"); i >= 0 {
		b, _ := os.ReadFile(argv[i+1])
		f.conf = string(b)
		if st, err := os.Stat(argv[i+1]); err == nil {
			f.confMode = st.Mode().Perm()
		}
	}
	f.proc = &fakeProc{lines: make(chan string, 8), done: make(chan struct{})}
	go f.script(f.proc)
	return f.proc, nil
}

func printThenStay(lines ...string) func(*fakeProc) {
	return func(p *fakeProc) {
		for _, l := range lines {
			p.lines <- l
		}
	}
}

// printThenExit makes the client print lines and die with err before the tunnel is ever up.
func printThenExit(err error, lines ...string) func(*fakeProc) {
	return func(p *fakeProc) {
		for _, l := range lines {
			p.lines <- l
		}
		p.exit = err
		p.mu.Lock()
		if !p.stopped {
			p.stopped = true
			close(p.done)
			close(p.lines)
		}
		p.mu.Unlock()
	}
}

func ctx(t *testing.T) context.Context {
	c, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	return c
}

var forti = store.VPN{ID: "pine", Name: "HQ", Kind: store.Forti, Gateway: "203.0.113.50", Port: 10443, User: "info"}
var gp = store.VPN{ID: "gov", Name: "Gov", Kind: store.GP, Gateway: "connect.example.gov", Port: 443, User: "support"}

func TestFortiUpWaitsForTheTunnel(t *testing.T) {
	f := &fakeStarter{script: printThenStay("INFO: Connected to gateway.", "INFO: Tunnel is up and running.")}
	m := New(f)
	if err := m.Up(ctx(t), forti, "hunter2"); err != nil {
		t.Fatal(err)
	}
	if m.Status("pine") != Up {
		t.Fatalf("status %v, want up", m.Status("pine"))
	}
	if f.argv[0] != "pkexec" || !slices.Contains(f.argv, "openfortivpn") || !slices.Contains(f.argv, "-c") {
		t.Fatalf("argv %v", f.argv)
	}
	for _, want := range []string{"host = 203.0.113.50", "port = 10443", "username = info"} {
		if !strings.Contains(f.conf, want) {
			t.Errorf("config is missing %q:\n%s", want, strings.ReplaceAll(f.conf, "hunter2", "***"))
		}
	}
}

func TestPasswordNeverInArgv(t *testing.T) {
	for _, v := range []store.VPN{forti, gp} {
		f := &fakeStarter{script: printThenStay("Tunnel is up and running.", "Connected as 10.0.0.2")}
		if err := New(f).Up(ctx(t), v, "hunter2"); err != nil {
			t.Fatalf("%s: %v", v.Kind, err)
		}
		if strings.Contains(strings.Join(f.argv, " "), "hunter2") {
			t.Errorf("%s: password found in argv %v", v.Kind, f.argv)
		}
	}
}

func TestFortiPasswordGoesInAPrivateFileThatIsRemoved(t *testing.T) {
	f := &fakeStarter{script: printThenStay("Tunnel is up and running.")}
	if err := New(f).Up(ctx(t), forti, "hunter2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.conf, "password = hunter2") {
		t.Error("the password must reach openfortivpn through its config file")
	}
	if f.confMode != 0o600 {
		t.Errorf("config file mode %v, want 0600", f.confMode)
	}
	if _, err := os.Stat(f.argv[slices.Index(f.argv, "-c")+1]); err == nil {
		t.Error("the config file with the password was left on disk")
	}
}

func TestGlobalProtectPasswordGoesOnStdin(t *testing.T) {
	f := &fakeStarter{script: printThenStay("Connected as 10.0.0.2")}
	if err := New(f).Up(ctx(t), gp, "hunter2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.stdin, "hunter2") {
		t.Errorf("password not on stdin (%q)", f.stdin)
	}
}

func TestFortiTrustedCertGoesInTheConfig(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	v := forti
	v.TrustedCert = digest
	f := &fakeStarter{script: printThenStay("Tunnel is up and running.")}
	if err := New(f).Up(ctx(t), v, "pw"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.conf, "trusted-cert = "+digest) {
		t.Errorf("config:\n%s", strings.ReplaceAll(f.conf, "pw", "***"))
	}
}

func TestTheRealErrorIsShownNotJustTheLastLine(t *testing.T) {
	f := &fakeStarter{script: printThenExit(errors.New("exit status 1"),
		"INFO: Connected to gateway.",
		"ERROR: Could not authenticate to gateway. Please check the username and password.",
		"INFO: Could not log out.")}
	err := New(f).Up(ctx(t), forti, "bad")
	if err == nil || !strings.Contains(err.Error(), "Could not authenticate") {
		t.Fatalf("the ERROR line must be in the message, got %v", err)
	}
}

func TestUntrustedCertificateGivesTheDigest(t *testing.T) {
	digest := strings.Repeat("0f", 32)
	f := &fakeStarter{script: printThenExit(errors.New("exit status 1"),
		"ERROR:  Gateway certificate validation failed, and the certificate digest in not in the local whitelist. If you trust it, rerun with:",
		"INFO:      --trusted-cert "+digest,
		"INFO: Could not log out.")}
	err := New(f).Up(ctx(t), forti, "pw")
	var ce *CertError
	if !errors.As(err, &ce) || ce.Digest != digest {
		t.Fatalf("want a CertError with the digest, got %v", err)
	}
}

func TestGlobalProtectUsesOpenconnect(t *testing.T) {
	f := &fakeStarter{script: printThenStay("Connected as 10.0.0.2, using SSL")}
	if err := New(f).Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"openconnect", "--protocol=gp", "connect.example.gov", "support"} {
		if !slices.Contains(f.argv, want) && !strings.Contains(strings.Join(f.argv, " "), want) {
			t.Errorf("argv %v is missing %q", f.argv, want)
		}
	}
	if !slices.Contains(f.argv, "--passwd-on-stdin") {
		t.Errorf("argv %v: openconnect needs --passwd-on-stdin to read the password from stdin", f.argv)
	}
}

func TestClientExitingEarlyIsAFailureWithItsLastLine(t *testing.T) {
	f := &fakeStarter{script: printThenExit(errors.New("exit status 1"), "ERROR: Could not authenticate to gateway.")}
	m := New(f)
	err := m.Up(ctx(t), forti, "bad")
	if err == nil || !strings.Contains(err.Error(), "Could not authenticate") {
		t.Fatalf("want the client's last line in the error, got %v", err)
	}
	if m.Status("pine") != Failed {
		t.Fatalf("status %v, want failed", m.Status("pine"))
	}
}

func TestCancelledContextStopsTheClient(t *testing.T) {
	f := &fakeStarter{script: printThenStay("INFO: Connecting...")} // never reaches "up"
	c, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	m := New(f)
	if err := m.Up(c, forti, "pw"); err == nil {
		t.Fatal("want an error when ctx ends before the tunnel is up")
	}
	if !f.proc.wasStopped() {
		t.Error("the client process was left running")
	}
	if m.Status("pine") != Failed {
		t.Errorf("status %v, want failed", m.Status("pine"))
	}
}

func TestHillstoneIsManual(t *testing.T) {
	f := &fakeStarter{script: printThenStay()}
	err := New(f).Up(ctx(t), store.VPN{ID: "sable", Kind: store.Hillstone, Gateway: "x", Port: 1}, "pw")
	if !errors.Is(err, ErrManual) {
		t.Fatalf("got %v, want ErrManual", err)
	}
	if f.n != 0 {
		t.Error("no process should be started for a manual VPN")
	}
}

func TestDownStopsTheClient(t *testing.T) {
	f := &fakeStarter{script: printThenStay("Tunnel is up and running.")}
	m := New(f)
	if err := m.Up(ctx(t), forti, "pw"); err != nil {
		t.Fatal(err)
	}
	if err := m.Down("pine"); err != nil {
		t.Fatal(err)
	}
	if !f.proc.wasStopped() || m.Status("pine") != Down {
		t.Fatalf("stopped=%v status=%v", f.proc.wasStopped(), m.Status("pine"))
	}
	if err := m.Down("never-started"); err != nil {
		t.Errorf("Down on an unknown VPN: %v", err)
	}
}

func TestUpTwiceKeepsOneClient(t *testing.T) {
	f := &fakeStarter{script: printThenStay("Tunnel is up and running.")}
	m := New(f)
	for range 2 {
		if err := m.Up(ctx(t), forti, "pw"); err != nil {
			t.Fatal(err)
		}
	}
	if f.n != 1 {
		t.Fatalf("started %d clients, want 1", f.n)
	}
}

func TestStatusOfUnknownIsDown(t *testing.T) {
	if s := New(&fakeStarter{script: printThenStay()}).Status("nope"); s != Down {
		t.Fatalf("got %v", s)
	}
}

// The exact shape openfortivpn 1.21 prints when it rejects a certificate (digest on its own line, no hint).
func TestUntrustedCertificateWithoutTheHintLine(t *testing.T) {
	digest := "642e95c75bc31cded4a3048580a2e976dad45549d779a9b8bd2b1b2d22bcd627"
	f := &fakeStarter{script: printThenExit(errors.New("exit status 1"),
		"ERROR:  Gateway certificate validation failed.",
		"ERROR:  Gateway certificate subject: CN=support; emailAddress=support@fortinet.com",
		"ERROR:  Gateway certificate issuer: C=US; O=Fortinet; CN=support",
		"ERROR:  Gateway certificate sha256 digest:",
		"ERROR:  "+digest,
		"INFO:   Could not log out.")}
	err := New(f).Up(ctx(t), forti, "pw")
	var ce *CertError
	if !errors.As(err, &ce) || ce.Digest != digest {
		t.Fatalf("want a CertError with the digest, got %v", err)
	}
}
