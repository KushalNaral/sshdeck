package metrics

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

// This runs the real command against the machine the tests run on, so the parser is checked against a real
// /proc and not only against fixtures written by hand.
func TestCmdAgainstThisMachine(t *testing.T) {
	if _, err := os.Stat("/proc/stat"); err != nil {
		t.Skip("no /proc here")
	}
	read := func() string {
		out, _ := exec.Command("sh", "-c", Cmd).Output() // df may exit non-zero on odd mounts; the output is still good
		return string(out)
	}
	var p Parser
	if _, err := p.Parse(read(), time.Now()); err != nil {
		t.Fatal(err)
	}
	busy := time.Now().Add(150 * time.Millisecond)
	for time.Now().Before(busy) { // give the CPU counters something to count
	}
	s, err := p.Parse(read(), time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !s.HasRates || s.Cores < 1 || s.MemTotalMB < 1 || s.MemPct <= 0 || s.MemPct > 100 || s.CPU < 0 || s.CPU > 100 {
		t.Errorf("implausible reading: %+v", s)
	}
	if s.NetRx < 0 || s.NetTx < 0 || s.DiskR < 0 || s.DiskW < 0 {
		t.Errorf("negative rate: %+v", s)
	}
	t.Logf("cores=%d cpu=%.1f%% mem=%.0f%% (%d MB) load=%.2f disks=%v net=%.0f/%.0f B/s", s.Cores, s.CPU, s.MemPct, s.MemTotalMB, s.Load1, s.Disks, s.NetRx, s.NetTx)
}
