package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KushalNaral/sshdeck/internal/probe"
	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/vpn"
)

// Step is one stage of connecting, sent to the UI as it changes.
type Step struct {
	Key    string `json:"key"` // vpn, ssh, shell
	Label  string `json:"label"`
	Status string `json:"status"` // wait, run, manual, done, fail
	Ms     int64  `json:"ms,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// ConnectResult says how Connect ended. A failure is data, not an error, so the UI can show problem and recovery.
type ConnectResult struct {
	OK       bool   `json:"ok"`
	Problem  string `json:"problem,omitempty"`
	Recovery string `json:"recovery,omitempty"`
	// Details is the VPN client's command and output after a failed connect, to read or copy (no password in it).
	Details string `json:"details,omitempty"`
	quiet   bool   // not an attempt worth recording in the audit trail
}

type shell struct {
	s      sshx.Session
	pumped bool
}

type conn struct {
	vpnID  string
	client sshx.Client // the one SSH login every tab and the analysis share
	shells map[int]*shell
	stop   context.CancelFunc // ends the watcher that redials a dropped link
}

func fail(problem, recovery string) ConnectResult {
	return ConnectResult{Problem: problem, Recovery: recovery}
}

func (a *App) server(id string) (store.Server, bool) {
	for _, s := range a.st.Servers() {
		if s.ID == id {
			return s, true
		}
	}
	return store.Server{}, false
}

// Connect brings the VPN up if the server needs one, logs in over SSH and holds the first shell open.
// Progress arrives as "connect:<serverId>" events carrying the Step list.
// Connect brings up the VPN if the server needs one, then SSH, and records the outcome in the audit trail.
func (a *App) Connect(serverID string) ConnectResult {
	r := a.connect(serverID)
	if r.quiet {
		return r
	}
	name, via := a.serverName(serverID), "direct"
	if sv, ok := a.server(serverID); ok && a.vpnFor(sv) != "" {
		via = "via " + a.vpnName(a.vpnFor(sv))
	}
	switch {
	case r.OK:
		a.audit("connect", name, via)
	case r.Problem == "Cancelled.":
		a.audit("connect.cancel", name, via)
	default:
		a.audit("connect.fail", name, r.Problem)
	}
	return r
}

// quiet marks results that are not an attempt worth recording (locked, unknown server, already connected).
func quiet(r ConnectResult) ConnectResult { r.quiet = true; return r }

func (a *App) connect(serverID string) ConnectResult {
	if a.isLocked() {
		return quiet(fail("sshdeck is locked.", "Enter the passphrase, then connect."))
	}
	sv, ok := a.server(serverID)
	if !ok {
		return quiet(fail("That server no longer exists.", "Pick another server."))
	}
	a.mu.Lock()
	if a.conns[serverID] != nil {
		a.mu.Unlock()
		return quiet(ConnectResult{OK: true})
	}
	if a.cancels[serverID] != nil {
		a.mu.Unlock()
		return quiet(fail("Already connecting to "+sv.Name+".", "Wait for it to finish, or disconnect first."))
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.cancels[serverID] = cancel
	a.mu.Unlock()
	defer func() {
		cancel()
		a.mu.Lock()
		delete(a.cancels, serverID)
		a.mu.Unlock()
	}()

	var vp *store.VPN
	for _, v := range a.st.VPNs() {
		if want := a.vpnFor(sv); v.ID == want && want != "" {
			v := v
			vp = &v
		}
	}
	var steps []Step
	if vp != nil || sv.VPNRequired {
		d := "not set"
		if vp != nil {
			d = vp.Name
		}
		steps = append(steps, Step{Key: "vpn", Label: "VPN", Status: "wait", Detail: d})
	}
	steps = append(steps,
		Step{Key: "ssh", Label: "SSH", Status: "wait", Detail: a.sshDetail(sv)},
		Step{Key: "shell", Label: "Shell", Status: "wait"})
	set := func(key string, patch func(*Step)) {
		for i := range steps {
			if steps[i].Key == key {
				patch(&steps[i])
			}
		}
		a.emit("connect:"+serverID, append([]Step(nil), steps...))
	}
	a.emit("connect:"+serverID, append([]Step(nil), steps...))

	switch {
	case vp == nil && sv.VPNRequired:
		set("vpn", func(s *Step) { s.Status = "fail" })
		return fail(sv.Name+" needs a VPN, but no VPN is set.", "Choose its VPN in Admin, then connect again.")
	case vp != nil && !autoVPN(vp):
		ch := make(chan struct{}, 1)
		a.mu.Lock()
		a.manual[serverID] = ch
		a.mu.Unlock()
		t := time.Now()
		set("vpn", func(s *Step) { s.Status = "manual" })
		select {
		case <-ch:
			set("vpn", func(s *Step) { s.Status, s.Ms = "done", time.Since(t).Milliseconds() })
		case <-ctx.Done():
			return fail("Cancelled.", "Connect again when the VPN is ready.")
		}
	case vp != nil:
		if res, bad := a.vpnUp(ctx, *vp, set); bad {
			return res
		}
	}

	t := time.Now()
	set("ssh", func(s *Step) { s.Status = "run" })
	if vp != nil { // a VPN that has just come up may not have its routes in place yet
		if err := a.waitRoute(ctx, sv, set); err != nil {
			a.releaseVPN(vp)
			return cancelled()
		}
	}
	pw, err := a.serverSecret(sv)
	if err != nil {
		set("ssh", func(s *Step) { s.Status = "fail" })
		return fail("No password is stored for "+sv.Name+".", "Open Admin, edit the server and enter its password.")
	}
	client, err := a.dialServer(ctx, sv, pw)
	var sess sshx.Session
	if err == nil {
		if sess, err = client.Shell(ctx); err != nil {
			client.Close()
		}
	}
	if err != nil {
		if ctx.Err() != nil {
			a.releaseVPN(vp)
			return cancelled()
		}
		set("ssh", func(s *Step) { s.Status = "fail" })
		return sshFailure(err)
	}
	set("ssh", func(s *Step) { s.Status, s.Ms = "done", time.Since(t).Milliseconds() })
	set("shell", func(s *Step) { s.Status = "done" })

	c := &conn{client: client, shells: map[int]*shell{1: {s: sess}}}
	if vp != nil {
		c.vpnID = vp.ID
	}
	a.mu.Lock()
	if ctx.Err() != nil { // cancelled just as SSH finished: Disconnect cancels under this same lock
		a.mu.Unlock()
		sess.Close()
		client.Close()
		a.releaseVPN(vp)
		return cancelled()
	}
	wctx, stop := context.WithCancel(a.ctx)
	c.stop = stop
	a.conns[serverID] = c
	a.mu.Unlock()
	go a.watch(wctx, serverID, c, vp)
	go a.startAutoTunnels(serverID)
	return ConnectResult{OK: true}
}

func cancelled() ConnectResult { return fail("Cancelled.", "Connect again when you are ready.") }

// releaseVPN takes a VPN down after a connect that was cancelled, unless another server is using it.
func (a *App) releaseVPN(vp *store.VPN) {
	if vp == nil {
		return
	}
	a.mu.Lock()
	for _, o := range a.conns {
		if o.vpnID == vp.ID {
			a.mu.Unlock()
			return
		}
	}
	a.mu.Unlock()
	_ = a.vpn.Down(vp.ID)
	a.audit("vpn.down", vp.Name, "connect cancelled")
}

func (a *App) vpnUp(ctx context.Context, v store.VPN, set func(string, func(*Step))) (ConnectResult, bool) {
	tool := vpn.Tool(v.Kind)
	if _, err := a.lookPath(tool); err != nil {
		set("vpn", func(s *Step) { s.Status = "fail" })
		return fail(tool+" is not installed, so "+v.Name+" cannot start.", "Open Admin, System, and install it. Then connect again."), true
	}
	pw, err := a.st.Password("vpn", v.ID)
	if err != nil {
		set("vpn", func(s *Step) { s.Status = "fail" })
		return fail("No password is stored for the VPN "+v.Name+".", "Open Admin, edit the VPN profile and enter its password."), true
	}
	t := time.Now()
	set("vpn", func(s *Step) { s.Status = "run" })
	upCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// Show what the client is saying, so a connect that is slow or stuck says why.
	upCtx = vpn.WithProgress(upCtx, func(line string) {
		set("vpn", func(s *Step) { s.Detail = v.Name + " · " + shorten(line, 90) })
	})
	if err := a.vpn.Up(upCtx, v, pw); err != nil {
		set("vpn", func(s *Step) { s.Status = "fail" })
		recovery := "Check the VPN user, password and gateway in Admin, then connect again."
		if errors.Is(err, context.Canceled) {
			return fail("Cancelled.", "Connect again when you are ready."), true
		}
		var ce *vpn.CertError
		if errors.As(err, &ce) {
			res := fail(v.Name+" presented a certificate this computer does not trust.",
				"If you trust this gateway, open Admin, VPN profiles, edit "+v.Name+", press Fetch from gateway, compare the fingerprint with the administrator's and trust it. Or paste this into Trusted certificate: "+ce.Digest)
			return res, true
		}
		res := fail(fmt.Sprintf("%s did not come up: %v", v.Name, err), recovery)
		var cerr *vpn.ClientError
		if errors.As(err, &cerr) {
			res.Details = "$ " + cerr.Command + "\n" + strings.Join(cerr.Output, "\n")
		}
		if cerr != nil && strings.Contains(strings.Join(cerr.Output, "\n"), "512") {
			res.Recovery = "The portal answered HTTP 512 to the login, which means it refused it: the user or password is wrong for this portal, or it rejected this client. Check the password stored for this VPN (try the same login in the official app or on the portal's web page). If they are right, edit the VPN profile and set Sign in at to Gateway, or tick \"I connect this VPN myself\" and use the official app."
		} else if strings.Contains(err.Error(), "Failed to complete authentication") {
			res.Recovery = "The gateway refused the login. Check the user and password. If they are right, this gateway may need single sign-on or a host check that openconnect cannot do: mark the VPN profile as \"I connect this VPN myself\" and use the official GlobalProtect or FortiClient app."
		}
		return res, true
	}
	set("vpn", func(s *Step) { s.Status, s.Ms, s.Detail = "done", time.Since(t).Milliseconds(), v.Name })
	a.audit("vpn.up", v.Name, "")
	return ConnectResult{}, false
}

// autoVPN reports whether sshdeck itself starts this VPN, as opposed to waiting for you to connect it.
func autoVPN(v *store.VPN) bool { return vpn.Automatic(v.Kind) && !v.Manual }

// probeTCP reports whether host:port accepts a connection.
func probeTCP(ctx context.Context, host string, port int) error {
	c, err := (&net.Dialer{}).DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return c.Close()
}

// waitRoute waits until the server answers on its SSH port, for at most routeWait. A VPN client says the tunnel is up
// a moment before its routes and DNS are in place, so the first connection attempt right after "up" can time out
// although a retry a few seconds later works. When the time is up it returns nil all the same, so that SSH is tried
// and its own error reaches the user. It returns an error only when the connect is cancelled.
func (a *App) waitRoute(ctx context.Context, sv store.Server, set func(string, func(*Step))) error {
	deadline := time.Now().Add(a.routeWait)
	waiting := false
	for {
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		en := a.entry(sv) // behind a jump host, the route that matters is the one to the jump host
		err := a.probe(pctx, en.Host, en.Port)
		cancel()
		if err == nil {
			if waiting {
				set("ssh", func(s *Step) { s.Detail = a.sshDetail(sv) })
			}
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !waiting {
			waiting = true
			set("ssh", func(s *Step) {
				s.Detail = a.sshDetail(sv) + " · waiting for the VPN route"
			})
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(a.routePoll):
		}
	}
}

// shorten cuts text to at most n characters, with an ellipsis.
func shorten(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// serverSecret is the stored password, or for key login the key's passphrase, which may be empty (no passphrase, or ssh-agent).
func (a *App) serverSecret(sv store.Server) (string, error) {
	pw, err := a.st.Password("server", sv.ID)
	if err != nil && sv.Auth == "key" && errors.Is(err, store.ErrNotFound) {
		return "", nil
	}
	return pw, err
}

// sshFailure turns an sshx error into the problem and recovery the UI shows.
func sshFailure(err error) ConnectResult {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "wrong user or password"):
		return fail("The server refused the login: wrong user or password.", "Fix the user or password in Admin, then connect again.")
	case strings.Contains(msg, "did not accept the key"):
		return fail("The server did not accept the key.", "Add the public key to the user's authorized_keys on the server, or pick another key file in Admin.")
	case strings.Contains(msg, "passphrase") || strings.Contains(msg, "key file") || strings.Contains(msg, "ssh-agent"):
		return fail(msg, "Fix the key settings for this server in Admin, then connect again.")
	case strings.Contains(msg, "cannot reach"):
		return fail("The server cannot be reached. "+msg, "Check the host and port, and that the VPN is up.")
	case strings.Contains(msg, "host key"):
		return fail(msg, "If the server was rebuilt on purpose, remove its line from the known_hosts file.")
	}
	return fail("SSH failed: "+msg, "Try again; if it keeps failing, check the server in Admin.")
}

// ConfirmManualVPN tells a waiting Hillstone connect that the user has connected the VPN themselves.
func (a *App) ConfirmManualVPN(serverID string) {
	a.mu.Lock()
	ch := a.manual[serverID]
	delete(a.manual, serverID)
	a.mu.Unlock()
	if ch != nil {
		ch <- struct{}{}
	}
}

// Disconnect stops a connect in flight, closes every shell and takes the VPN down when nothing else uses it.
func (a *App) Disconnect(serverID string) {
	a.mu.Lock()
	if cancel := a.cancels[serverID]; cancel != nil {
		cancel()
	}
	delete(a.manual, serverID)
	c := a.conns[serverID]
	delete(a.conns, serverID)
	inUse := false
	if c != nil {
		for _, o := range a.conns {
			if o.vpnID == c.vpnID {
				inUse = true
			}
		}
	}
	a.mu.Unlock()
	if c == nil {
		return
	}
	a.stopTunnels(serverID)
	a.stopLogs(serverID)
	if c.stop != nil {
		c.stop()
	}
	for _, sh := range c.shells {
		sh.s.Close()
	}
	if c.client != nil {
		c.client.Close()
	}
	a.audit("disconnect", a.serverName(serverID), "")
	if c.vpnID != "" && !inUse {
		_ = a.vpn.Down(c.vpnID)
		a.audit("vpn.down", a.vpnName(c.vpnID), "")
	}
}

func shellEvent(serverID string, tab int, kind string) string {
	return fmt.Sprintf("term:%s:%d:%s", serverID, tab, kind)
}

// AttachShell starts streaming a shell's output as "term:<serverId>:<tab>:out" events (base64) and sets its size.
// Tab 1 already exists from Connect; any other tab is opened here. Call it after subscribing to the events.
func (a *App) AttachShell(serverID string, tab, cols, rows int) error {
	if err := a.guard(); err != nil {
		return err
	}
	a.mu.Lock()
	c := a.conns[serverID]
	a.mu.Unlock()
	if c == nil {
		return errors.New("not connected")
	}
	sh, err := a.ensureShell(serverID, c, tab)
	if err != nil {
		return err
	}
	_ = sh.s.Resize(cols, rows)
	a.mu.Lock()
	start := !sh.pumped
	sh.pumped = true
	a.mu.Unlock()
	if start { // a remount of the same tab reuses the running stream: events are keyed by name
		go func() {
			for b := range sh.s.Output() {
				a.emit(shellEvent(serverID, tab, "out"), base64.StdEncoding.EncodeToString(b))
			}
			a.mu.Lock()
			if c.shells[tab] == sh {
				delete(c.shells, tab)
			}
			a.mu.Unlock()
			a.emit(shellEvent(serverID, tab, "end"))
		}()
	}
	return nil
}

func (a *App) shell(serverID string, tab int) *shell {
	a.mu.Lock()
	defer a.mu.Unlock()
	if c := a.conns[serverID]; c != nil {
		return c.shells[tab]
	}
	return nil
}

func (a *App) TermWrite(serverID string, tab int, data string) {
	if a.isLocked() {
		return
	}
	a.Touch()
	if sh := a.shell(serverID, tab); sh != nil {
		_ = sh.s.Write([]byte(data))
	}
}

func (a *App) TermResize(serverID string, tab, cols, rows int) {
	if a.isLocked() {
		return
	}
	if sh := a.shell(serverID, tab); sh != nil {
		_ = sh.s.Resize(cols, rows)
	}
}

func (a *App) CloseShell(serverID string, tab int) {
	a.mu.Lock()
	var sh *shell
	if c := a.conns[serverID]; c != nil {
		sh = c.shells[tab]
		delete(c.shells, tab)
	}
	a.mu.Unlock()
	if sh != nil {
		sh.s.Close()
	}
}

// Analyze runs the read-only probe commands on a server. The server must be reachable (VPN up if it needs one).
func (a *App) Analyze(serverID string) (probe.Report, error) {
	if err := a.guard(); err != nil {
		return probe.Report{}, err
	}
	sv, ok := a.server(serverID)
	if !ok {
		return probe.Report{}, errors.New("that server no longer exists")
	}
	pw, err := a.serverSecret(sv)
	if err != nil {
		return probe.Report{}, errors.New("no password is stored for " + sv.Name)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	// Use the connection that is already open. Without one, log in once for the whole analysis, not per command.
	a.mu.Lock()
	var cl sshx.Client
	if c := a.conns[serverID]; c != nil {
		cl = c.client
	}
	a.mu.Unlock()
	if cl == nil {
		dctx, dcancel := context.WithTimeout(ctx, 20*time.Second)
		var err error
		cl, err = a.dialServer(dctx, sv, pw)
		dcancel()
		if err != nil {
			return probe.Report{}, fmt.Errorf("%s is not reachable: %w", sv.Name, err)
		}
		defer cl.Close()
	}
	run := func(cmd string) (string, error) {
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		return cl.Run(c, cmd)
	}
	if _, err := run("true"); err != nil { // one quick check first, so an unreachable server fails once, not eleven times
		return probe.Report{}, fmt.Errorf("%s is not reachable: %w", sv.Name, err)
	}
	r, err := probe.Analyze(ctx, run)
	if err != nil {
		return r, fmt.Errorf("%s could not be analysed: %w", sv.Name, err)
	}
	return r, nil
}

// ensureShell returns the shell for a tab, opening its own SSH session when the tab is new.
// liveClient returns the connection's shared SSH client, logging in again when the link has dropped
// (the next tab then works without a full reconnect).
func (a *App) liveClient(ctx context.Context, c *conn, sv store.Server, pw string) (sshx.Client, error) {
	a.mu.Lock()
	cl := c.client
	a.mu.Unlock()
	if cl != nil {
		select {
		case <-cl.Done():
		default:
			return cl, nil
		}
	}
	fresh, err := a.dialServer(ctx, sv, pw)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if cur := c.client; cur != nil && cur != cl { // another tab reconnected first
		fresh.Close()
		return cur, nil
	}
	c.client = fresh
	return fresh, nil
}

func (a *App) ensureShell(serverID string, c *conn, tab int) (*shell, error) {
	a.mu.Lock()
	sh := c.shells[tab]
	a.mu.Unlock()
	if sh != nil {
		return sh, nil
	}
	sv, ok := a.server(serverID)
	if !ok {
		return nil, errors.New("that server no longer exists")
	}
	pw, err := a.serverSecret(sv)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	cl, err := a.liveClient(ctx, c, sv, pw)
	if err != nil {
		return nil, err
	}
	s, err := cl.Shell(ctx)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.conns[serverID] != c { // disconnected while the shell was opening
		a.mu.Unlock()
		s.Close()
		if c.client != nil {
			c.client.Close()
		}
		return nil, errors.New("not connected")
	}
	if cur := c.shells[tab]; cur != nil { // lost a race with another attach; keep the first
		a.mu.Unlock()
		s.Close()
		return cur, nil
	}
	sh = &shell{s: s}
	c.shells[tab] = sh
	a.mu.Unlock()
	a.audit("shell.open", sv.Name, fmt.Sprintf("tab %d", tab))
	return sh, nil
}

var (
	containerRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
	userRe      = regexp.MustCompile(`^[A-Za-z0-9_.][A-Za-z0-9_.:-]*$`)
)

// dockerExec builds the command typed into a fresh shell. Names are checked first, so nothing here can
// carry shell syntax. exec replaces the login shell, so leaving the container ends the tab.
func dockerExec(container, user string, sudo bool) (string, error) {
	if !containerRe.MatchString(container) {
		return "", fmt.Errorf("%q is not a valid container name", container)
	}
	if user != "" && !userRe.MatchString(user) {
		return "", fmt.Errorf("%q is not a valid user (use a name, or uid:gid)", user)
	}
	cmd := "exec "
	if sudo {
		cmd += "sudo "
	}
	cmd += "docker exec -it "
	if user != "" {
		cmd += "-u " + user + " "
	}
	return cmd + container + " sh -c 'command -v bash >/dev/null 2>&1 && exec bash || exec sh'\n", nil
}

// OpenContainerShell opens tab as an SSH session that drops straight into a running container.
// user "" means the container's default user; "root" is the admin user. Then call AttachShell for the tab.
func (a *App) OpenContainerShell(serverID string, tab int, container, user string, sudo bool) error {
	if err := a.guard(); err != nil {
		return err
	}
	line, err := dockerExec(container, user, sudo)
	if err != nil {
		return err
	}
	a.mu.Lock()
	c := a.conns[serverID]
	a.mu.Unlock()
	if c == nil {
		return errors.New("not connected")
	}
	sh, err := a.ensureShell(serverID, c, tab)
	if err != nil {
		return err
	}
	if err := sh.s.Write([]byte(line)); err != nil {
		return err
	}
	a.audit("container.exec", a.serverName(serverID), fmt.Sprintf("container=%s user=%s sudo=%t", container, user, sudo))
	return nil
}
