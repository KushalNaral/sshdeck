package main

import (
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/vpn"
)

type memSecrets struct{ m map[string]string }

func (s memSecrets) Set(k, v string) error { s.m[k] = v; return nil }
func (s memSecrets) Get(k string) (string, error) {
	if v, ok := s.m[k]; ok {
		return v, nil
	}
	return "", store.ErrNotFound
}
func (s memSecrets) Delete(k string) error { delete(s.m, k); return nil }

type fakeVPN struct {
	mu   sync.Mutex
	up   []string
	down []string
	err  error
	dead bool // Status reports Down until the next Up
}

func (f *fakeVPN) Up(_ context.Context, v store.VPN, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.up = append(f.up, v.ID)
	f.dead = false
	return f.err
}
func (f *fakeVPN) Down(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.down = append(f.down, id)
	return nil
}
func (f *fakeVPN) Status(string) vpn.State {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.dead {
		return vpn.Down
	}
	return vpn.Up
}

type fakeShell struct {
	out    chan []byte
	got    strings.Builder
	cols   int
	rows   int
	closed bool
	mu     sync.Mutex
}

func newShell() *fakeShell { return &fakeShell{out: make(chan []byte, 4)} }
func (f *fakeShell) Write(p []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got.Write(p)
	return nil
}
func (f *fakeShell) Resize(c, r int) error {
	f.mu.Lock()
	f.cols, f.rows = c, r
	f.mu.Unlock()
	return nil
}
func (f *fakeShell) Output() <-chan []byte { return f.out }
func (f *fakeShell) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.out)
	}
	return nil
}

type events struct {
	mu   sync.Mutex
	list []string
	last map[string]any
	all  map[string][]any
}

func (e *events) emit(name string, data ...any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, name)
	if e.last == nil {
		e.last = map[string]any{}
	}
	if len(data) > 0 {
		e.last[name] = data[0]
		if e.all == nil {
			e.all = map[string][]any{}
		}
		e.all[name] = append(e.all[name], data[0])
	}
}
func (e *events) get(name string) any {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.last[name]
}
func (e *events) has(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range e.list {
		if n == name {
			return true
		}
	}
	return false
}

// rig builds an App over a temp store with two direct servers, one on a shared Forti VPN pair and one Hillstone.
type openFn = func(context.Context, store.Server, string) (sshx.Session, error)

// fakeClient is one shared SSH connection. Shell calls the test's open function, so existing tests that script
// a shell keep working; Run records the command.
type fakeClient struct {
	mu     sync.Mutex
	open   openFn
	sv     store.Server
	pw     string
	done   chan struct{}
	closed bool
	dead   bool
	shells int
	runs   []string
}

func (f *fakeClient) Shell(ctx context.Context) (sshx.Session, error) {
	f.mu.Lock()
	f.shells++
	f.mu.Unlock()
	return f.open(ctx, f.sv, f.pw)
}
func (f *fakeClient) Run(_ context.Context, cmd string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs = append(f.runs, cmd)
	return "", nil
}
func (f *fakeClient) Done() <-chan struct{} { return f.done }
func (f *fakeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.dead {
		f.dead = true
		close(f.done)
	}
	f.closed = true
	return nil
}

// drop simulates the network ending the connection: Done fires, but nobody called Close.
func (f *fakeClient) drop() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.dead {
		f.dead = true
		close(f.done)
	}
}

type dialer struct {
	mu      sync.Mutex
	open    openFn
	clients []*fakeClient
	err     error
}

func (d *dialer) dial(_ context.Context, sv store.Server, pw string) (sshx.Client, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return nil, d.err
	}
	c := &fakeClient{open: d.open, sv: sv, pw: pw, done: make(chan struct{})}
	d.clients = append(d.clients, c)
	return c, nil
}

func (d *dialer) count() int { d.mu.Lock(); defer d.mu.Unlock(); return len(d.clients) }
func (d *dialer) last() *fakeClient {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.clients[len(d.clients)-1]
}

func rig(t *testing.T, open openFn) (*App, *fakeVPN, *events) {
	a, fv, ev, _ := rigDial(t, open)
	return a, fv, ev
}

func rigDial(t *testing.T, open openFn) (*App, *fakeVPN, *events, *dialer) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "data.json"), memSecrets{map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []store.VPN{
		{ID: "f", Name: "Forti", Kind: store.Forti, Gateway: "gw", Port: 10443, User: "u"},
		{ID: "h", Name: "Hill", Kind: store.Hillstone, Gateway: "gw", Port: 1, User: "u"},
	} {
		if _, err := st.SaveVPN(v, "vpw"); err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []store.Server{
		{ID: "direct", Name: "Direct", Host: "h", Port: 22, User: "u", Auth: "password"},
		{ID: "a", Name: "A", Host: "h", Port: 22, User: "u", Auth: "password", VPNID: "f"},
		{ID: "b", Name: "B", Host: "h", Port: 22, User: "u", Auth: "password", VPNID: "f"},
		{ID: "hs", Name: "HS", Host: "h", Port: 22, User: "u", Auth: "password", VPNID: "h"},
		{ID: "need", Name: "Need", Host: "h", Port: 22, User: "u", Auth: "password", VPNRequired: true},
	} {
		if _, err := st.SaveServer(s, "pw"); err != nil {
			t.Fatal(err)
		}
	}
	fv, ev := &fakeVPN{}, &events{}
	d := &dialer{open: open}
	a := newApp(st, fv, d.dial)
	a.emit = ev.emit
	a.lookPath = func(string) (string, error) { return "/bin/true", nil } // pretend the VPN clients are installed
	return a, fv, ev, d
}

func okOpen(f *fakeShell) func(context.Context, store.Server, string) (sshx.Session, error) {
	return func(context.Context, store.Server, string) (sshx.Session, error) { return f, nil }
}

func TestConnectDirectServer(t *testing.T) {
	a, fv, ev := rig(t, okOpen(newShell()))
	if r := a.Connect("direct"); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(fv.up) != 0 {
		t.Error("a direct server must not start a VPN")
	}
	steps := ev.get("connect:direct").([]Step)
	if len(steps) != 2 || steps[0].Key != "ssh" || steps[0].Status != "done" || steps[1].Status != "done" {
		t.Fatalf("steps %+v", steps)
	}
}

func TestConnectViaVPNStartsItFirst(t *testing.T) {
	a, fv, ev := rig(t, okOpen(newShell()))
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(fv.up) != 1 || fv.up[0] != "f" {
		t.Fatalf("vpn up calls %v", fv.up)
	}
	steps := ev.get("connect:a").([]Step)
	if steps[0].Key != "vpn" || steps[0].Status != "done" {
		t.Fatalf("steps %+v", steps)
	}
}

func TestVPNRequiredButNotSet(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	r := a.Connect("need")
	if r.OK || !strings.Contains(r.Problem, "needs a VPN") || r.Recovery == "" {
		t.Fatalf("%+v", r)
	}
}

func TestHillstoneWaitsForConfirmation(t *testing.T) {
	a, fv, ev := rig(t, okOpen(newShell()))
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("hs") }()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s, ok := ev.get("connect:hs").([]Step); ok && s[0].Status == "manual" {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case r := <-done:
		t.Fatalf("finished before confirmation: %+v", r)
	case <-time.After(50 * time.Millisecond):
	}
	a.ConfirmManualVPN("hs")
	if r := <-done; !r.OK {
		t.Fatalf("%+v", r)
	}
	if len(fv.up) != 0 {
		t.Error("the app must not start a Hillstone VPN")
	}
}

func TestWrongPasswordGivesProblemAndRecovery(t *testing.T) {
	a, _, ev := rig(t, func(context.Context, store.Server, string) (sshx.Session, error) {
		return nil, errors.New("login to h:22 as u failed: wrong user or password")
	})
	r := a.Connect("direct")
	if r.OK || !strings.Contains(r.Problem, "wrong user or password") || !strings.Contains(r.Recovery, "Admin") {
		t.Fatalf("%+v", r)
	}
	if s := ev.get("connect:direct").([]Step); s[0].Status != "fail" {
		t.Fatalf("ssh step %+v", s[0])
	}
	if a.conns["direct"] != nil {
		t.Error("a failed connect must not leave a connection behind")
	}
}

func TestDisconnectKeepsASharedVPNUntilTheLastServerLeaves(t *testing.T) {
	a, fv, _ := rig(t, okOpen(newShell()))
	for _, id := range []string{"a", "b"} {
		if r := a.Connect(id); !r.OK {
			t.Fatalf("%+v", r)
		}
	}
	a.Disconnect("a")
	if len(fv.down) != 0 {
		t.Fatalf("VPN taken down while B still uses it: %v", fv.down)
	}
	a.Disconnect("b")
	if len(fv.down) != 1 || fv.down[0] != "f" {
		t.Fatalf("VPN down calls %v", fv.down)
	}
}

func TestShellStreamsOutputAndInput(t *testing.T) {
	sh := newShell()
	a, _, ev := rig(t, okOpen(sh))
	if r := a.Connect("direct"); !r.OK {
		t.Fatal(r)
	}
	if err := a.AttachShell("direct", 1, 120, 30); err != nil {
		t.Fatal(err)
	}
	if sh.cols != 120 || sh.rows != 30 {
		t.Errorf("size %dx%d", sh.cols, sh.rows)
	}
	sh.out <- []byte("hello\xff")
	deadline := time.Now().Add(2 * time.Second)
	for !ev.has("term:direct:1:out") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	got, _ := ev.get("term:direct:1:out").(string)
	if b, _ := base64.StdEncoding.DecodeString(got); string(b) != "hello\xff" {
		t.Fatalf("output %q (bytes must survive, including invalid UTF-8)", b)
	}
	a.TermWrite("direct", 1, "ls\n")
	if sh.got.String() != "ls\n" {
		t.Fatalf("input %q", sh.got.String())
	}
	sh.Close() // the remote side hangs up
	for !ev.has("term:direct:1:end") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !ev.has("term:direct:1:end") {
		t.Fatal("no end event when the shell closed")
	}
}

func TestAttachWithoutConnectFails(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	if err := a.AttachShell("direct", 1, 80, 24); err == nil {
		t.Fatal("want an error when not connected")
	}
}

func TestNewTabOpensItsOwnSession(t *testing.T) {
	var opened int
	a, _, _ := rig(t, func(context.Context, store.Server, string) (sshx.Session, error) {
		opened++
		return newShell(), nil
	})
	if r := a.Connect("direct"); !r.OK {
		t.Fatal(r)
	}
	if err := a.AttachShell("direct", 2, 80, 24); err != nil {
		t.Fatal(err)
	}
	if opened != 2 {
		t.Fatalf("opened %d sessions, want 2 (one per tab)", opened)
	}
}

func TestMissingVPNClientIsExplained(t *testing.T) {
	a, fv, _ := rig(t, okOpen(newShell()))
	a.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	r := a.Connect("a")
	if r.OK || !strings.Contains(r.Problem, "openfortivpn is not installed") || !strings.Contains(r.Recovery, "System") || len(fv.up) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestDockerExecCommandIsBuiltSafely(t *testing.T) {
	got, err := dockerExec("app", "root", false)
	if err != nil || got != "exec docker exec -it -u root app sh -c 'command -v bash >/dev/null 2>&1 && exec bash || exec sh'\n" {
		t.Fatalf("%q, %v", got, err)
	}
	if got, _ := dockerExec("web-1", "", true); !strings.HasPrefix(got, "exec sudo docker exec -it web-1 ") || strings.Contains(got, "-u") {
		t.Fatalf("default user with sudo: %q", got)
	}
	for _, bad := range [][2]string{{"app; rm -rf /", ""}, {"$(id)", ""}, {"app", "root;id"}, {"-rm", ""}, {"", ""}, {"app", "a b"}} {
		if _, err := dockerExec(bad[0], bad[1], false); err == nil {
			t.Errorf("%q as %q must be refused", bad[0], bad[1])
		}
	}
	if _, err := dockerExec("app", "1000:1000", false); err != nil {
		t.Errorf("uid:gid is a valid user: %v", err)
	}
}

func TestContainerShellTypesTheCommandIntoItsOwnSession(t *testing.T) {
	var shells []*fakeShell
	a, _, _ := rig(t, func(context.Context, store.Server, string) (sshx.Session, error) {
		s := newShell()
		shells = append(shells, s)
		return s, nil
	})
	if r := a.Connect("direct"); !r.OK {
		t.Fatal(r)
	}
	if err := a.OpenContainerShell("direct", 2, "app", "root", false); err != nil {
		t.Fatal(err)
	}
	if len(shells) != 2 || !strings.Contains(shells[1].got.String(), "docker exec -it -u root app") {
		t.Fatalf("sessions %d, typed %q", len(shells), shells[len(shells)-1].got.String())
	}
	if shells[0].got.Len() != 0 {
		t.Error("the main shell must not receive the docker command")
	}
	if err := a.OpenContainerShell("direct", 3, "bad name", "", false); err == nil {
		t.Error("an invalid container name must be refused")
	}
}

func TestShutdownClosesEveryShellAndTheVPN(t *testing.T) {
	sh := newShell()
	a, fv, _ := rig(t, okOpen(sh))
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	a.shutdown(context.Background())
	if !sh.closed {
		t.Error("the SSH session was left open")
	}
	if len(fv.down) != 1 || fv.down[0] != "f" {
		t.Errorf("VPN down calls %v", fv.down)
	}
	if len(a.conns) != 0 {
		t.Error("connections remain after shutdown")
	}
}

func TestDependencyCheckAndInstallGuards(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	a.lookPath = func(f string) (string, error) {
		if f == "openconnect" || f == "pkexec" {
			return "/usr/bin/" + f, nil
		}
		return "", errors.New("no")
	}
	got := map[string]bool{}
	for _, d := range a.CheckDependencies() {
		got[d.Name] = d.Installed
	}
	if got["openfortivpn"] || !got["openconnect"] || !got["pkexec"] {
		t.Fatalf("%v", got)
	}
	for _, bad := range []string{"bash", "openfortivpn; id", "pkexec"} {
		if err := a.InstallDependencies([]string{bad}); err == nil {
			t.Errorf("%q must be refused: it is not an installable dependency", bad)
		}
	}
	argv, err := installArgv(func(f string) (string, error) {
		if f == "dnf" {
			return "/usr/bin/dnf", nil
		}
		return "", errors.New("no")
	}, []string{"openfortivpn"})
	if err != nil || strings.Join(argv, " ") != "pkexec dnf install -y openfortivpn" {
		t.Fatalf("%v, %v", argv, err)
	}
	if _, err := installArgv(func(string) (string, error) { return "", errors.New("no") }, []string{"x"}); err == nil {
		t.Error("an unknown distro must be reported")
	}
}

func TestUntrustedGatewayCertificateTellsTheUserWhatToPaste(t *testing.T) {
	a, fv, _ := rig(t, okOpen(newShell()))
	digest := strings.Repeat("ab", 32)
	fv.err = &vpn.CertError{Digest: digest}
	r := a.Connect("a")
	if r.OK || !strings.Contains(r.Problem, "certificate") || !strings.Contains(r.Recovery, digest) || !strings.Contains(r.Recovery, "Trusted certificate") {
		t.Fatalf("%+v", r)
	}
}

func waitFor(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out")
	}
}

func TestCancelWhileSSHIsConnectingTakesTheVPNDown(t *testing.T) {
	started := make(chan struct{})
	a, fv, _ := rig(t, func(ctx context.Context, _ store.Server, _ string) (sshx.Session, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("a") }()
	waitFor(t, started)
	a.Disconnect("a")
	r := <-done
	if r.OK || r.Problem != "Cancelled." {
		t.Fatalf("%+v", r)
	}
	if len(fv.down) != 1 || fv.down[0] != "f" {
		t.Fatalf("a VPN started for a cancelled connect must come down: %v", fv.down)
	}
	if a.conns["a"] != nil {
		t.Error("cancelled connect left a connection")
	}
}

func TestCancelThatLosesTheRaceStillLeavesNothingBehind(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	sh := newShell()
	a, fv, _ := rig(t, func(context.Context, store.Server, string) (sshx.Session, error) {
		close(started)
		<-release // SSH finishes after the user pressed Cancel
		return sh, nil
	})
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("a") }()
	waitFor(t, started)
	a.Disconnect("a")
	close(release)
	if r := <-done; r.OK {
		t.Fatalf("%+v", r)
	}
	sh.mu.Lock()
	closed := sh.closed
	sh.mu.Unlock()
	if !closed || a.conns["a"] != nil || len(fv.down) != 1 {
		t.Fatalf("shell closed=%v conn=%v vpn down=%v", closed, a.conns["a"], fv.down)
	}
}

func TestCancelKeepsAVPNAnotherServerIsUsing(t *testing.T) {
	started := make(chan struct{})
	n := 0
	a, fv, _ := rig(t, func(ctx context.Context, _ store.Server, _ string) (sshx.Session, error) {
		if n++; n == 1 {
			return newShell(), nil
		}
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if r := a.Connect("a"); !r.OK {
		t.Fatalf("%+v", r)
	}
	done := make(chan ConnectResult, 1)
	go func() { done <- a.Connect("b") }()
	waitFor(t, started)
	a.Disconnect("b")
	<-done
	if len(fv.down) != 0 {
		t.Fatalf("VPN taken down while A still uses it: %v", fv.down)
	}
}

func TestKeyLoginServerNeedsNoStoredPassword(t *testing.T) {
	var got string
	a, _, _ := rig(t, func(_ context.Context, sv store.Server, pw string) (sshx.Session, error) {
		got = sv.Auth + ":" + sv.KeyPath + ":" + pw
		return newShell(), nil
	})
	if _, err := a.SaveServer(store.Server{ID: "k", Name: "K", Host: "h", Port: 22, User: "u", Auth: "key", KeyPath: "~/.ssh/id_ed25519"}, ""); err != nil {
		t.Fatal(err)
	}
	if r := a.Connect("k"); !r.OK {
		t.Fatalf("%+v", r)
	}
	if got != "key:~/.ssh/id_ed25519:" {
		t.Fatalf("open got %q", got)
	}
	if _, err := a.Analyze("k"); err != nil && strings.Contains(err.Error(), "no password") {
		t.Fatalf("analysis demanded a password for a key server: %v", err)
	}
}

func TestRejectedKeyIsExplained(t *testing.T) {
	a, _, _ := rig(t, func(context.Context, store.Server, string) (sshx.Session, error) {
		return nil, errors.New("login to h:22 as u failed: the server did not accept the key: ssh: unable to authenticate")
	})
	if _, err := a.SaveServer(store.Server{ID: "k", Name: "K", Host: "h", Port: 22, User: "u", Auth: "key"}, ""); err != nil {
		t.Fatal(err)
	}
	r := a.Connect("k")
	if r.OK || !strings.Contains(r.Problem, "key") || !strings.Contains(r.Recovery, "authorized_keys") {
		t.Fatalf("%+v", r)
	}
}
