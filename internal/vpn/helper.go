package vpn

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// On Windows a VPN client needs administrator rights, and a normal process cannot read the output of, or stop, an
// elevated one. So sshdeck starts a second copy of itself with a UAC prompt (the helper, "sshdeck --vpn-helper").
// The helper dials back to the app over loopback, proves it knows a one-time token, and then runs VPN clients on
// request. It runs nothing but openconnect with a fixed set of options (validateOpenconnectArgs), so even a process
// that learned the token cannot make it run anything else. It stops its clients and exits when the app goes away.
// The code here is plain Go so it can be tested anywhere; only the elevated launch is Windows-specific.

type helperMsg struct {
	Op    string   `json:"op"` // app to helper: hello, start, stop. helper to app: hello, line, exit
	ID    int      `json:"id,omitempty"`
	Token string   `json:"token,omitempty"`
	Argv  []string `json:"argv,omitempty"`
	Stdin string   `json:"stdin,omitempty"`
	Line  string   `json:"line,omitempty"`
	Err   string   `json:"err,omitempty"`
}

// HelperStarter is a Starter that runs clients in the elevated helper, starting the helper on first use.
type HelperStarter struct {
	// Launch starts the elevated helper, which must connect to addr and say hello with token.
	Launch func(addr, token string) error

	mu   sync.Mutex
	sess *helperSession
}

type helperSession struct {
	conn net.Conn
	wmu  sync.Mutex
	enc  *json.Encoder

	mu    sync.Mutex
	procs map[int]*helperProc
	next  int
	dead  bool
}

type helperProc struct {
	id    int
	sess  *helperSession
	lines chan string
	done  chan struct{}
	err   error
}

func (s *helperSession) send(m helperMsg) error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	return s.enc.Encode(m)
}

func (s *helperSession) isDead() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.dead }

func (s *helperSession) proc(id int) *helperProc {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.procs[id]
}

func (p *helperProc) finish(err error) {
	p.sess.mu.Lock()
	delete(p.sess.procs, p.id)
	p.sess.mu.Unlock()
	p.err = err
	close(p.lines)
	close(p.done)
}

func (p *helperProc) Lines() <-chan string { return p.lines }
func (p *helperProc) Wait() error          { <-p.done; return p.err }
func (p *helperProc) Stop() error {
	select {
	case <-p.done:
		return nil
	default:
	}
	_ = p.sess.send(helperMsg{Op: "stop", ID: p.id})
	select {
	case <-p.done:
		return nil
	case <-time.After(10 * time.Second):
		return errors.New("the VPN client did not stop")
	}
}

// pump reads the helper's messages and hands them to the right process. It ends when the connection does, and then
// every process still running is ended with an error, since nothing can stop or watch it any more.
func (s *helperSession) pump() {
	dec := json.NewDecoder(s.conn)
	for {
		var m helperMsg
		if err := dec.Decode(&m); err != nil {
			break
		}
		p := s.proc(m.ID)
		if p == nil {
			continue
		}
		switch m.Op {
		case "line":
			p.lines <- m.Line
		case "exit":
			if m.Err != "" {
				p.finish(errors.New(m.Err))
			} else {
				p.finish(nil)
			}
		}
	}
	s.mu.Lock()
	s.dead = true
	rest := make([]*helperProc, 0, len(s.procs))
	for _, p := range s.procs {
		rest = append(rest, p)
	}
	s.mu.Unlock()
	for _, p := range rest {
		p.finish(errors.New("the VPN helper stopped"))
	}
	s.conn.Close()
}

// session returns the connection to the helper, starting the helper (and so the UAC prompt) when there is none.
func (h *HelperStarter) session(ctx context.Context) (*helperSession, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sess != nil && !h.sess.isDead() {
		return h.sess, nil
	}
	if h.Launch == nil {
		return nil, errors.New("this system cannot start the elevated VPN helper")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(raw)
	if err := h.Launch(ln.Addr().String(), token); err != nil {
		return nil, fmt.Errorf("a VPN needs administrator approval to create its tunnel, and it was not given: %w", err)
	}
	got := make(chan net.Conn, 1)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
			var hello helperMsg
			dec := json.NewDecoder(c)
			if dec.Decode(&hello) == nil && hello.Op == "hello" && subtle.ConstantTimeCompare([]byte(hello.Token), []byte(token)) == 1 {
				_ = c.SetReadDeadline(time.Time{})
				got <- c
				return
			}
			c.Close() // someone else on this machine found the port: not our helper
		}
	}()
	select {
	case c := <-got:
		s := &helperSession{conn: c, enc: json.NewEncoder(c), procs: map[int]*helperProc{}}
		go s.pump()
		h.sess = s
		return s, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(90 * time.Second):
		return nil, errors.New("the administrator prompt was not answered in time")
	}
}

func (h *HelperStarter) Start(ctx context.Context, argv []string, stdin string) (Proc, error) {
	s, err := h.session(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.next++
	p := &helperProc{id: s.next, sess: s, lines: make(chan string, 64), done: make(chan struct{})}
	s.procs[p.id] = p
	s.mu.Unlock()
	if err := s.send(helperMsg{Op: "start", ID: p.id, Argv: argv, Stdin: stdin}); err != nil {
		p.finish(err)
		return nil, err
	}
	return p, nil
}

// Close ends the helper, which stops its clients and exits.
func (h *HelperStarter) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sess != nil {
		return h.sess.conn.Close()
	}
	return nil
}

func (h *HelperStarter) dropForTest() { _ = h.Close() }

// RunHelperWith is the helper's side: connect to the app, say hello, then serve start and stop requests with s,
// running only what allow accepts. It returns when the app disconnects.
func RunHelperWith(addr, token string, s Starter, allow func(argv []string) error) error {
	c, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	enc := json.NewEncoder(c)
	if err := enc.Encode(helperMsg{Op: "hello", Token: token}); err != nil {
		return err
	}
	var wmu sync.Mutex
	send := func(m helperMsg) { wmu.Lock(); _ = enc.Encode(m); wmu.Unlock() }
	var mu sync.Mutex
	procs := map[int]Proc{}
	dec := json.NewDecoder(c)
	for {
		var m helperMsg
		if err := dec.Decode(&m); err != nil {
			break
		}
		switch m.Op {
		case "start":
			if err := allow(m.Argv); err != nil {
				send(helperMsg{Op: "exit", ID: m.ID, Err: err.Error()})
				continue
			}
			p, err := s.Start(context.Background(), m.Argv, m.Stdin)
			if err != nil {
				send(helperMsg{Op: "exit", ID: m.ID, Err: err.Error()})
				continue
			}
			mu.Lock()
			procs[m.ID] = p
			mu.Unlock()
			go func(id int, p Proc) {
				for l := range p.Lines() {
					send(helperMsg{Op: "line", ID: id, Line: l})
				}
				exit := helperMsg{Op: "exit", ID: id}
				if err := p.Wait(); err != nil {
					exit.Err = err.Error()
				}
				mu.Lock()
				delete(procs, id)
				mu.Unlock()
				send(exit)
			}(m.ID, p)
		case "stop":
			mu.Lock()
			p := procs[m.ID]
			mu.Unlock()
			if p != nil {
				go p.Stop()
			}
		}
	}
	mu.Lock()
	rest := make([]Proc, 0, len(procs))
	for _, p := range procs {
		rest = append(rest, p)
	}
	mu.Unlock()
	for _, p := range rest { // the app is gone: leave no tunnel running
		_ = p.Stop()
	}
	return nil
}

// RunHelper is what "sshdeck --vpn-helper ADDR TOKEN" runs. It returns the process exit code.
func RunHelper(addr, token string) int {
	if err := RunHelperWith(addr, token, &helperExec{inner: ExecStarter{}, install: defaultInstall}, allowHelper); err != nil {
		fmt.Fprintln(os.Stderr, "sshdeck vpn helper:", err)
		return 1
	}
	return 0
}

// allowOpenconnect accepts only the installed openconnect with options validateOpenconnectArgs accepts.
func allowOpenconnect(argv []string) error {
	if len(argv) == 0 {
		return errors.New("empty command")
	}
	exe, err := resolve("openconnect")
	if err != nil {
		return err
	}
	if !strings.EqualFold(argv[0], exe) {
		return fmt.Errorf("only %s may be run", exe)
	}
	return validateOpenconnectArgs(argv[1:])
}

var serverCertRe = regexp.MustCompile(`^(pin-sha256:[A-Za-z0-9+/]{43}=|sha256:[0-9a-fA-F]{64})$`)

// validateOpenconnectArgs accepts exactly the options sshdeck itself builds, and checks every value, so the elevated
// helper cannot be made to run a script (--script) or read arbitrary files.
func validateOpenconnectArgs(args []string) error {
	var proto, user, cert, gw string
	stdin := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--protocol=gp" || a == "--protocol=fortinet":
			if proto != "" {
				return errors.New("protocol given twice")
			}
			proto = a
		case a == "--passwd-on-stdin":
			stdin = true
		case a == "--non-inter":
			// never wait for an answer: nobody can type one into a hidden process
		case a == "--user" || a == "--servercert":
			if i+1 >= len(args) {
				return fmt.Errorf("%s needs a value", a)
			}
			i++
			if a == "--user" {
				user = args[i]
			} else {
				cert = args[i]
			}
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("option %q is not allowed", a)
		default:
			if gw != "" {
				return errors.New("more than one gateway")
			}
			gw = a
		}
	}
	switch {
	case proto == "":
		return errors.New("protocol missing")
	case !stdin:
		return errors.New("the password must be read from stdin")
	case user == "":
		return errors.New("user missing")
	case gw == "":
		return errors.New("gateway missing")
	case cert != "" && !serverCertRe.MatchString(cert):
		return errors.New("certificate pin is not valid")
	}
	host, port := gw, 443
	if h, p, err := net.SplitHostPort(gw); err == nil {
		n, perr := strconv.Atoi(p)
		if perr != nil {
			return errors.New("gateway port is not a number")
		}
		host, port = h, n
	}
	return store.ValidateVPN(store.VPN{Name: "vpn", Kind: store.GP, Gateway: host, Port: port, User: user}, "")
}

// underDir reports whether path is inside dir, ignoring case and the kind of slash (Windows paths).
func underDir(path, dir string) bool {
	norm := func(s string) string { return strings.TrimRight(strings.ToLower(strings.ReplaceAll(s, `\`, "/")), "/") }
	return strings.HasPrefix(norm(path), norm(dir)+"/")
}
