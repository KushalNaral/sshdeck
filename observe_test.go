package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/metrics"
)

func procOut(diskPct int) string {
	return fmt.Sprintf("@@stat\ncpu  1000 0 500 8000 100 0 0 0 0 0\ncpu0 1 0 1 1 0 0 0 0 0 0\n@@meminfo\nMemTotal: 8000000 kB\nMemAvailable: 4000000 kB\n"+
		"@@loadavg\n0.1 0.1 0.1 1/1 1\n@@net/dev\n@@diskstats\n@@df\nFilesystem 1024-blocks Used Available Capacity Mounted on\n/dev/sda1 1000 1 1 %d%% /\n", diskPct)
}

// monitored is a connected server that answers metrics.Cmd with whatever disk() says, and counts the asks.
type monitored struct {
	mu    sync.Mutex
	asks  int
	out   string
	disk  int
	notes []string
}

func (m *monitored) exec(_ context.Context, cmd string, _ io.Reader, w io.Writer) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if cmd != metrics.Cmd {
		return fmt.Errorf("unexpected command %q", cmd)
	}
	m.asks++
	if m.out != "" {
		io.WriteString(w, m.out)
		return nil
	}
	io.WriteString(w, procOut(m.disk))
	return nil
}

func (m *monitored) setDisk(p int) { m.mu.Lock(); m.disk = p; m.mu.Unlock() }
func (m *monitored) count() int    { m.mu.Lock(); defer m.mu.Unlock(); return m.asks }

func rigMonitor(t *testing.T) (*App, *monitored, *events, *trail, string) {
	t.Helper()
	m := &monitored{disk: 40}
	a, ids := rigRun(t, 1, m.exec)
	ev, tr := &events{}, &trail{}
	a.emit, a.audit = ev.emit, tr.add
	a.ms = metrics.New(t.TempDir())
	a.notify = func(title, body string) {
		m.mu.Lock()
		m.notes = append(m.notes, title+"|"+body)
		m.mu.Unlock()
	}
	return a, m, ev, tr, ids[0]
}

func (m *monitored) notified() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.notes...)
}

func TestSamplerReadsConnectedServersOnly(t *testing.T) {
	a, m, ev, _, id := rigMonitor(t)
	a.sampleOnce(time.Unix(1000, 0))
	if m.count() != 1 {
		t.Errorf("asks = %d", m.count())
	}
	got := a.MetricsLatest()
	if len(got) != 1 || got[id].T != 1000 {
		t.Errorf("latest: %+v", got)
	}
	if _, ok := got["direct"]; ok {
		t.Error("a server that is not connected must not be dialled or sampled")
	}
	if !ev.has("metrics") {
		t.Error("the UI is told through a metrics event")
	}
}

func TestDiskAlertFiresOnceNotifiesOnceAndResolves(t *testing.T) {
	a, m, ev, tr, id := rigMonitor(t)
	m.setDisk(96)
	a.sampleOnce(time.Unix(1000, 0))
	al := a.Alerts()
	if len(al) != 1 || al[0].Kind != "disk" || al[0].Severity != "critical" || al[0].Server != id || al[0].Resolved != 0 {
		t.Fatalf("%+v", al)
	}
	if n := m.notified(); len(n) != 1 || !strings.Contains(n[0], "Disk / at 96%") || !strings.Contains(n[0], "S0") {
		t.Errorf("notifications: %v", n)
	}
	if tr.find("alert.fire") == nil || !ev.has("alerts") {
		t.Error("firing is audited and sent to the UI")
	}
	a.sampleOnce(time.Unix(1005, 0))
	if len(m.notified()) != 1 {
		t.Error("an alert that stays true must not notify again")
	}
	m.setDisk(50)
	a.sampleOnce(time.Unix(1010, 0))
	if al = a.Alerts(); len(al) != 1 || al[0].Resolved == 0 {
		t.Errorf("resolved: %+v", al)
	}
	if tr.find("alert.resolve") == nil {
		t.Error("resolving is audited")
	}
	if len(m.notified()) != 1 {
		t.Error("resolving is quiet")
	}
}

func TestNotificationsCanBeTurnedOff(t *testing.T) {
	a, m, _, _, _ := rigMonitor(t)
	a.settings.NoNotify = true
	m.setDisk(99)
	a.sampleOnce(time.Unix(1000, 0))
	if len(m.notified()) != 0 || len(a.Alerts()) != 1 {
		t.Error("the alert still exists, only the popup is off")
	}
}

func TestAServerThatIsNotLinuxIsLeftAlone(t *testing.T) {
	a, m, _, _, _ := rigMonitor(t)
	m.out = "@@stat\ncat: /proc/stat: No such file or directory\n"
	a.sampleOnce(time.Unix(1000, 0))
	a.sampleOnce(time.Unix(1005, 0))
	if m.count() != 1 {
		t.Errorf("asked %d times; an unsupported server should not be asked again", m.count())
	}
}

func TestDisconnectClosesTheServersAlerts(t *testing.T) {
	a, m, _, _, id := rigMonitor(t)
	m.setDisk(97)
	a.sampleOnce(time.Unix(1000, 0))
	a.Disconnect(id)
	if al := a.Alerts(); len(al) != 1 || al[0].Resolved == 0 {
		t.Errorf("a disconnected server cannot be checked, so its alerts close: %+v", al)
	}
}

func TestLostLinkRaisesAnAlertUntilItIsBack(t *testing.T) {
	a, _, _, _, id := rigMonitor(t)
	a.linkAlert(id, true, "Connection lost: timeout")
	if al := a.Alerts(); len(al) != 1 || al[0].Kind != "link" || al[0].Resolved != 0 {
		t.Fatalf("%+v", al)
	}
	a.linkAlert(id, false, "")
	if al := a.Alerts(); al[0].Resolved == 0 {
		t.Errorf("%+v", al)
	}
}

func TestScheduledScanPublishesTheReportAndRaisesServiceAlerts(t *testing.T) {
	a, _, ev, _, id := rigMonitor(t)
	a.liveClientFor(id).runf = func(cmd string) (string, error) {
		if strings.Contains(cmd, "systemctl --failed") {
			return "nginx.service loaded failed failed A web server\n", nil
		}
		return "x\n", nil
	}
	a.scanDue(time.Unix(1000, 0))
	eventually(t, "the analysis event", func() bool { return ev.has("analysis:" + id) })
	eventually(t, "the service alert", func() bool { return len(a.Alerts()) == 1 })
	if al := a.Alerts()[0]; al.Kind != "service" || al.Subject != "nginx.service" {
		t.Errorf("%+v", al)
	}
	before := len(a.liveClientFor(id).runs)
	a.scanDue(time.Unix(1000+60, 0)) // a minute later: not due yet
	time.Sleep(50 * time.Millisecond)
	if len(a.liveClientFor(id).runs) != before {
		t.Error("a scan is only repeated every ScanMinutes")
	}
}

func TestMetricsFileGoesWhenTheServerIsDeleted(t *testing.T) {
	a, _, _, _, id := rigMonitor(t)
	dir := t.TempDir()
	a.ms = metrics.New(dir)
	a.sampleOnce(time.Unix(6000, 0))
	a.sampleOnce(time.Unix(6070, 0)) // a new minute closes the first and writes it
	file := filepath.Join(dir, id+".jsonl")
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("the minute should be on disk: %v", err)
	}
	if err := a.DeleteServer(id); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("deleting a server must delete its history")
	}
}

func TestMonitorSettingsHaveDefaultsAndAreChecked(t *testing.T) {
	a, _, _, _, _ := rigMonitor(t)
	d := a.GetMonitorSettings()
	if d.Off || d.SampleSeconds != 5 || d.ScanMinutes != 15 || d.DiskWarn != 90 || d.DiskCrit != 95 || d.MemWarn != 95 || d.CPUWarn != 90 {
		t.Errorf("defaults: %+v", d)
	}
	bad := d
	bad.SampleSeconds = 1
	if a.SetMonitorSettings(bad) == nil {
		t.Error("sampling faster than every 2 seconds is refused")
	}
	bad = d
	bad.DiskWarn, bad.DiskCrit = 95, 90
	if a.SetMonitorSettings(bad) == nil {
		t.Error("the warning line must be under the critical one")
	}
	ok := d
	ok.DiskWarn, ok.DiskCrit, ok.SampleSeconds = 70, 80, 10
	if err := a.SetMonitorSettings(ok); err != nil {
		t.Fatal(err)
	}
	if g := a.GetMonitorSettings(); g.DiskWarn != 70 || g.SampleSeconds != 10 {
		t.Errorf("%+v", g)
	}
	a2, m, _, _, _ := rigMonitor(t)
	_ = a2.SetMonitorSettings(ok)
	m.setDisk(75)
	a2.sampleOnce(time.Unix(1000, 0))
	if al := a2.Alerts(); len(al) != 1 || al[0].Severity != "warning" {
		t.Errorf("new lines must apply at once: %+v", al)
	}
}

func TestTurningMonitoringOffStopsTheSampler(t *testing.T) {
	a, m, _, _, _ := rigMonitor(t)
	s := a.GetMonitorSettings()
	s.Off = true
	if err := a.SetMonitorSettings(s); err != nil {
		t.Fatal(err)
	}
	a.tick(time.Unix(1000, 0))
	if m.count() != 0 {
		t.Error("nothing should be sampled while monitoring is off")
	}
}

func TestParseProcs(t *testing.T) {
	out := "  812 root      45.2  1.3 postgres: writer\n 1 root 0.0 0.1 systemd\nbad line\n"
	ps := parseProcs(out)
	if len(ps) != 2 || ps[0].PID != 812 || ps[0].User != "root" || ps[0].CPU != 45.2 || ps[0].Mem != 1.3 || ps[0].Command != "postgres: writer" {
		t.Errorf("%+v", ps)
	}
}

func TestTopProcessesNeedsAConnection(t *testing.T) {
	a, _, _, _, _ := rigMonitor(t)
	if _, err := a.TopProcesses("ghost"); err == nil {
		t.Error("an unconnected server must not be dialled")
	}
}

func TestParseProcsAgainstThisMachinesPs(t *testing.T) {
	out, err := exec.Command("sh", "-c", "ps -eo pid=,user=,pcpu=,pmem=,comm= --sort=-pcpu | head -n 15").Output()
	if err != nil {
		t.Skip("no procps ps here")
	}
	ps := parseProcs(string(out))
	if len(ps) < 5 {
		t.Fatalf("only %d rows parsed from:\n%s", len(ps), out)
	}
	for _, p := range ps {
		if p.PID < 1 || p.User == "" || p.Command == "" {
			t.Errorf("bad row %+v", p)
		}
	}
}
