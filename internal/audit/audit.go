// Package audit keeps an append-only record of what was done through sshdeck: who connected where, which
// container shells were opened, what was changed. It never records passwords, keystrokes or output.
// Each line carries the sha256 of the line before it, so an edited, removed or reordered line shows up in Verify.
// Limit: the very last line has nothing after it yet, so editing only that line cannot be detected.
package audit

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// Entry is one line of the log.
type Entry struct {
	TS      string `json:"ts"`
	Actor   string `json:"actor"`
	Event   string `json:"event"`
	Subject string `json:"subject,omitempty"`
	Detail  string `json:"detail,omitempty"`
	Prev    string `json:"prev"` // sha256 of the previous line, empty for the first ever
}

// Log is safe for concurrent use.
type Log struct {
	mu       sync.Mutex
	path     string
	actor    string
	last     string // hash of the last line written
	clock    func() time.Time
	maxBytes int64
	keep     int // rotated files kept: audit.log.1 ... audit.log.<keep>
}

func hashLine(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Open continues the chain of an existing log, or starts a new one. actor is who the entries are attributed to.
func Open(path, actor string) (*Log, error) {
	l := &Log{path: path, actor: actor, clock: time.Now, maxBytes: 5 << 20, keep: 5}
	f, err := os.Open(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return l, nil
	case err != nil:
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		if line := sc.Bytes(); len(line) > 0 {
			l.last = hashLine(line)
		}
	}
	return l, sc.Err()
}

// clean keeps a detail on one line and short.
func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300])
	}
	return s
}

// Path is where the current log file lives.
func (l *Log) Path() string { return l.path }

// Add appends an entry. A failure to write is returned so the caller can report it.
func (l *Log) Add(event, subject, detail string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if st, err := os.Stat(l.path); err == nil && st.Size() >= l.maxBytes {
		l.rotate()
	}
	b, err := json.Marshal(Entry{TS: l.clock().UTC().Format(time.RFC3339), Actor: l.actor, Event: event, Subject: clean(subject), Detail: clean(detail), Prev: l.last})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	l.last = hashLine(b)
	return f.Sync()
}

// rotate moves audit.log to audit.log.1 and shifts the older ones up. The chain carries on in the new file:
// its first line points at the last line of the rotated one.
func (l *Log) rotate() {
	os.Remove(fmt.Sprintf("%s.%d", l.path, l.keep))
	for i := l.keep - 1; i >= 1; i-- {
		os.Rename(fmt.Sprintf("%s.%d", l.path, i), fmt.Sprintf("%s.%d", l.path, i+1))
	}
	os.Rename(l.path, l.path+".1")
}

func readLines(path string) ([][]byte, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		if len(sc.Bytes()) > 0 {
			out = append(out, append([]byte(nil), sc.Bytes()...))
		}
	}
	return out, sc.Err()
}

// Read returns up to limit entries, newest first, reaching into rotated files when needed.
func (l *Log) Read(limit int) ([]Entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []Entry
	for i := 0; i <= l.keep && len(out) < limit; i++ {
		p := l.path
		if i > 0 {
			p = fmt.Sprintf("%s.%d", l.path, i)
		}
		lines, err := readLines(p)
		if err != nil {
			return nil, err
		}
		for j := len(lines) - 1; j >= 0 && len(out) < limit; j-- {
			var e Entry
			if json.Unmarshal(lines[j], &e) == nil {
				out = append(out, e)
			}
		}
	}
	return out, nil
}

// Check is the result of Verify.
type Check struct {
	OK      bool `json:"ok"`
	Entries int  `json:"entries"`
	BadLine int  `json:"badLine"` // 1-based line where the chain first breaks, 0 when intact
}

// Verify walks one log file and checks that every line points at the hash of the line before it.
// The first line of a file is not checked, because its predecessor may be in a rotated file.
func Verify(path string) Check {
	lines, err := readLines(path)
	if err != nil {
		return Check{BadLine: 1}
	}
	for i, line := range lines {
		var e Entry
		if json.Unmarshal(line, &e) != nil {
			return Check{Entries: len(lines), BadLine: i + 1}
		}
		if i > 0 && e.Prev != hashLine(lines[i-1]) {
			return Check{Entries: len(lines), BadLine: i + 1}
		}
	}
	return Check{OK: true, Entries: len(lines)}
}
