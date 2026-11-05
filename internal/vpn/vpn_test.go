package vpn

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	if f.argv[0] != "/usr/bin/pkexec" || !slices.Contains(f.argv, "/usr/bin/openfortivpn") || !slices.Contains(f.argv, "-c") {
		t.Fatalf("argv %v", f.argv)
	}
	for _, want := range []string{"host = 203.0.113.50", "port = 10443", "username = info"} {
		if !strings.Contains(f.conf, want) {
			t.Errorf("config is missing %q:\n%s", want, strings.ReplaceAll(f.conf, "hunter2", "***"))
		}
	}
}

func TestPasswordNeverInArgv(t *testing.T) {
	withClients(t, "openconnect", "openfortivpn")
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

// Unless a test says otherwise, every client is "installed" in a root-owned place.
func init() {
	lookPath = func(n string) (string, error) { return "/usr/bin/" + n, nil }
	rootOwned = func(string) error { return nil }
	tunWait = 20 * time.Millisecond
}

// withClients pretends only the named programs (and pkexec) are installed.
func withClients(t *testing.T, have ...string) {
	t.Helper()
	old := lookPath
	lookPath = func(n string) (string, error) {
		if n == "pkexec" || slices.Contains(have, n) {
			return "/usr/bin/" + n, nil
		}
		return "", errors.New("not found")
	}
	t.Cleanup(func() { lookPath = old })
}

func TestGlobalProtectPasswordGoesOnStdin(t *testing.T) {
	withClients(t, "openconnect")
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
	withClients(t, "openconnect")
	f := &fakeStarter{script: printThenStay("Connected as 10.0.0.2, using SSL")}
	if err := New(f).Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/usr/bin/openconnect", "--protocol=gp", "connect.example.gov", "support"} {
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

func TestGlobalProtectPrefersGpclientAndIsUpWhenItExitsCleanly(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	f := &fakeStarter{script: printThenExit(nil, "Sign-in complete")}
	m := New(f)
	if err := m.Up(ctx(t), gp, "hunter2"); err != nil {
		t.Fatal(err)
	}
	if m.Status("gov") != Up {
		t.Fatalf("status %v", m.Status("gov"))
	}
	want := []string{"/usr/bin/pkexec", "/usr/bin/gpclient", "--fix-openssl", "connect", "connect.example.gov", "-u", "support", "--auto-gateway", "--passwd-on-stdin", "--background"}
	if !slices.Equal(f.argv, want) {
		t.Fatalf("argv %v\nwant %v", f.argv, want)
	}
	if strings.Contains(strings.Join(f.argv, " "), "hunter2") || f.stdin != "hunter2\n" {
		t.Fatalf("password must be on stdin only: argv %v stdin %q", f.argv, f.stdin)
	}
}

func TestGpclientNonStandardPortIsPartOfTheServer(t *testing.T) {
	withClients(t, "gpclient")
	f := &fakeStarter{script: printThenExit(nil)}
	v := gp
	v.Port = 8443
	if err := New(f).Up(ctx(t), v, "pw"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(f.argv, "connect.example.gov:8443") {
		t.Fatalf("argv %v", f.argv)
	}
}

func TestGpclientFailureShowsItsOutput(t *testing.T) {
	withClients(t, "gpclient")
	f := &fakeStarter{script: printThenExit(errors.New("exit status 1"), "INFO  portal login", "ERROR Gateway auth failed: bad password")}
	m := New(f)
	err := m.Up(ctx(t), gp, "pw")
	if err == nil || !strings.Contains(err.Error(), "Gateway auth failed") {
		t.Fatalf("err %v", err)
	}
	if m.Status("gov") != Failed {
		t.Fatalf("status %v", m.Status("gov"))
	}
}

func TestGpclientDownRunsDisconnect(t *testing.T) {
	withClients(t, "gpclient")
	f := &fakeStarter{script: printThenExit(nil)}
	m := New(f)
	if err := m.Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	if err := m.Down("gov"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"/usr/bin/pkexec", "/usr/bin/gpclient", "disconnect"}; !slices.Equal(f.argv, want) {
		t.Fatalf("down argv %v, want %v", f.argv, want)
	}
	if m.Status("gov") != Down {
		t.Fatalf("status %v", m.Status("gov"))
	}
}

func TestToolPicksGpclientWhenInstalled(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	if got := Tool(store.GP); got != "gpclient" {
		t.Fatal(got)
	}
	withClients(t, "openconnect")
	if got := Tool(store.GP); got != "openconnect" {
		t.Fatal(got)
	}
	if got := Tool(store.Forti); got != "openfortivpn" {
		t.Fatal(got)
	}
}

func TestNothingStartsWhenAFieldCouldInjectIntoTheConfig(t *testing.T) {
	for name, v := range map[string]store.VPN{
		"user newline":    {ID: "x", Name: "X", Kind: store.Forti, Gateway: "gw", Port: 443, User: "u\npppd-plugin = x"},
		"gateway newline": {ID: "x", Name: "X", Kind: store.Forti, Gateway: "gw\nx", Port: 443, User: "u"},
		"gateway dash":    {ID: "x", Name: "X", Kind: store.Forti, Gateway: "--script=/x", Port: 443, User: "u"},
	} {
		f := &fakeStarter{script: printThenStay("Tunnel is up")}
		if err := New(f).Up(ctx(t), v, "pw"); err == nil || f.n != 0 {
			t.Errorf("%s: err=%v starts=%d", name, err, f.n)
		}
	}
	f := &fakeStarter{script: printThenStay("Tunnel is up")}
	if err := New(f).Up(ctx(t), forti, "pass\nword"); err == nil || f.n != 0 {
		t.Errorf("password newline: err=%v starts=%d", err, f.n)
	}
}

func TestAClientAUserCouldReplaceIsNeverRunAsRoot(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	old := rootOwned
	rootOwned = func(p string) error {
		if strings.HasSuffix(p, "openfortivpn") {
			return errors.New("the directory /home/me/bin is writable by its owner")
		}
		return nil
	}
	t.Cleanup(func() { rootOwned = old })
	f := &fakeStarter{script: printThenStay("Tunnel is up")}
	err := New(f).Up(ctx(t), forti, "pw")
	if err == nil || !strings.Contains(err.Error(), "refusing to run") || f.n != 0 {
		t.Fatalf("err=%v starts=%d", err, f.n)
	}
	if _, serr := os.ReadDir(runtimeDir()); serr == nil {
		ents, _ := os.ReadDir(runtimeDir())
		for _, e := range ents {
			if strings.HasPrefix(e.Name(), "sshdeck-vpn-") {
				t.Errorf("a config file with the password was left behind: %s", e.Name())
			}
		}
	}
}

func TestRootOwnedCheckOnRealFiles(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: everything here is root-owned")
	}
	f, err := os.CreateTemp(t.TempDir(), "tool")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := checkRootOwned(f.Name()); err == nil {
		t.Error("a file owned by an ordinary user was accepted for root")
	}
	if _, err := os.Stat("/usr/bin/env"); err == nil {
		if err := checkRootOwned("/usr/bin/env"); err != nil {
			t.Errorf("/usr/bin/env should be fine: %v", err)
		}
	}
}

func TestPasswordNeverAppearsInAnErrorFromTheClient(t *testing.T) {
	withClients(t, "openconnect")
	f := &fakeStarter{script: printThenExit(errors.New("exit status 1"), "ERROR: Failed to complete authentication")}
	err := New(f).Up(ctx(t), gp, "hunter2-secret")
	if err == nil || strings.Contains(err.Error(), "hunter2-secret") {
		t.Fatalf("%v", err)
	}
}

func TestSweepRemovesOnlyStaleConfigFiles(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string, age time.Duration) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("password = x\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-age)
		os.Chtimes(p, old, old)
		return p
	}
	stale := mk("sshdeck-vpn-111.conf", time.Hour)
	fresh := mk("sshdeck-vpn-222.conf", time.Second) // may belong to a connect in progress
	other := mk("something-else.conf", time.Hour)
	if n := SweepStale(dir, time.Minute); n != 1 {
		t.Fatalf("removed %d files", n)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Error("stale config survived")
	}
	for _, keep := range []string{fresh, other} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s must be left alone", keep)
		}
	}
}

// fakeNet is the machine's interface list for a test: add and remove interfaces while the VPN manager watches.
type fakeNet struct {
	mu sync.Mutex
	m  map[string]bool // name -> up
}

func (n *fakeNet) set(name string, up bool) { n.mu.Lock(); n.m[name] = up; n.mu.Unlock() }
func (n *fakeNet) remove(name string)       { n.mu.Lock(); delete(n.m, name); n.mu.Unlock() }
func (n *fakeNet) list() []iface {
	n.mu.Lock()
	defer n.mu.Unlock()
	var out []iface
	for k, v := range n.m {
		out = append(out, iface{k, v})
	}
	return out
}

func withNet(t *testing.T, names ...string) *fakeNet {
	t.Helper()
	n := &fakeNet{m: map[string]bool{}}
	for _, name := range names {
		n.m[name] = true
	}
	old := listInterfaces
	listInterfaces = n.list
	t.Cleanup(func() { listInterfaces = old })
	return n
}

// bringsUpTunnel is a background client that creates the interface and then exits cleanly.
func bringsUpTunnel(n *fakeNet, name string) func(*fakeProc) {
	return func(p *fakeProc) {
		n.set(name, true)
		printThenExit(nil, "Sign-in complete")(p)
	}
}

func TestBackgroundTunnelIsWatchedByItsInterface(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	n := withNet(t, "lo", "wlp0s20f3", "docker0")
	m := New(&fakeStarter{script: bringsUpTunnel(n, "tun0")})
	if err := m.Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	if m.Status("gov") != Up {
		t.Fatalf("status %v, want up while tun0 exists", m.Status("gov"))
	}
	n.remove("tun0")
	if m.Status("gov") != Down {
		t.Errorf("status %v, want down once tun0 is gone", m.Status("gov"))
	}
}

func TestTunnelFlaggedDownCountsAsDown(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	n := withNet(t, "lo")
	m := New(&fakeStarter{script: bringsUpTunnel(n, "tun0")})
	if err := m.Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	if m.Status("gov") != Up {
		t.Fatal("an interface that is up counts as up")
	}
	n.set("tun0", false)
	if m.Status("gov") != Down {
		t.Error("an interface flagged down must count as down")
	}
}

func TestOnlyANewInterfaceIsTheTunnelAndNoneMeansTrustTheClient(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	withNet(t, "lo", "tun0") // tun0 already existed: another VPN's, not ours
	m := New(&fakeStarter{script: printThenExit(nil, "Sign-in complete")})
	if err := m.Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	if m.Status("gov") != Up {
		t.Errorf("status %v: with no new interface there is nothing to watch, so keep trusting the client", m.Status("gov"))
	}
}

func TestDockerBridgeAppearingIsNotTheTunnel(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	n := withNet(t, "lo")
	m := New(&fakeStarter{script: func(p *fakeProc) {
		n.set("br-abc123", true)
		n.set("tun1", true)
		printThenExit(nil, "Sign-in complete")(p)
	}})
	if err := m.Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	n.remove("br-abc123") // a bridge coming and going changes nothing
	if m.Status("gov") != Up {
		t.Error("the bridge must not be mistaken for the tunnel")
	}
	n.remove("tun1")
	if m.Status("gov") != Down {
		t.Error("tun1 is the tunnel")
	}
}

// seqStarter runs a different script for each process it starts, and remembers the commands.
type seqStarter struct {
	scripts []func(*fakeProc)
	argvs   [][]string
}

func (s *seqStarter) Start(_ context.Context, argv []string, _ string) (Proc, error) {
	i := len(s.argvs)
	s.argvs = append(s.argvs, argv)
	p := &fakeProc{lines: make(chan string, 8), done: make(chan struct{})}
	if i >= len(s.scripts) {
		i = len(s.scripts) - 1
	}
	go s.scripts[i](p)
	return p, nil
}

func TestStaleGpclientIsDisconnectedAndConnectRetriedOnce(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	withNet(t, "lo")
	s := &seqStarter{scripts: []func(*fakeProc){
		printThenExit(errors.New("exit status 1"), "Error: Another instance of the client is already running"),
		printThenExit(nil, "disconnected"),
		printThenExit(nil, "Sign-in complete"),
	}}
	m := New(s)
	if err := m.Up(ctx(t), gp, "pw"); err != nil {
		t.Fatal(err)
	}
	if len(s.argvs) != 3 || !slices.Contains(s.argvs[1], "disconnect") || !slices.Contains(s.argvs[2], "connect") {
		t.Fatalf("commands = %v, want connect, disconnect, connect", s.argvs)
	}
	if m.Status("gov") != Up {
		t.Errorf("status %v", m.Status("gov"))
	}
}

func TestStaleGpclientIsOnlyRetriedOnce(t *testing.T) {
	withClients(t, "gpclient", "openconnect")
	withNet(t, "lo")
	stale := printThenExit(errors.New("exit status 1"), "Error: Another instance of the client is already running")
	s := &seqStarter{scripts: []func(*fakeProc){stale, printThenExit(nil), stale}}
	err := New(s).Up(ctx(t), gp, "pw")
	if err == nil || !strings.Contains(err.Error(), "Another instance") {
		t.Fatalf("err = %v, want the client's message after one retry", err)
	}
	if len(s.argvs) != 3 {
		t.Errorf("commands = %d, want 3 (no endless loop)", len(s.argvs))
	}
}

func TestAutomaticDependsOnTheClientAndTheSystem(t *testing.T) {
	if Automatic(store.Hillstone) {
		t.Error("Hillstone has no client: always manual")
	}
	if !Automatic(store.GP) || !Automatic(store.Forti) {
		t.Error("GP and Forti are automatic on Linux")
	}
}

// asWindows makes command() build Windows-style commands: openconnect for both kinds, no pkexec.
func asWindows(t *testing.T) {
	t.Helper()
	old, oldLook := goos, lookPath
	goos = "windows"
	lookPath = func(n string) (string, error) { return "/Program Files/OpenConnect/" + n + ".exe", nil }
	t.Cleanup(func() { goos, lookPath = old, oldLook })
}

func TestWindowsGlobalProtectUsesOpenconnectDirectly(t *testing.T) {
	asWindows(t)
	f := &fakeStarter{script: printThenStay("Connected as 10.0.0.2, using SSL")}
	if err := New(f).Up(ctx(t), gp, "hunter2"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/Program Files/OpenConnect/openconnect.exe", "--protocol=gp", "--user", "support", "--passwd-on-stdin", "connect.example.gov"}
	if !slices.Equal(f.argv, want) {
		t.Fatalf("argv %v\nwant %v", f.argv, want)
	}
	if f.stdin != "hunter2\n" || strings.Contains(strings.Join(f.argv, " "), "hunter2") {
		t.Errorf("the password belongs on stdin only: argv %v stdin %q", f.argv, f.stdin)
	}
	if err := validateOpenconnectArgs(f.argv[1:]); err != nil {
		t.Errorf("the helper would refuse our own command: %v", err)
	}
}

func TestWindowsFortinetUsesOpenconnectToo(t *testing.T) {
	asWindows(t)
	f := &fakeStarter{script: printThenStay("Connected as 10.1.1.9, using SSL")}
	if err := New(f).Up(ctx(t), forti, "pw"); err != nil {
		t.Fatal(err)
	}
	if f.argv[1] != "--protocol=fortinet" || f.argv[len(f.argv)-1] != "203.0.113.50:10443" {
		t.Fatalf("argv %v", f.argv)
	}
	if err := validateOpenconnectArgs(f.argv[1:]); err != nil {
		t.Errorf("the helper would refuse our own command: %v", err)
	}
}

func TestWindowsCertificatePinsAreNotMangled(t *testing.T) {
	asWindows(t)
	pin := "pin-sha256:" + strings.Repeat("A", 43) + "="
	hex := strings.Repeat("ab", 32)
	for _, c := range []struct{ stored, want string }{{pin, pin}, {hex, "sha256:" + hex}} {
		v := gp
		v.TrustedCert = c.stored
		f := &fakeStarter{script: printThenStay("Connected as 10.0.0.2")}
		if err := New(f).Up(ctx(t), v, "pw"); err != nil {
			t.Fatal(err)
		}
		i := slices.Index(f.argv, "--servercert")
		if i < 0 || f.argv[i+1] != c.want {
			t.Errorf("stored %q: argv %v, want --servercert %s", c.stored, f.argv, c.want)
		}
		if err := validateOpenconnectArgs(f.argv[1:]); err != nil {
			t.Errorf("the helper would refuse our own command: %v", err)
		}
	}
}

func TestOpenconnectCertificateHintGivesThePin(t *testing.T) {
	pin := "pin-sha256:" + strings.Repeat("B", 43) + "="
	f := &fakeStarter{script: printThenExit(errors.New("exit status 1"),
		"Certificate from VPN server \"vpn.example.org\" failed verification.",
		"To trust this server in future, perhaps add this to your command line:",
		"    --servercert "+pin)}
	asWindows(t)
	err := New(f).Up(ctx(t), gp, "pw")
	var ce *CertError
	if !errors.As(err, &ce) || ce.Digest != pin {
		t.Fatalf("want a CertError with the pin, got %v", err)
	}
}

func TestWindowsVPNsAreAutomaticExceptHillstone(t *testing.T) {
	asWindows(t)
	if !Automatic(store.GP) || !Automatic(store.Forti) || Automatic(store.Hillstone) {
		t.Error("Automatic on Windows: GP and Forti yes, Hillstone no")
	}
	if Tool(store.Forti) != "openconnect" || Tool(store.GP) != "openconnect" {
		t.Error("openconnect drives both kinds on Windows")
	}
}

func TestOpenfortivpnRefusesAPinFormat(t *testing.T) {
	withClients(t, "openfortivpn")
	v := forti
	v.TrustedCert = "pin-sha256:" + strings.Repeat("A", 43) + "="
	err := New(&fakeStarter{script: printThenStay("Tunnel is up")}).Up(ctx(t), v, "pw")
	if err == nil || !strings.Contains(err.Error(), "digest") {
		t.Errorf("err = %v: openfortivpn needs the sha256 digest, not an openconnect pin", err)
	}
}
