package main

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// lineBatcher turns a byte stream into whole lines and hands them on at most ten times a second.
type lineBatcher struct {
	mu    sync.Mutex
	buf   []byte
	lines []string
	emit  func([]string)
	stop  chan struct{}
	done  chan struct{}
}

func newBatcher(emit func([]string)) *lineBatcher {
	b := &lineBatcher{emit: emit, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(b.done)
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				b.flush()
			case <-b.stop:
				return
			}
		}
	}()
	return b
}

func (b *lineBatcher) Write(p []byte) (int, error) {
	b.mu.Lock()
	b.buf = append(b.buf, p...)
	for {
		i := strings.IndexByte(string(b.buf), '\n')
		if i < 0 {
			break
		}
		b.lines = append(b.lines, strings.TrimSuffix(string(b.buf[:i]), "\r"))
		b.buf = b.buf[i+1:]
	}
	if len(b.buf) > 64*1024 { // a line that never ends: show it rather than hold it for ever
		b.lines, b.buf = append(b.lines, string(b.buf)), nil
	}
	b.mu.Unlock()
	return len(p), nil
}

func (b *lineBatcher) flush() {
	b.mu.Lock()
	l := b.lines
	b.lines = nil
	b.mu.Unlock()
	if len(l) > 0 {
		b.emit(l)
	}
}

// Close sends what is left, including a last line that had no newline.
func (b *lineBatcher) Close() {
	close(b.stop)
	<-b.done
	b.mu.Lock()
	if len(b.buf) > 0 {
		b.lines, b.buf = append(b.lines, string(b.buf)), nil
	}
	b.mu.Unlock()
	b.flush()
}

var unitRe = regexp.MustCompile(`^[A-Za-z0-9@:._-]+$`)

// StartLog follows a log and streams it as "log:<id>" events (batches of lines); "log:<id>:end" says why it stopped.
// kind is "journal" (arg: an optional unit), "file" (arg: a path, on the server or in the container of the target)
// or "container" (arg: a container's name, for its docker logs). It returns the id to pass to StopLog.
func (a *App) StartLog(target, kind, arg string) (string, error) {
	sid, ctr := splitTarget(target)
	cl, err := a.live(sid)
	if err != nil {
		return "", err
	}
	var cmd string
	switch kind {
	case "journal":
		if arg != "" && !unitRe.MatchString(arg) {
			return "", errors.New("that is not a unit name")
		}
		cmd = "journalctl -f -n 200 --no-pager"
		if arg != "" {
			cmd += " -u " + shq(arg)
		}
	case "container":
		if !containerRe.MatchString(arg) {
			return "", errors.New("that is not a container name")
		}
		cmd = "(docker ps >/dev/null 2>&1 && docker logs -f --tail 200 " + arg + " 2>&1) || sudo -n docker logs -f --tail 200 " + arg + " 2>&1"
	case "file":
		if arg == "" {
			return "", errors.New("pick a file to follow")
		}
		if ctr == "" {
			cmd = "tail -n 200 -F -- " + shq(arg) + " 2>&1"
		} else {
			cmd = dockerFS{ex: cl, name: ctr}.cmd(false, `tail -n 200 -F -- "$1" 2>&1`, arg)
		}
	default:
		return "", errors.New("unknown log source")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.lmu.Lock()
	a.logSeq++
	id := "log" + strconv.Itoa(a.logSeq)
	a.logs[id] = logRun{server: sid, cancel: cancel}
	a.lmu.Unlock()
	a.audit("log.start", a.serverName(sid), kind+" "+arg)
	go func() {
		b := newBatcher(func(l []string) { a.emit("log:"+id, l) })
		err := cl.Exec(ctx, cmd, nil, b)
		b.Close()
		why := ""
		if err != nil && ctx.Err() == nil {
			why = err.Error()
		}
		cancel()
		a.lmu.Lock()
		delete(a.logs, id)
		a.lmu.Unlock()
		a.emit("log:"+id+":end", why)
	}()
	return id, nil
}

// StopLog ends a followed log.
func (a *App) StopLog(id string) {
	a.lmu.Lock()
	r, ok := a.logs[id]
	a.lmu.Unlock()
	if ok {
		r.cancel()
	}
}

func (a *App) stopLogs(serverID string) {
	a.lmu.Lock()
	var cancels []context.CancelFunc
	for _, r := range a.logs {
		if r.server == serverID {
			cancels = append(cancels, r.cancel)
		}
	}
	a.lmu.Unlock()
	for _, c := range cancels {
		c()
	}
}

type logRun struct {
	server string
	cancel context.CancelFunc
}
