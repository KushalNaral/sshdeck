package metrics

import (
	"fmt"
	"testing"
	"time"

	"github.com/KushalNaral/sshdeck/internal/probe"
)

var t0 = time.Unix(1_700_000_000, 0)

func at(min int) time.Time { return t0.Add(time.Duration(min) * time.Minute) }

func disk(pct int) Sample { return Sample{Disks: []Disk{{"/", pct}}} }

func fired(cs []Change) []Alert {
	var out []Alert
	for _, c := range cs {
		if c.Fired {
			out = append(out, c.Alert)
		}
	}
	return out
}

func resolved(cs []Change) []Alert {
	var out []Alert
	for _, c := range cs {
		if !c.Fired {
			out = append(out, c.Alert)
		}
	}
	return out
}

func TestDiskFiresOnceEscalatesOnceAndResolvesWithAMargin(t *testing.T) {
	e := NewEngine(Defaults())
	if cs := e.Sample("web", disk(89), at(0)); len(cs) != 0 {
		t.Fatalf("below the line: %+v", cs)
	}
	f := fired(e.Sample("web", disk(91), at(1)))
	if len(f) != 1 || f[0].Severity != "warning" || f[0].Subject != "/" || f[0].Kind != "disk" {
		t.Fatalf("warn: %+v", f)
	}
	if cs := e.Sample("web", disk(92), at(2)); len(cs) != 0 {
		t.Errorf("still true: it must not fire again: %+v", cs)
	}
	if f := fired(e.Sample("web", disk(96), at(3))); len(f) != 1 || f[0].Severity != "critical" {
		t.Errorf("escalation should be announced once: %+v", f)
	}
	if cs := e.Sample("web", disk(96), at(4)); len(cs) != 0 {
		t.Errorf("no repeat at critical: %+v", cs)
	}
	if cs := e.Sample("web", disk(89), at(5)); len(cs) != 0 {
		t.Errorf("89%% is inside the 3 point margin and must not flap: %+v", cs)
	}
	if r := resolved(e.Sample("web", disk(80), at(6))); len(r) != 1 || r[0].Resolved == 0 {
		t.Errorf("resolve: %+v", r)
	}
}

func TestMemoryNeedsToStayHighForTheWindow(t *testing.T) {
	e := NewEngine(Defaults()) // 5 minute window
	hi := Sample{MemPct: 97}
	for m := 0; m < 5; m++ {
		if cs := e.Sample("web", hi, at(m)); len(cs) != 0 {
			t.Fatalf("minute %d fired early: %+v", m, cs)
		}
	}
	if f := fired(e.Sample("web", hi, at(5))); len(f) != 1 || f[0].Kind != "memory" {
		t.Fatalf("after the window: %+v", f)
	}
	e2 := NewEngine(Defaults())
	e2.Sample("web", hi, at(0))
	e2.Sample("web", Sample{MemPct: 50}, at(3)) // a dip starts the clock again
	e2.Sample("web", hi, at(4))
	if cs := e2.Sample("web", hi, at(8)); len(cs) != 0 {
		t.Errorf("only 4 minutes since the dip: %+v", cs)
	}
}

func TestCPUIgnoresReadingsWithoutRatesAndLoadUsesTheCoreCount(t *testing.T) {
	e := NewEngine(Defaults())
	e.Sample("web", Sample{HasRates: true, CPU: 95}, at(0))
	if cs := e.Sample("web", Sample{HasRates: false}, at(3)); len(cs) != 0 {
		t.Errorf("no rates, nothing to judge: %+v", cs)
	}
	if f := fired(e.Sample("web", Sample{HasRates: true, CPU: 95}, at(5))); len(f) != 1 || f[0].Kind != "cpu" {
		t.Errorf("sustained cpu: %+v", f)
	}

	l := NewEngine(Defaults())
	busy := Sample{Cores: 4, Load1: 9}
	l.Sample("db", busy, at(0))
	if f := fired(l.Sample("db", busy, at(5))); len(f) != 1 || f[0].Kind != "load" {
		t.Errorf("load 9 on 4 cores: %+v", f)
	}
	ok := NewEngine(Defaults())
	fine := Sample{Cores: 4, Load1: 7}
	ok.Sample("db", fine, at(0))
	if cs := ok.Sample("db", fine, at(10)); len(cs) != 0 {
		t.Errorf("load 7 on 4 cores is under 2x: %+v", cs)
	}
}

func TestFailedServicesFireAndResolveWithTheScan(t *testing.T) {
	e := NewEngine(Defaults())
	f := fired(e.Scan("web", probe.Report{FailedUnits: []string{"nginx.service"}}, at(0)))
	if len(f) != 1 || f[0].Severity != "critical" || f[0].Subject != "nginx.service" || f[0].Kind != "service" {
		t.Fatalf("%+v", f)
	}
	if cs := e.Scan("web", probe.Report{FailedUnits: []string{"nginx.service"}}, at(15)); len(cs) != 0 {
		t.Errorf("same failure, same alert: %+v", cs)
	}
	if r := resolved(e.Scan("web", probe.Report{}, at(30))); len(r) != 1 {
		t.Errorf("fixed: %+v", r)
	}
}

func ctr(name, status string) probe.Container { return probe.Container{Name: name, Status: status} }

func TestContainersAreJudgedAgainstTheScanBefore(t *testing.T) {
	e := NewEngine(Defaults())
	// the first scan: a stopped container is just how the server is, but a restarting one is already a problem
	f := fired(e.Scan("web", probe.Report{Containers: []probe.Container{ctr("old", "exited"), ctr("loop", "restarting"), ctr("ok", "running")}}, at(0)))
	if len(f) != 1 || f[0].Subject != "loop" {
		t.Fatalf("first scan: %+v", f)
	}
	// next: "ok" stopped since the last scan
	f = fired(e.Scan("web", probe.Report{Containers: []probe.Container{ctr("old", "exited"), ctr("loop", "restarting"), ctr("ok", "exited")}}, at(15)))
	if len(f) != 1 || f[0].Subject != "ok" || f[0].Kind != "container" {
		t.Fatalf("newly stopped: %+v", f)
	}
	// both come back
	r := resolved(e.Scan("web", probe.Report{Containers: []probe.Container{ctr("old", "exited"), ctr("loop", "running"), ctr("ok", "running")}}, at(30)))
	if len(r) != 2 {
		t.Errorf("recovered: %+v", r)
	}
}

func TestAContainerThatIsRemovedResolvesItsAlert(t *testing.T) {
	e := NewEngine(Defaults())
	e.Scan("web", probe.Report{Containers: []probe.Container{ctr("a", "running")}}, at(0))
	e.Scan("web", probe.Report{Containers: []probe.Container{ctr("a", "exited")}}, at(15))
	if r := resolved(e.Scan("web", probe.Report{Containers: []probe.Container{}}, at(30))); len(r) != 1 {
		t.Errorf("gone: %+v", r)
	}
}

func TestLinkLossAndForget(t *testing.T) {
	e := NewEngine(Defaults())
	e.Sample("web", disk(97), at(0))
	if f := fired(e.Link("web", true, "Connection lost: timeout", at(1))); len(f) != 1 || f[0].Kind != "link" || f[0].Severity != "critical" {
		t.Fatalf("%+v", f)
	}
	if cs := e.Link("web", true, "Connection lost: timeout", at(2)); len(cs) != 0 {
		t.Errorf("one alert per outage: %+v", cs)
	}
	if r := resolved(e.Link("web", false, "", at(3))); len(r) != 1 || r[0].Kind != "link" {
		t.Errorf("reconnected: %+v", r)
	}
	// a disconnect makes the metric alerts unknowable, so they close rather than stay red forever
	if r := resolved(e.Forget("web", at(4))); len(r) != 1 || r[0].Kind != "disk" {
		t.Errorf("forget: %+v", r)
	}
}

func TestListShowsActiveFirstAndKeepsAtMost200(t *testing.T) {
	e := NewEngine(Defaults())
	for i := 0; i < 250; i++ {
		id := fmt.Sprintf("s%d", i)
		e.Sample(id, disk(99), at(i))
		e.Sample(id, disk(10), at(i)) // fires then resolves
	}
	e.Sample("live", disk(99), at(300))
	l := e.List()
	if len(l) != 200 {
		t.Fatalf("len = %d", len(l))
	}
	if l[0].Server != "live" || l[0].Resolved != 0 {
		t.Errorf("the firing one comes first: %+v", l[0])
	}
}

func TestThresholdsCanBeChangedAndZeroMeansDefault(t *testing.T) {
	e := NewEngine(Thresholds{DiskWarn: 70, DiskCrit: 80})
	if f := fired(e.Sample("web", disk(75), at(0))); len(f) != 1 || f[0].Severity != "warning" {
		t.Errorf("custom warn: %+v", f)
	}
	e.SetThresholds(Thresholds{}) // back to defaults: 90
	e2 := NewEngine(Thresholds{})
	if cs := e2.Sample("web", disk(85), at(0)); len(cs) != 0 {
		t.Errorf("default is 90: %+v", cs)
	}
}

func TestAlertsAreSeparatePerServer(t *testing.T) {
	e := NewEngine(Defaults())
	a := fired(e.Sample("a", disk(97), at(0)))
	b := fired(e.Sample("b", disk(97), at(0)))
	if len(a) != 1 || len(b) != 1 || a[0].ID == b[0].ID {
		t.Errorf("%+v %+v", a, b)
	}
}
