package metrics

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"
)

// fixture builds what Cmd prints. cpu is user, system, idle, iowait, steal.
type fixture struct {
	cpu                 [5]int
	cores               int
	memTotal, memAvail  int // kB
	swapTotal, swapFree int // kB
	load                string
	rx, tx              int // eth0 bytes
	sectorsR, sectorsW  int // sda, a whole disk
	df                  string
}

func (f fixture) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "@@stat\ncpu  %d 0 %d %d %d 0 0 %d 0 0\n", f.cpu[0], f.cpu[1], f.cpu[2], f.cpu[3], f.cpu[4])
	for i := 0; i < f.cores; i++ {
		fmt.Fprintf(&b, "cpu%d  1 0 1 1 0 0 0 0 0 0\n", i)
	}
	b.WriteString("intr 1 2 3\nctxt 99\nbtime 1700000000\n")
	fmt.Fprintf(&b, "@@meminfo\nMemTotal: %d kB\nMemFree: 1 kB\nMemAvailable: %d kB\nBuffers: 1 kB\nSwapTotal: %d kB\nSwapFree: %d kB\n", f.memTotal, f.memAvail, f.swapTotal, f.swapFree)
	fmt.Fprintf(&b, "@@loadavg\n%s 1/467 12345\n", f.load)
	b.WriteString("@@net/dev\nInter-|   Receive                                                |  Transmit\n face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed\n")
	b.WriteString("    lo: 999999999 1 0 0 0 0 0 0 999999999 1 0 0 0 0 0 0\n")
	b.WriteString("docker0: 888888888 1 0 0 0 0 0 0 888888888 1 0 0 0 0 0 0\n")
	b.WriteString("veth1a2: 777777777 1 0 0 0 0 0 0 777777777 1 0 0 0 0 0 0\n")
	fmt.Fprintf(&b, "  eth0: %d 10 0 0 0 0 0 0 %d 10 0 0 0 0 0 0\n", f.rx, f.tx)
	b.WriteString("@@diskstats\n")
	b.WriteString("   7       0 loop0 5 0 5000000 0 5 0 5000000 0 0 0 0\n")
	fmt.Fprintf(&b, "   8       0 sda 1 0 %d 0 1 0 %d 0 0 0 0\n", f.sectorsR, f.sectorsW)
	b.WriteString("   8       1 sda1 1 0 66666666 0 1 0 66666666 0 0 0 0\n")
	b.WriteString("@@df\n")
	b.WriteString(f.df)
	return b.String()
}

const df = `Filesystem     1024-blocks    Used Available Capacity Mounted on
/dev/sda1             1000     900       100      90% /
/dev/sda1             1000     900       100      90% /etc/hosts
/dev/sdb1             1000     500       500      50% /data
`

func base() fixture {
	return fixture{cpu: [5]int{1000, 500, 8000, 100, 0}, cores: 4, memTotal: 8000000, memAvail: 2000000,
		swapTotal: 1000000, swapFree: 750000, load: "0.52 0.58 0.59", rx: 1000, tx: 2000, sectorsR: 100, sectorsW: 0, df: df}
}

func near(a, b float64) bool { return math.Abs(a-b) < 0.05 }

func TestFirstSampleHasNoRatesButHasLevels(t *testing.T) {
	var p Parser
	s, err := p.Parse(base().String(), time.Unix(1000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if s.HasRates {
		t.Error("one reading cannot give a rate")
	}
	if s.Cores != 4 || s.Load1 != 0.52 || s.Load5 != 0.58 || s.Load15 != 0.59 {
		t.Errorf("cores/load: %+v", s)
	}
	if !near(s.MemPct, 75) || s.MemTotalMB != 7812 || s.MemUsedMB != 5859 || !near(s.SwapPct, 25) {
		t.Errorf("memory: %+v", s)
	}
	if s.T != 1000 {
		t.Errorf("T = %d", s.T)
	}
}

func TestRatesComeFromTwoReadings(t *testing.T) {
	var p Parser
	p.Parse(base().String(), time.Unix(1000, 0))
	b := base()
	b.cpu = [5]int{1300, 600, 8400, 100, 40} // +300 user, +100 system, +400 idle, +0 iowait, +40 steal
	b.rx, b.tx = 6000, 12000
	b.sectorsR, b.sectorsW = 1100, 500
	s, err := p.Parse(b.String(), time.Unix(1005, 0))
	if err != nil || !s.HasRates {
		t.Fatalf("%+v %v", s, err)
	}
	if !near(s.CPU, 52.4) || !near(s.Steal, 4.8) { // busy 440 of 840
		t.Errorf("cpu %.2f steal %.2f", s.CPU, s.Steal)
	}
	if s.NetRx != 1000 || s.NetTx != 2000 {
		t.Errorf("net: lo, docker0 and veth must not count: %v %v", s.NetRx, s.NetTx)
	}
	if s.DiskR != 102400 || s.DiskW != 51200 {
		t.Errorf("disk: partitions and loop devices must not count: %v %v", s.DiskR, s.DiskW)
	}
}

func TestACounterThatGoesBackwardsGivesNoRate(t *testing.T) {
	var p Parser
	p.Parse(base().String(), time.Unix(1000, 0))
	b := base()
	b.cpu = [5]int{10, 10, 10, 10, 0} // the machine rebooted
	b.rx = 5                          // and its network counters reset
	s, err := p.Parse(b.String(), time.Unix(1005, 0))
	if err != nil {
		t.Fatal(err)
	}
	if s.HasRates || s.CPU < 0 || s.NetRx < 0 || s.DiskR < 0 {
		t.Errorf("a reset must not make negative numbers: %+v", s)
	}
	// and the next reading works again
	b.cpu = [5]int{110, 10, 110, 10, 0}
	if s, _ = p.Parse(b.String(), time.Unix(1010, 0)); !s.HasRates || !near(s.CPU, 50) {
		t.Errorf("recovery: %+v", s)
	}
}

func TestSameSecondTwiceDoesNotDivideByZero(t *testing.T) {
	var p Parser
	p.Parse(base().String(), time.Unix(1000, 0))
	s, err := p.Parse(base().String(), time.Unix(1000, 0))
	if err != nil || s.HasRates {
		t.Errorf("%+v %v", s, err)
	}
}

func TestDisksAreOnePerDeviceUsingTheShortestMount(t *testing.T) {
	var p Parser
	s, _ := p.Parse(base().String(), time.Unix(1000, 0))
	if len(s.Disks) != 2 || s.Disks[0] != (Disk{"/", 90}) || s.Disks[1] != (Disk{"/data", 50}) {
		t.Errorf("%+v", s.Disks)
	}
}

func TestWithoutMemAvailableItFallsBackToFreeBuffersCached(t *testing.T) {
	out := strings.Replace(base().String(), "MemAvailable: 2000000 kB\n", "", 1)
	out = strings.Replace(out, "MemFree: 1 kB", "MemFree: 1000000 kB\nCached: 1000000 kB", 1)
	var p Parser
	s, err := p.Parse(out, time.Unix(1000, 0))
	if err != nil || !near(s.MemPct, 75) { // 8,000,000 total, 1,000,000 + 1 + 1,000,000 free-ish
		t.Errorf("%+v %v", s, err)
	}
}

func TestOutputWithoutProcIsUnsupported(t *testing.T) {
	var p Parser
	_, err := p.Parse("@@stat\ncat: /proc/stat: No such file or directory\n@@meminfo\n", time.Unix(1, 0))
	if !errors.Is(err, ErrUnsupported) {
		t.Errorf("got %v", err)
	}
	if _, err := p.Parse("", time.Unix(1, 0)); !errors.Is(err, ErrUnsupported) {
		t.Errorf("empty: %v", err)
	}
}

func TestCmdIsReadOnly(t *testing.T) {
	c := strings.ReplaceAll(Cmd, "2>/dev/null", "")
	for _, bad := range []string{" rm ", ">", "sudo", "kill", "mv ", "tee"} {
		if strings.Contains(c, bad) {
			t.Errorf("Cmd must only read, found %q", bad)
		}
	}
}
