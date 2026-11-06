package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	runWorkers = 4               // servers working at once
	runCap     = 256 * 1024      // output kept per server; the end is kept
	runTimeout = 5 * time.Minute // per server
	exitMark   = "\x01sshdeck-exit:"
)

// RunResult is one server's part of a run.
type RunResult struct {
	Server    string `json:"server"`
	Name      string `json:"name"`
	State     string `json:"state"` // waiting, running, done, failed, cancelled or skipped
	Exit      int    `json:"exit"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated,omitempty"`
	Ms        int64  `json:"ms"`
	Error     string `json:"error,omitempty"`
}

type runState struct {
	mu      sync.Mutex
	results []RunResult
	cancel  context.CancelFunc
}

// capWriter keeps the last runCap bytes written to it.
type capWriter struct {
	mu   sync.Mutex
	b    []byte
	cut  bool
	note func()
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.b = append(w.b, p...)
	if len(w.b) > runCap*2 { // trim in bulk so a long stream is not copied on every write
		w.b, w.cut = append([]byte(nil), w.b[len(w.b)-runCap:]...), true
	}
	w.mu.Unlock()
	if w.note != nil {
		w.note()
	}
	return len(p), nil
}

func (w *capWriter) text() (string, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	b, cut := w.b, w.cut
	if len(b) > runCap {
		b, cut = b[len(b)-runCap:], true
	}
	return string(b), cut
}

// RunOn runs a command on the given servers that are connected, a few at a time, and returns a run id.
// Progress arrives as "run:<id>" events, one RunResult at a time. Servers that are not connected are skipped,
// never dialled: connecting goes through the normal flow, which asks about VPNs and production.
func (a *App) RunOn(serverIDs []string, command string) (string, error) {
	command = strings.TrimSpace(command)
	switch {
	case command == "":
		return "", errors.New("type a command to run")
	case len(serverIDs) == 0:
		return "", errors.New("pick at least one server")
	case len(command) > 8192:
		return "", errors.New("the command is too long")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	rs := &runState{cancel: cancel}
	for _, id := range serverIDs {
		rs.results = append(rs.results, RunResult{Server: id, Name: a.serverName(id), State: "waiting"})
	}
	a.rmu.Lock()
	a.runSeq++
	id := "run" + strconv.Itoa(a.runSeq)
	a.runs[id] = rs
	delete(a.runs, "run"+strconv.Itoa(a.runSeq-5)) // only the last few are kept
	a.rmu.Unlock()
	a.audit("run", short(command), fmt.Sprintf("%d servers", len(serverIDs)))

	go func() {
		defer cancel()
		sem := make(chan struct{}, runWorkers)
		var wg sync.WaitGroup
		for i := range serverIDs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-ctx.Done():
				}
				a.runOne(ctx, id, rs, i, command)
			}()
		}
		wg.Wait()
	}()
	return id, nil
}

func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 80 {
		return s[:80]
	}
	return s
}

func (a *App) runOne(ctx context.Context, id string, rs *runState, i int, command string) {
	set := func(f func(*RunResult)) {
		rs.mu.Lock()
		f(&rs.results[i])
		r := rs.results[i]
		rs.mu.Unlock()
		a.emit("run:"+id, r)
	}
	server := rs.results[i].Server
	if ctx.Err() != nil {
		set(func(r *RunResult) { r.State = "cancelled" })
		return
	}
	cl := a.clientOf(server)
	if cl == nil {
		set(func(r *RunResult) { r.State, r.Error = "skipped", "Not connected. Connect to this server first." })
		return
	}
	start := time.Now()
	set(func(r *RunResult) { r.State = "running" })
	var last time.Time
	w := &capWriter{}
	w.note = func() { // live output, at most ten updates a second
		if time.Since(last) < 100*time.Millisecond {
			return
		}
		last = time.Now()
		out, cut := w.text()
		set(func(r *RunResult) { r.Output, r.Truncated = out, cut })
	}
	rctx, cancel := context.WithTimeout(ctx, runTimeout)
	defer cancel()
	// A subshell keeps "exit" in the command from skipping the marker; stderr joins stdout.
	err := cl.Exec(rctx, "(\n"+command+"\n) 2>&1; printf '"+strings.ReplaceAll(exitMark, "\x01", "\\001")+"%d' $?", nil, w)
	out, cut := w.text()
	code := -1
	if k := strings.LastIndex(out, exitMark); k >= 0 {
		if n, e := strconv.Atoi(strings.TrimSpace(out[k+len(exitMark):])); e == nil {
			code = n
		}
		out = out[:k]
	}
	set(func(r *RunResult) {
		r.Output, r.Truncated, r.Exit, r.Ms = out, cut, code, time.Since(start).Milliseconds()
		switch {
		case ctx.Err() != nil:
			r.State = "cancelled"
		case rctx.Err() != nil:
			r.State, r.Error = "failed", "Stopped after "+runTimeout.String()+"."
		case err != nil && code < 0:
			r.State, r.Error = "failed", err.Error()
		case code != 0:
			r.State = "failed"
		default:
			r.State = "done"
		}
	})
}

// RunResults is the current state of a run, for when the UI missed events.
func (a *App) RunResults(id string) []RunResult {
	a.rmu.Lock()
	rs := a.runs[id]
	a.rmu.Unlock()
	if rs == nil {
		return nil
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]RunResult(nil), rs.results...)
}

// CancelRun stops a run: servers still waiting never start, running commands are ended.
func (a *App) CancelRun(id string) {
	a.rmu.Lock()
	rs := a.runs[id]
	a.rmu.Unlock()
	if rs != nil {
		rs.cancel()
	}
}
