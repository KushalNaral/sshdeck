// Package metrics turns /proc readings taken over SSH into rates and levels, keeps them for a while and decides
// when something deserves an alert. It runs nothing itself: the caller supplies the output of Cmd.
package metrics

import (
	"errors"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Cmd is the one read-only command a sample costs. Each file is introduced by a marker line so the output can be
// split without guessing which file a line came from.
const Cmd = `for f in stat meminfo loadavg net/dev diskstats; do echo "@@$f"; cat /proc/$f 2>/dev/null; done; echo @@df; df -P -x tmpfs -x devtmpfs -x overlay 2>/dev/null`

// ErrUnsupported means the server did not answer like Linux (no /proc/stat).
var ErrUnsupported = errors.New("this server does not expose /proc, so it cannot be sampled")

type Disk struct {
	Mount   string `json:"mount"`
	UsedPct int    `json:"usedPct"`
}

// Sample is one reading. The rate fields (CPU, Steal, Net*, Disk*) are only meaningful when HasRates is true:
// they need the reading before this one.
type Sample struct {
	T          int64   `json:"t"` // unix seconds
	HasRates   bool    `json:"hasRates"`
	CPU        float64 `json:"cpu"`   // percent of all cores that was busy
	Steal      float64 `json:"steal"` // percent taken by the hypervisor
	Load1      float64 `json:"load1"`
	Load5      float64 `json:"load5"`
	Load15     float64 `json:"load15"`
	Cores      int     `json:"cores"`
	MemPct     float64 `json:"memPct"`
	MemUsedMB  int     `json:"memUsedMb"`
	MemTotalMB int     `json:"memTotalMb"`
	SwapPct    float64 `json:"swapPct"`
	NetRx      float64 `json:"netRx"` // bytes per second
	NetTx      float64 `json:"netTx"`
	DiskR      float64 `json:"diskR"` // bytes per second
	DiskW      float64 `json:"diskW"`
	Disks      []Disk  `json:"disks"`
}

// raw holds the running counters of one reading.
type raw struct {
	at                         time.Time
	cpu                        [8]float64 // user nice system idle iowait irq softirq steal
	rx, tx, sectorsR, sectorsW float64
}

// Parser remembers the previous reading of one server so it can turn counters into rates.
type Parser struct {
	prev *raw
}

// Parse reads the output of Cmd. A counter that went backwards (a reboot) gives a sample without rates; the
// next one works again.
func (p *Parser) Parse(out string, now time.Time) (Sample, error) {
	secs := sections(out)
	statLines := strings.Split(secs["stat"], "\n")
	var cur raw
	cur.at = now
	s := Sample{T: now.Unix(), Disks: []Disk{}}
	seenCPU := false
	for _, l := range statLines {
		f := strings.Fields(l)
		switch {
		case len(f) >= 9 && f[0] == "cpu":
			seenCPU = true
			for i := 0; i < 8; i++ {
				cur.cpu[i], _ = strconv.ParseFloat(f[i+1], 64)
			}
		case len(f) > 0 && cpuN.MatchString(f[0]):
			s.Cores++
		}
	}
	if !seenCPU {
		return Sample{}, ErrUnsupported
	}

	var memTotal, memAvail, memFree, buffers, cached, swapTotal, swapFree float64
	hasAvail := false
	for _, l := range strings.Split(secs["meminfo"], "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		n, _ := strconv.ParseFloat(strings.Fields(v + " 0")[0], 64)
		switch k {
		case "MemTotal":
			memTotal = n
		case "MemAvailable":
			memAvail, hasAvail = n, true
		case "MemFree":
			memFree = n
		case "Buffers":
			buffers = n
		case "Cached":
			cached = n
		case "SwapTotal":
			swapTotal = n
		case "SwapFree":
			swapFree = n
		}
	}
	if !hasAvail {
		memAvail = memFree + buffers + cached
	}
	if memTotal > 0 {
		s.MemPct = 100 * (memTotal - memAvail) / memTotal
		s.MemTotalMB = int(memTotal / 1024)
		s.MemUsedMB = int((memTotal - memAvail) / 1024)
	}
	if swapTotal > 0 {
		s.SwapPct = 100 * (swapTotal - swapFree) / swapTotal
	}

	if f := strings.Fields(secs["loadavg"]); len(f) >= 3 {
		s.Load1, _ = strconv.ParseFloat(f[0], 64)
		s.Load5, _ = strconv.ParseFloat(f[1], 64)
		s.Load15, _ = strconv.ParseFloat(f[2], 64)
	}

	for _, l := range strings.Split(secs["net/dev"], "\n") {
		name, rest, ok := strings.Cut(l, ":")
		name = strings.TrimSpace(name)
		if !ok || skipNIC(name) {
			continue
		}
		if f := strings.Fields(rest); len(f) >= 9 {
			rx, _ := strconv.ParseFloat(f[0], 64)
			tx, _ := strconv.ParseFloat(f[8], 64)
			cur.rx += rx
			cur.tx += tx
		}
	}
	for _, l := range strings.Split(secs["diskstats"], "\n") {
		if f := strings.Fields(l); len(f) >= 10 && wholeDisk.MatchString(f[2]) {
			r, _ := strconv.ParseFloat(f[5], 64)
			w, _ := strconv.ParseFloat(f[9], 64)
			cur.sectorsR += r
			cur.sectorsW += w
		}
	}
	s.Disks = parseDF(secs["df"])

	if prev := p.prev; prev != nil {
		dt := now.Sub(prev.at).Seconds()
		var dTot [8]float64
		sum, sumPrev := 0.0, 0.0
		for i := range cur.cpu {
			dTot[i] = cur.cpu[i] - prev.cpu[i]
			sum += cur.cpu[i]
			sumPrev += prev.cpu[i]
		}
		back := false
		for i := range dTot {
			if dTot[i] < 0 {
				back = true
			}
		}
		if cur.rx < prev.rx || cur.tx < prev.tx || cur.sectorsR < prev.sectorsR || cur.sectorsW < prev.sectorsW {
			back = true
		}
		if dt > 0 && !back && sum-sumPrev > 0 {
			total := sum - sumPrev
			s.CPU = 100 * (total - dTot[3] - dTot[4]) / total
			s.Steal = 100 * dTot[7] / total
			s.NetRx = (cur.rx - prev.rx) / dt
			s.NetTx = (cur.tx - prev.tx) / dt
			s.DiskR = (cur.sectorsR - prev.sectorsR) * 512 / dt
			s.DiskW = (cur.sectorsW - prev.sectorsW) * 512 / dt
			s.HasRates = true
		}
	}
	p.prev = &cur
	return s, nil
}

var (
	cpuN      = regexp.MustCompile(`^cpu\d+$`)
	wholeDisk = regexp.MustCompile(`^(sd[a-z]+|vd[a-z]+|xvd[a-z]+|hd[a-z]+|nvme\d+n\d+|mmcblk\d+)$`)
)

// skipNIC leaves out interfaces whose traffic is already counted on another one (container bridges and their
// veth ends) or never leaves the machine.
func skipNIC(n string) bool {
	return n == "lo" || n == "docker0" || strings.HasPrefix(n, "veth") || strings.HasPrefix(n, "br-") || strings.HasPrefix(n, "virbr")
}

// sections splits Cmd's output at its "@@name" marker lines.
func sections(out string) map[string]string {
	m := map[string]string{}
	var name string
	var b strings.Builder
	flush := func() {
		if name != "" {
			m[name] = b.String()
		}
		b.Reset()
	}
	for _, l := range strings.Split(out, "\n") {
		if n, ok := strings.CutPrefix(l, "@@"); ok {
			flush()
			name = strings.TrimSpace(n)
			continue
		}
		b.WriteString(l + "\n")
	}
	flush()
	return m
}

// parseDF reads df -P. A device bind-mounted in several places (docker does this) counts once, under its
// shortest mount.
func parseDF(s string) []Disk {
	best := map[string]Disk{}
	var order []string
	for _, l := range strings.Split(s, "\n") {
		f := strings.Fields(l)
		if len(f) < 6 || !strings.HasPrefix(f[0], "/dev/") {
			continue
		}
		pct, err := strconv.Atoi(strings.TrimSuffix(f[4], "%"))
		if err != nil {
			continue
		}
		mount := strings.Join(f[5:], " ")
		cur, seen := best[f[0]]
		if !seen {
			order = append(order, f[0])
		}
		if !seen || len(mount) < len(cur.Mount) {
			best[f[0]] = Disk{Mount: mount, UsedPct: pct}
		}
	}
	out := make([]Disk, 0, len(order))
	for _, d := range order {
		out = append(out, best[d])
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Mount < out[j].Mount })
	return out
}
