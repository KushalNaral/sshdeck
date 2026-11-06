package metrics

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/KushalNaral/sshdeck/internal/probe"
)

// Alert is one thing that needs a look. It fires once when its condition has held long enough and resolves once
// when the condition ends; it is not sent again while it stays true.
type Alert struct {
	ID       string `json:"id"` // server/kind/subject, so one problem is one alert
	Server   string `json:"server"`
	Kind     string `json:"kind"` // disk, memory, cpu, load, service, container or link
	Subject  string `json:"subject"`
	Severity string `json:"severity"` // warning or critical
	Text     string `json:"text"`
	Since    int64  `json:"since"`
	Resolved int64  `json:"resolved,omitempty"`
}

// Change reports an alert that just fired (also when it got worse) or resolved.
type Change struct {
	Alert Alert
	Fired bool
}

// Thresholds are percentages, except Window. A zero field means its default.
type Thresholds struct {
	DiskWarn, DiskCrit int
	MemWarn, CPUWarn   int
	Window             time.Duration // how long memory, cpu and load must stay high
}

func Defaults() Thresholds {
	return Thresholds{DiskWarn: 90, DiskCrit: 95, MemWarn: 95, CPUWarn: 90, Window: 5 * time.Minute}
}

func (t Thresholds) filled() Thresholds {
	d := Defaults()
	if t.DiskWarn > 0 {
		d.DiskWarn = t.DiskWarn
	}
	if t.DiskCrit > 0 {
		d.DiskCrit = t.DiskCrit
	}
	if t.MemWarn > 0 {
		d.MemWarn = t.MemWarn
	}
	if t.CPUWarn > 0 {
		d.CPUWarn = t.CPUWarn
	}
	if t.Window > 0 {
		d.Window = t.Window
	}
	return d
}

const (
	margin   = 3 // an alert stays until the value is this many points under its line, so it does not flap
	keepLast = 200
)

type Engine struct {
	mu     sync.Mutex
	th     Thresholds
	active map[string]*Alert
	since  map[string]time.Time // when each condition began
	ended  []Alert              // newest first
	ctr    map[string]map[string]string
}

func NewEngine(th Thresholds) *Engine {
	return &Engine{th: th.filled(), active: map[string]*Alert{}, since: map[string]time.Time{}, ctr: map[string]map[string]string{}}
}

func (e *Engine) SetThresholds(th Thresholds) {
	e.mu.Lock()
	e.th = th.filled()
	e.mu.Unlock()
}

// judge moves one alert along. want starts it (after window), keep lets an active one stay.
func (e *Engine) judge(server, kind, subject, sev, text string, want, keep bool, window time.Duration, now time.Time) []Change {
	key := server + "/" + kind + "/" + subject
	if a := e.active[key]; a != nil {
		if keep {
			if sev == "critical" && a.Severity != "critical" { // it got worse: say so once
				a.Severity, a.Text = sev, text
				return []Change{{*a, true}}
			}
			return nil
		}
		return e.resolve(key, now)
	}
	if !want {
		delete(e.since, key)
		return nil
	}
	if _, ok := e.since[key]; !ok {
		e.since[key] = now
	}
	if now.Sub(e.since[key]) < window {
		return nil
	}
	delete(e.since, key)
	a := &Alert{ID: key, Server: server, Kind: kind, Subject: subject, Severity: sev, Text: text, Since: now.Unix()}
	e.active[key] = a
	return []Change{{*a, true}}
}

func (e *Engine) resolve(key string, now time.Time) []Change {
	a := e.active[key]
	if a == nil {
		return nil
	}
	delete(e.active, key)
	delete(e.since, key)
	a.Resolved = now.Unix()
	e.ended = append([]Alert{*a}, e.ended...)
	if len(e.ended) > keepLast {
		e.ended = e.ended[:keepLast]
	}
	return []Change{{*a, false}}
}

// sweep resolves the active alerts of a kind on a server whose subject is no longer present.
func (e *Engine) sweep(server, kind string, present map[string]bool, now time.Time) []Change {
	var out []Change
	for key, a := range e.active {
		if a.Server == server && a.Kind == kind && !present[a.Subject] {
			out = append(out, e.resolve(key, now)...)
		}
	}
	return out
}

// Sample judges a reading: disks at once, memory, cpu and load once they have stayed high for the window.
func (e *Engine) Sample(server string, s Sample, now time.Time) []Change {
	e.mu.Lock()
	defer e.mu.Unlock()
	th := e.th
	var out []Change
	mounts := map[string]bool{}
	for _, d := range s.Disks {
		mounts[d.Mount] = true
		sev := "warning"
		if d.UsedPct >= th.DiskCrit {
			sev = "critical"
		}
		out = append(out, e.judge(server, "disk", d.Mount, sev, fmt.Sprintf("Disk %s at %d%%", d.Mount, d.UsedPct),
			d.UsedPct >= th.DiskWarn, d.UsedPct >= th.DiskWarn-margin, 0, now)...)
	}
	if len(s.Disks) > 0 {
		out = append(out, e.sweep(server, "disk", mounts, now)...)
	}
	out = append(out, e.judge(server, "memory", "", "warning", fmt.Sprintf("Memory at %.0f%%", s.MemPct),
		s.MemPct >= float64(th.MemWarn), s.MemPct >= float64(th.MemWarn-margin), th.Window, now)...)
	if s.HasRates {
		out = append(out, e.judge(server, "cpu", "", "warning", fmt.Sprintf("CPU at %.0f%%", s.CPU),
			s.CPU >= float64(th.CPUWarn), s.CPU >= float64(th.CPUWarn-margin), th.Window, now)...)
	}
	if s.Cores > 0 {
		c := float64(s.Cores)
		out = append(out, e.judge(server, "load", "", "warning", fmt.Sprintf("Load %.1f on %d cores", s.Load1, s.Cores),
			s.Load1 > 2*c, s.Load1 > 1.5*c, th.Window, now)...)
	}
	return out
}

// Scan judges a full analysis: a failed service is critical; a container is flagged when it is restarting, or has
// stopped since the scan before (a container that was already stopped is just how the server is).
func (e *Engine) Scan(server string, r probe.Report, now time.Time) []Change {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []Change
	units := map[string]bool{}
	for _, u := range r.FailedUnits {
		units[u] = true
		out = append(out, e.judge(server, "service", u, "critical", "Service "+u+" failed", true, true, 0, now)...)
	}
	out = append(out, e.sweep(server, "service", units, now)...)

	prev := e.ctr[server]
	cur := map[string]string{}
	names := map[string]bool{}
	for _, c := range r.Containers {
		cur[c.Name] = c.Status
		names[c.Name] = true
		restarting := c.Status == "restarting"
		stopped := c.Status == "exited"
		text := "Container " + c.Name + " is restarting"
		if stopped {
			text = "Container " + c.Name + " stopped"
		}
		out = append(out, e.judge(server, "container", c.Name, "warning", text,
			restarting || (stopped && prev[c.Name] == "running"), restarting || stopped, 0, now)...)
	}
	out = append(out, e.sweep(server, "container", names, now)...)
	e.ctr[server] = cur
	return out
}

// Link raises or clears the alert for a lost connection.
func (e *Engine) Link(server string, lost bool, text string, now time.Time) []Change {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.judge(server, "link", "", "critical", text, lost, lost, 0, now)
}

// Forget closes everything for a server that was disconnected: what its readings said can no longer be checked.
func (e *Engine) Forget(server string, now time.Time) []Change {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []Change
	for key, a := range e.active {
		if a.Server == server {
			out = append(out, e.resolve(key, now)...)
		}
	}
	for key := range e.since {
		if len(key) > len(server) && key[:len(server)+1] == server+"/" {
			delete(e.since, key)
		}
	}
	delete(e.ctr, server)
	return out
}

// List is the alerts that are firing (newest first) followed by the ones that resolved, at most 200 in all.
func (e *Engine) List() []Alert {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Alert, 0, len(e.active)+len(e.ended))
	for _, a := range e.active {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Since > out[j].Since || (out[i].Since == out[j].Since && out[i].ID < out[j].ID)
	})
	out = append(out, e.ended...)
	if len(out) > keepLast {
		out = out[:keepLast]
	}
	return out
}
