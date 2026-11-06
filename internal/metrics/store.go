package metrics

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const ringSize = 720 // an hour of 5 second samples

// A Store keeps the last hour of samples in memory and one averaged row per minute on disk, so a week of history
// survives restarts. The files hold numbers under a server id: no host names, users or secrets.
// shortcut: minute rows keep averages only, so a spike shorter than a minute is invisible in the 24 h and 7 d
// views; add a max column when that matters.
type Store struct {
	dir string
	mu  sync.Mutex
	m   map[string]*series
}

type series struct {
	ring   []Sample
	minute int64    // the minute (unix/60) cur belongs to
	cur    []Sample // samples of that minute, not yet written
}

func New(dir string) *Store { return &Store{dir: dir, m: map[string]*series{}} }

var safeID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (s *Store) path(id string) string { return filepath.Join(s.dir, id+".jsonl") }

func (s *Store) get(id string) *series {
	x := s.m[id]
	if x == nil {
		x = &series{minute: -1}
		s.m[id] = x
	}
	return x
}

// Add records a sample. A server id that cannot be a file name is kept in memory only.
func (s *Store) Add(id string, smp Sample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x := s.get(id)
	x.ring = append(x.ring, smp)
	if len(x.ring) > ringSize {
		x.ring = x.ring[len(x.ring)-ringSize:]
	}
	if m := smp.T / 60; m != x.minute {
		s.closeMinute(id, x)
		x.minute = m
	}
	x.cur = append(x.cur, smp)
}

// closeMinute averages the minute that just ended and appends it to the file.
func (s *Store) closeMinute(id string, x *series) {
	if len(x.cur) == 0 {
		return
	}
	row := average(x.cur, x.minute*60)
	x.cur = x.cur[:0]
	if s.dir == "" || !safeID.MatchString(id) { // no folder: memory only
		return
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(s.path(id), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(row)
	f.Write(append(b, '\n'))
}

// Flush writes the minute that is still open, for a clean shutdown.
func (s *Store) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, x := range s.m {
		s.closeMinute(id, x)
	}
}

func (s *Store) Latest(id string) (Sample, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if x := s.m[id]; x != nil && len(x.ring) > 0 {
		return x.ring[len(x.ring)-1], true
	}
	return Sample{}, false
}

// All returns the newest sample of every server that has one.
func (s *Store) All() map[string]Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]Sample{}
	for id, x := range s.m {
		if len(x.ring) > 0 {
			out[id] = x.ring[len(x.ring)-1]
		}
	}
	return out
}

// Range returns the samples between from and to in time order, at most max of them. The last hour comes from
// memory at full resolution; older times come from the minute file. A range that reaches back past what memory
// holds gets file minutes first and the newer samples after them.
func (s *Store) Range(id string, from, to time.Time, max int) []Sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, t := from.Unix(), to.Unix()
	var ring []Sample
	if x := s.m[id]; x != nil {
		for _, p := range x.ring {
			if p.T >= f && p.T <= t {
				ring = append(ring, p)
			}
		}
	}
	var out []Sample
	// minutes cover whatever the ring does not (everything older than its first sample)
	cut := t + 1
	if len(ring) > 0 && to.Sub(from) <= time.Hour {
		cut = ring[0].T
	}
	if len(ring) > 0 && to.Sub(from) > time.Hour {
		cut = 0 // the file is the base; the ring only adds what is newer than its last row
	}
	rows := s.read(id)
	lastRow := int64(0)
	for _, r := range rows {
		if r.T >= f && r.T <= t && (cut == 0 || r.T < cut) {
			out = append(out, r)
			lastRow = r.T
		}
	}
	for _, p := range ring {
		if to.Sub(from) <= time.Hour || p.T >= lastRow+60 {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].T < out[j].T })
	return downsample(out, max)
}

// read returns the file's rows, a later row winning over an earlier one for the same minute.
func (s *Store) read(id string) []Sample {
	if !safeID.MatchString(id) {
		return nil
	}
	f, err := os.Open(s.path(id))
	if err != nil {
		return nil
	}
	defer f.Close()
	var rows []Sample
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		var r Sample
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if n := len(rows); n > 0 && rows[n-1].T == r.T {
			rows[n-1] = r
			continue
		}
		rows = append(rows, r)
	}
	return rows
}

// Prune drops rows older than keep from every file.
func (s *Store) Prune(now time.Time, keep time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ents, _ := os.ReadDir(s.dir)
	cutoff := now.Add(-keep).Unix()
	for _, e := range ents {
		id, ok := strings.CutSuffix(e.Name(), ".jsonl")
		if !ok || !safeID.MatchString(id) {
			continue
		}
		rows := s.read(id)
		kept := rows[:0:0]
		for _, r := range rows {
			if r.T >= cutoff {
				kept = append(kept, r)
			}
		}
		if len(kept) == len(rows) {
			continue
		}
		tmp := s.path(id) + ".tmp"
		f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
		if err != nil {
			continue
		}
		w := bufio.NewWriter(f)
		for _, r := range kept {
			b, _ := json.Marshal(r)
			w.Write(append(b, '\n'))
		}
		w.Flush()
		f.Close()
		os.Rename(tmp, s.path(id))
	}
}

// Delete forgets a server: its memory and its file.
func (s *Store) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, id)
	if safeID.MatchString(id) {
		os.Remove(s.path(id))
	}
}

// average folds samples into one at time t. Rates are averaged over the samples that have them, so a first
// reading without rates does not drag the minute down.
func average(in []Sample, t int64) Sample {
	out := Sample{T: t, Disks: []Disk{}}
	if len(in) == 0 {
		return out
	}
	var r float64
	for _, p := range in {
		out.Load1 += p.Load1
		out.Load5 += p.Load5
		out.Load15 += p.Load15
		out.MemPct += p.MemPct
		out.SwapPct += p.SwapPct
		if p.HasRates {
			r++
			out.CPU += p.CPU
			out.Steal += p.Steal
			out.NetRx += p.NetRx
			out.NetTx += p.NetTx
			out.DiskR += p.DiskR
			out.DiskW += p.DiskW
		}
	}
	n := float64(len(in))
	out.Load1, out.Load5, out.Load15, out.MemPct, out.SwapPct = out.Load1/n, out.Load5/n, out.Load15/n, out.MemPct/n, out.SwapPct/n
	if r > 0 {
		out.HasRates = true
		out.CPU, out.Steal, out.NetRx, out.NetTx, out.DiskR, out.DiskW = out.CPU/r, out.Steal/r, out.NetRx/r, out.NetTx/r, out.DiskR/r, out.DiskW/r
	}
	last := in[len(in)-1]
	out.Cores, out.MemUsedMB, out.MemTotalMB, out.Disks = last.Cores, last.MemUsedMB, last.MemTotalMB, last.Disks
	return out
}

// downsample averages neighbouring points until at most max remain.
func downsample(in []Sample, max int) []Sample {
	if max <= 0 || len(in) <= max {
		return in
	}
	step := (len(in) + max - 1) / max
	out := make([]Sample, 0, max)
	for i := 0; i < len(in); i += step {
		end := i + step
		if end > len(in) {
			end = len(in)
		}
		out = append(out, average(in[i:end], in[(i+end-1)/2].T))
	}
	return out
}
