package main

// Observing connected servers: a sampler reads /proc over the existing SSH connection every few seconds, a
// scheduled rescan re-runs the analysis, and both feed the alert rules in internal/metrics. Nothing here dials a
// server that is not already connected, so no VPN comes up and no production login happens by itself.

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KushalNaral/sshdeck/internal/metrics"
	"github.com/KushalNaral/sshdeck/internal/sshx"
)

const (
	keepMetrics    = 7 * 24 * time.Hour
	sampleTimeout  = 4 * time.Second
	sampleWorkers  = 4
	defaultSampleS = 5
	defaultScanMin = 15
)

// MonitorSettings is what Admin, System shows. Zero thresholds are filled with their defaults on the way out.
type MonitorSettings struct {
	Off           bool `json:"off"`
	SampleSeconds int  `json:"sampleSeconds"`
	ScanMinutes   int  `json:"scanMinutes"`
	DiskWarn      int  `json:"diskWarn"`
	DiskCrit      int  `json:"diskCrit"`
	MemWarn       int  `json:"memWarn"`
	CPUWarn       int  `json:"cpuWarn"`
	NoNotify      bool `json:"noNotify"`
}

func orDefault(v, d int) int {
	if v > 0 {
		return v
	}
	return d
}

func (a *App) GetMonitorSettings() MonitorSettings {
	a.mu.Lock()
	defer a.mu.Unlock()
	s, d := a.settings, metrics.Defaults()
	return MonitorSettings{Off: s.NoMonitor, SampleSeconds: orDefault(s.SampleSeconds, defaultSampleS), ScanMinutes: orDefault(s.ScanMinutes, defaultScanMin),
		DiskWarn: orDefault(s.DiskWarn, d.DiskWarn), DiskCrit: orDefault(s.DiskCrit, d.DiskCrit), MemWarn: orDefault(s.MemWarn, d.MemWarn), CPUWarn: orDefault(s.CPUWarn, d.CPUWarn), NoNotify: s.NoNotify}
}

func (a *App) SetMonitorSettings(m MonitorSettings) error {
	in := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	switch {
	case !in(m.SampleSeconds, 2, 300):
		return errors.New("sample every 2 to 300 seconds")
	case !in(m.ScanMinutes, 1, 1440):
		return errors.New("rescan every 1 to 1440 minutes")
	case !in(m.DiskWarn, 50, 100) || !in(m.DiskCrit, 50, 100) || !in(m.MemWarn, 50, 100) || !in(m.CPUWarn, 50, 100):
		return errors.New("alert levels are percentages between 50 and 100")
	case m.DiskWarn >= m.DiskCrit:
		return errors.New("the disk warning level must be lower than the critical one")
	}
	a.mu.Lock()
	s := &a.settings
	s.NoMonitor, s.SampleSeconds, s.ScanMinutes, s.NoNotify = m.Off, m.SampleSeconds, m.ScanMinutes, m.NoNotify
	s.DiskWarn, s.DiskCrit, s.MemWarn, s.CPUWarn = m.DiskWarn, m.DiskCrit, m.MemWarn, m.CPUWarn
	saved := *s
	a.mu.Unlock()
	a.eng.SetThresholds(metrics.Thresholds{DiskWarn: m.DiskWarn, DiskCrit: m.DiskCrit, MemWarn: m.MemWarn, CPUWarn: m.CPUWarn})
	a.audit("settings.change", "", fmt.Sprintf("monitoring off=%t sample=%ds scan=%dm", m.Off, m.SampleSeconds, m.ScanMinutes))
	return saveSettings(a.settingsPath, saved)
}

// applyThresholds loads the saved alert levels into the engine; startup calls it once.
func (a *App) applyThresholds() {
	m := a.GetMonitorSettings()
	a.eng.SetThresholds(metrics.Thresholds{DiskWarn: m.DiskWarn, DiskCrit: m.DiskCrit, MemWarn: m.MemWarn, CPUWarn: m.CPUWarn})
}

// monitor is the state the sampler keeps between ticks.
type monitor struct {
	mu          sync.Mutex
	parsers     map[string]*metrics.Parser
	unsupported map[string]bool      // servers that did not answer like Linux: not asked again until they reconnect
	lastScan    map[string]time.Time // when each server was last analysed by the schedule
	scanning    map[string]bool
}

func newMonitor() *monitor {
	return &monitor{parsers: map[string]*metrics.Parser{}, unsupported: map[string]bool{}, lastScan: map[string]time.Time{}, scanning: map[string]bool{}}
}

// monitorLoop runs until ctx ends: sample, rescan when due, and once a day drop history older than a week.
func (a *App) monitorLoop(ctx context.Context) {
	a.ms.Prune(a.clock(), keepMetrics)
	lastPrune := a.clock()
	for {
		a.mu.Lock()
		every := time.Duration(orDefault(a.settings.SampleSeconds, defaultSampleS)) * time.Second
		a.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(every):
		}
		now := a.clock()
		a.tick(now)
		if now.Sub(lastPrune) > 24*time.Hour {
			a.ms.Prune(now, keepMetrics)
			lastPrune = now
		}
	}
}

func (a *App) tick(now time.Time) {
	a.mu.Lock()
	off := a.settings.NoMonitor
	a.mu.Unlock()
	if off {
		return
	}
	a.sampleOnce(now)
	a.scanDue(now)
}

type target struct {
	id string
	cl sshx.Client
}

// connected lists the servers whose SSH link is up right now.
func (a *App) connected() []target {
	a.mu.Lock()
	defer a.mu.Unlock()
	var ts []target
	for id, c := range a.conns {
		if c.client == nil {
			continue
		}
		select {
		case <-c.client.Done():
		default:
			ts = append(ts, target{id, c.client})
		}
	}
	return ts
}

// sampleOnce reads every connected server (a few at a time) and publishes the latest numbers.
func (a *App) sampleOnce(now time.Time) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, sampleWorkers)
	for _, t := range a.connected() {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			a.sampleServer(t.id, t.cl, now)
		}()
	}
	wg.Wait()
	a.emit("metrics", a.ms.All())
}

func (a *App) sampleServer(id string, cl sshx.Client, now time.Time) {
	a.mon.mu.Lock()
	if a.mon.unsupported[id] {
		a.mon.mu.Unlock()
		return
	}
	p := a.mon.parsers[id]
	if p == nil {
		p = &metrics.Parser{}
		a.mon.parsers[id] = p
	}
	a.mon.mu.Unlock()

	ctx, cancel := context.WithTimeout(a.ctx, sampleTimeout)
	defer cancel()
	var out strings.Builder
	if err := cl.Exec(ctx, metrics.Cmd, nil, &out); err != nil && out.Len() == 0 {
		return // a slow or broken moment; the next tick tries again
	}
	s, err := p.Parse(out.String(), now)
	if errors.Is(err, metrics.ErrUnsupported) {
		a.mon.mu.Lock()
		a.mon.unsupported[id] = true
		a.mon.mu.Unlock()
		return
	}
	if err != nil {
		return
	}
	a.ms.Add(id, s)
	a.dispatch(a.eng.Sample(id, s, now))
}

// scanDue re-runs the analysis on connected servers whose last scheduled scan is older than ScanMinutes.
// Each scan runs on its own so a slow server does not hold up the sampler.
func (a *App) scanDue(now time.Time) {
	a.mu.Lock()
	every := time.Duration(orDefault(a.settings.ScanMinutes, defaultScanMin)) * time.Minute
	a.mu.Unlock()
	for _, t := range a.connected() {
		a.mon.mu.Lock()
		last := a.mon.lastScan[t.id]
		if a.mon.scanning[t.id] || (!last.IsZero() && now.Sub(last) < every) {
			a.mon.mu.Unlock()
			continue
		}
		a.mon.lastScan[t.id], a.mon.scanning[t.id] = now, true
		a.mon.mu.Unlock()
		go func() {
			defer func() {
				a.mon.mu.Lock()
				delete(a.mon.scanning, t.id)
				a.mon.mu.Unlock()
			}()
			ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
			defer cancel()
			r, err := a.analyzeClient(ctx, a.serverName(t.id), t.cl)
			if err != nil {
				return
			}
			a.emit("analysis:"+t.id, r)
			a.dispatch(a.eng.Scan(t.id, r, now))
		}()
	}
}

// dispatch audits what changed, tells the UI, and pops a desktop notification for new alerts.
func (a *App) dispatch(cs []metrics.Change) {
	if len(cs) == 0 {
		return
	}
	a.mu.Lock()
	quiet := a.settings.NoNotify
	a.mu.Unlock()
	for _, c := range cs {
		name := a.serverName(c.Alert.Server)
		if !c.Fired {
			a.audit("alert.resolve", name, c.Alert.Text)
			continue
		}
		a.audit("alert.fire", name, c.Alert.Text)
		if !quiet && a.notify != nil {
			a.notify("sshdeck: "+name, c.Alert.Text)
		}
	}
	a.emit("alerts", a.eng.List())
}

// linkAlert is called when a dropped connection could not be brought back (lost) and when it is back.
func (a *App) linkAlert(serverID string, lost bool, text string) {
	if !lost {
		a.mon.mu.Lock()
		delete(a.mon.parsers, serverID) // the counters carry on from before the drop; start the rates afresh
		a.mon.mu.Unlock()
	}
	a.dispatch(a.eng.Link(serverID, lost, text, a.clock()))
}

// forgetMonitor closes a disconnected server's alerts and drops what the sampler remembered about it.
func (a *App) forgetMonitor(serverID string) {
	a.dispatch(a.eng.Forget(serverID, a.clock()))
	a.mon.mu.Lock()
	delete(a.mon.parsers, serverID)
	delete(a.mon.unsupported, serverID)
	delete(a.mon.lastScan, serverID)
	a.mon.mu.Unlock()
}

// MetricsLatest is the newest reading of every server that has one.
func (a *App) MetricsLatest() map[string]metrics.Sample { return a.ms.All() }

// MetricsRange returns one server's readings between two unix times, at most 600 points.
func (a *App) MetricsRange(serverID string, from, to int64) []metrics.Sample {
	if to <= from {
		return []metrics.Sample{}
	}
	if to-from > int64(keepMetrics/time.Second)+3600 {
		from = to - int64(keepMetrics/time.Second)
	}
	out := a.ms.Range(serverID, time.Unix(from, 0), time.Unix(to, 0), 600)
	if out == nil {
		return []metrics.Sample{}
	}
	return out
}

// Alerts lists what is firing, then what resolved recently.
func (a *App) Alerts() []metrics.Alert { return a.eng.List() }

// Proc is one row of the busiest-processes table.
type Proc struct {
	PID     int     `json:"pid"`
	User    string  `json:"user"`
	CPU     float64 `json:"cpu"`
	Mem     float64 `json:"mem"`
	Command string  `json:"command"`
}

// TopProcesses reads the busiest processes of a connected server. It is only asked for while the detail view is open.
func (a *App) TopProcesses(serverID string) ([]Proc, error) {
	cl, err := a.live(serverID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
	defer cancel()
	var out strings.Builder
	err = cl.Exec(ctx, "ps -eo pid=,user=,pcpu=,pmem=,comm= --sort=-pcpu | head -n 15", nil, &out)
	if out.Len() == 0 {
		if err == nil {
			err = errors.New("no answer")
		}
		return nil, errors.New("this server's ps could not list processes: " + err.Error())
	}
	return parseProcs(out.String()), nil
}

func parseProcs(out string) []Proc {
	ps := []Proc{}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) < 5 {
			continue
		}
		pid, e1 := strconv.Atoi(f[0])
		cpu, e2 := strconv.ParseFloat(f[2], 64)
		mem, e3 := strconv.ParseFloat(f[3], 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		ps = append(ps, Proc{PID: pid, User: f[1], CPU: cpu, Mem: mem, Command: strings.Join(f[4:], " ")})
	}
	return ps
}
