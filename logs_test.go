package main

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type sink struct {
	mu    sync.Mutex
	lines []string
	ended []any
}

func (s *sink) emit(name string, data ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case strings.HasSuffix(name, ":end"):
		s.ended = append(s.ended, data...)
	case strings.HasPrefix(name, "log:"):
		s.lines = append(s.lines, data[0].([]string)...)
	}
}
func (s *sink) got() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.lines...)
}
func (s *sink) over() bool { s.mu.Lock(); defer s.mu.Unlock(); return len(s.ended) > 0 }

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()
	for i := 0; i < 300; i++ {
		if ok() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("timed out waiting for " + what)
}

func TestLineBatcherHoldsAPartialLineUntilItsNewline(t *testing.T) {
	var mu sync.Mutex
	var got []string
	b := newBatcher(func(l []string) { mu.Lock(); got = append(got, l...); mu.Unlock() })
	b.Write([]byte("one\r\ntwo\npart"))
	time.Sleep(250 * time.Millisecond)
	mu.Lock()
	if len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("whole lines only, without the carriage return: %q", got)
	}
	mu.Unlock()
	b.Write([]byte("ial\n"))
	b.Write([]byte("tail without newline"))
	b.Close()
	mu.Lock()
	defer mu.Unlock()
	if len(got) != 4 || got[2] != "partial" || got[3] != "tail without newline" {
		t.Fatalf("%q", got)
	}
}

func TestStartLogStreamsAndStopEnds(t *testing.T) {
	a, ids := rigRun(t, 1, nil)
	s := &sink{}
	a.emit = s.emit
	var cmd atomic.Value
	a.liveClientFor(ids[0]).exec = func(ctx context.Context, c string, _ io.Reader, out io.Writer) error {
		cmd.Store(c)
		io.WriteString(out, "first\nsecond\n")
		<-ctx.Done()
		return ctx.Err()
	}
	id, err := a.StartLog(ids[0], "journal", "nginx.service")
	if err != nil {
		t.Fatal(err)
	}
	eventually(t, "lines", func() bool { return len(s.got()) == 2 })
	if c, _ := cmd.Load().(string); !strings.Contains(c, "journalctl -f -n 200") || !strings.Contains(c, "'nginx.service'") {
		t.Fatalf("command %q", c)
	}
	a.StopLog(id)
	eventually(t, "the end event", s.over)
}

func TestLogSourcesAreChecked(t *testing.T) {
	a, ids := rigRun(t, 1, nil)
	for _, c := range []struct{ kind, arg string }{
		{"journal", "x; reboot"},
		{"container", "web; id"},
		{"container", ""},
		{"file", ""},
		{"nope", "x"},
	} {
		if _, err := a.StartLog(ids[0], c.kind, c.arg); err == nil {
			t.Errorf("%s %q should be refused", c.kind, c.arg)
		}
	}
	if _, err := a.StartLog("direct", "journal", ""); err == nil {
		t.Error("a server that is not connected should be refused")
	}
}

func TestLogFilePathsAndContainersAreQuoted(t *testing.T) {
	a, ids := rigRun(t, 1, nil)
	var mu sync.Mutex
	var cmds []string
	a.liveClientFor(ids[0]).exec = func(ctx context.Context, c string, _ io.Reader, _ io.Writer) error {
		mu.Lock()
		cmds = append(cmds, c)
		mu.Unlock()
		return nil
	}
	for _, c := range []struct{ target, kind, arg string }{
		{ids[0], "file", "/var/log/it's here.log"},
		{ids[0] + "@web", "file", "/var/log/x.log"},
		{ids[0], "container", "web"},
	} {
		if _, err := a.StartLog(c.target, c.kind, c.arg); err != nil {
			t.Fatalf("%+v: %v", c, err)
		}
	}
	eventually(t, "three commands", func() bool { mu.Lock(); defer mu.Unlock(); return len(cmds) == 3 })
	mu.Lock()
	defer mu.Unlock()
	joined := strings.Join(cmds, "\n")
	for _, want := range []string{`tail -n 200 -F -- '/var/log/it'\''s here.log'`, "docker exec web sh -c", "docker logs -f --tail 200 web"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in\n%s", want, joined)
		}
	}
}

func TestDisconnectStopsLogs(t *testing.T) {
	a, ids := rigRun(t, 1, nil)
	s := &sink{}
	a.emit = s.emit
	a.liveClientFor(ids[0]).exec = func(ctx context.Context, c string, _ io.Reader, out io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	}
	if _, err := a.StartLog(ids[0], "journal", ""); err != nil {
		t.Fatal(err)
	}
	a.Disconnect(ids[0])
	eventually(t, "the end event", s.over)
}
