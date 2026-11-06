package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// shellExec runs the command in a local shell, which is what the real server would do with it.
func shellExec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer) error {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	c.Stdin, c.Stdout = stdin, stdout
	return c.Run()
}

// rigRun connects n direct servers whose commands run in a local shell.
func rigRun(t *testing.T, n int, exec func(context.Context, string, io.Reader, io.Writer) error) (*App, []string) {
	t.Helper()
	a, _, _ := rig(t, okOpen(newShell()))
	var ids []string
	for i := 0; i < n; i++ {
		sv, err := a.st.SaveServer(store.Server{Name: fmt.Sprintf("S%d", i), Host: "h", Port: 22, User: "u", Auth: "password"}, "pw")
		if err != nil {
			t.Fatal(err)
		}
		if r := a.Connect(sv.ID); !r.OK {
			t.Fatalf("%+v", r)
		}
		a.liveClientFor(sv.ID).exec = exec
		ids = append(ids, sv.ID)
	}
	return a, ids
}

func waitRun(t *testing.T, a *App, id string) []RunResult {
	t.Helper()
	for i := 0; i < 500; i++ {
		rs := a.RunResults(id)
		done := true
		for _, r := range rs {
			if r.State == "waiting" || r.State == "running" {
				done = false
			}
		}
		if done {
			return rs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the run never finished")
	return nil
}

func TestRunReportsOutputAndExitCodePerServer(t *testing.T) {
	a, ids := rigRun(t, 2, shellExec)
	id, err := a.RunOn(ids, "echo hello; echo oops >&2; exit 3")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range waitRun(t, a, id) {
		if r.State != "failed" || r.Exit != 3 {
			t.Errorf("a non-zero exit is a failure with its code: %+v", r)
		}
		if !strings.Contains(r.Output, "hello") || !strings.Contains(r.Output, "oops") || strings.Contains(r.Output, "sshdeck-exit") {
			t.Errorf("stdout and stderr, without our marker: %q", r.Output)
		}
	}
	id, _ = a.RunOn(ids, "true")
	for _, r := range waitRun(t, a, id) {
		if r.State != "done" || r.Exit != 0 {
			t.Errorf("%+v", r)
		}
	}
}

func TestRunSkipsServersThatAreNotConnected(t *testing.T) {
	a, ids := rigRun(t, 1, shellExec)
	id, err := a.RunOn(append(ids, "direct"), "true")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]RunResult{}
	for _, r := range waitRun(t, a, id) {
		got[r.Server] = r
	}
	if got["direct"].State != "skipped" || !strings.Contains(got["direct"].Error, "Connect") {
		t.Errorf("%+v", got["direct"])
	}
	if a.conns["direct"] != nil {
		t.Error("a run must never dial a server by itself")
	}
}

func TestRunKeepsTheEndOfLongOutput(t *testing.T) {
	a, ids := rigRun(t, 1, shellExec)
	id, _ := a.RunOn(ids, "head -c 400000 /dev/zero | tr '\\0' x; echo THE-END")
	r := waitRun(t, a, id)[0]
	if len(r.Output) > runCap+64 || !r.Truncated || !strings.Contains(r.Output, "THE-END") || r.Exit != 0 {
		t.Fatalf("len %d truncated %v exit %d", len(r.Output), r.Truncated, r.Exit)
	}
}

func TestRunLimitsHowManyServersWorkAtOnce(t *testing.T) {
	var now, peak atomic.Int32
	release := make(chan struct{})
	slow := func(ctx context.Context, cmd string, _ io.Reader, out io.Writer) error {
		n := now.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		<-release
		now.Add(-1)
		return nil
	}
	a, ids := rigRun(t, 7, slow)
	id, _ := a.RunOn(ids, "true")
	time.Sleep(300 * time.Millisecond)
	close(release)
	waitRun(t, a, id)
	if peak.Load() != runWorkers {
		t.Fatalf("peak %d, want %d", peak.Load(), runWorkers)
	}
}

func TestCancelRunStopsWhatIsStillGoing(t *testing.T) {
	var started sync.WaitGroup
	started.Add(1)
	hang := func(ctx context.Context, _ string, _ io.Reader, _ io.Writer) error {
		started.Done()
		<-ctx.Done()
		return ctx.Err()
	}
	a, ids := rigRun(t, 1, hang)
	id, _ := a.RunOn(ids, "sleep 100")
	started.Wait()
	a.CancelRun(id)
	if r := waitRun(t, a, id)[0]; r.State != "cancelled" {
		t.Fatalf("%+v", r)
	}
}

func TestRunEventsEndWithTheFinalState(t *testing.T) {
	a, ids := rigRun(t, 1, shellExec)
	var mu sync.Mutex
	var last RunResult
	a.emit = func(name string, data ...any) {
		if strings.HasPrefix(name, "run:") {
			mu.Lock()
			last = data[0].(RunResult)
			mu.Unlock()
		}
	}
	id, _ := a.RunOn(ids, "echo done-text")
	waitRun(t, a, id)
	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if last.State != "done" || !strings.Contains(last.Output, "done-text") {
		t.Fatalf("%+v", last)
	}
	_ = bytes.MinRead
}

func TestRunRefusesAnEmptyCommand(t *testing.T) {
	a, ids := rigRun(t, 1, shellExec)
	if _, err := a.RunOn(ids, "  \n"); err == nil {
		t.Error("an empty command should be refused")
	}
	if _, err := a.RunOn(nil, "true"); err == nil {
		t.Error("no servers should be refused")
	}
}
