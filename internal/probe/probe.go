// Package probe analyses a server by running read-only commands over SSH and parsing what they print.
// It never changes anything on the server.
package probe

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// The commands Analyze runs. They are part of the contract: tests (and the UI's "what did you run" view)
// key on these exact strings. Every one is read-only.
const (
	CmdOSRelease = "cat /etc/os-release"
	CmdKernel    = "uname -r"
	CmdUptime    = "uptime -p"
	CmdCPU       = "nproc"
	CmdMem       = "free -m"
	CmdDisk      = "df -hP"
	CmdPHP       = "php -v"
	CmdWeb       = "nginx -v 2>&1 || apache2 -v 2>&1 || httpd -v 2>&1"
	CmdDB        = "mysql --version"
	CmdFailed    = "systemctl --failed --no-legend --plain"
	CmdDocker    = "docker ps -a --format '{{.Names}}|{{.Image}}|{{.Status}}|{{.Ports}}'"
)

type Disk struct {
	Mount   string `json:"mount"`
	UsedPct int    `json:"usedPct"`
}

type Container struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"` // "running", "restarting" or "exited"
	Ports  string `json:"ports"`
}

// Report is best-effort: a field stays empty when its command is missing or fails (no PHP, no MySQL).
type Report struct {
	OS          string      `json:"os"`     // PRETTY_NAME from /etc/os-release, e.g. "Ubuntu 22.04.4 LTS"
	Kernel      string      `json:"kernel"` // "5.15.0-107-generic"
	Uptime      string      `json:"uptime"` // as printed by uptime -p, "up 3 weeks, 2 days"
	CPU         int         `json:"cpu"`
	MemTotalMB  int         `json:"memTotalMb"`
	MemUsedMB   int         `json:"memUsedMb"`
	Disks       []Disk      `json:"disks"`     // real filesystems only (source starts with /dev/), not tmpfs
	PHP         string      `json:"php"`       // "8.2.25"
	WebServer   string      `json:"webserver"` // "Nginx 1.18.0" or "Apache 2.4.6"
	DB          string      `json:"db"`        // "MySQL 8.0.36" or "MariaDB 10.6.16"
	FailedUnits []string    `json:"failedUnits"`
	Containers  []Container `json:"containers"` // nil (null) when docker is not installed
}

// Run executes one command on the server and returns its stdout. The real one wraps sshx.Run.
type Run func(cmd string) (string, error)

// Analyze runs the commands above and fills a Report. A command that fails leaves its fields empty and is
// not an error. Analyze fails only when ctx ends or when every command failed (the server was unreachable).
func Analyze(ctx context.Context, run Run) (Report, error) {
	var r Report
	answered := 0
	get := func(cmd string) (string, bool) {
		if ctx.Err() != nil {
			return "", false
		}
		out, err := run(cmd)
		if err != nil {
			return "", false
		}
		answered++
		return strings.TrimSpace(out), true
	}
	lines := func(s string) []string { return strings.Split(s, "\n") }

	if o, ok := get(CmdOSRelease); ok {
		for _, l := range lines(o) {
			if v, found := strings.CutPrefix(l, "PRETTY_NAME="); found {
				r.OS = strings.Trim(v, `"'`)
			}
		}
	}
	r.Kernel, _ = get(CmdKernel)
	r.Uptime, _ = get(CmdUptime)
	if o, ok := get(CmdCPU); ok {
		r.CPU, _ = strconv.Atoi(o)
	}
	if o, ok := get(CmdMem); ok {
		for _, l := range lines(o) {
			if f := strings.Fields(l); len(f) > 2 && f[0] == "Mem:" {
				r.MemTotalMB, _ = strconv.Atoi(f[1])
				r.MemUsedMB, _ = strconv.Atoi(f[2])
			}
		}
	}
	if o, ok := get(CmdDisk); ok {
		for _, l := range lines(o) {
			if f := strings.Fields(l); len(f) >= 6 && strings.HasPrefix(f[0], "/dev/") {
				if pct, err := strconv.Atoi(strings.TrimSuffix(f[4], "%")); err == nil {
					r.Disks = append(r.Disks, Disk{Mount: f[5], UsedPct: pct})
				}
			}
		}
	}
	if o, ok := get(CmdPHP); ok {
		if f := strings.Fields(o); len(f) > 1 && f[0] == "PHP" {
			r.PHP = f[1]
		}
	}
	if o, ok := get(CmdWeb); ok {
		if m := webRe.FindStringSubmatch(o); m != nil {
			r.WebServer = strings.ToUpper(m[1][:1]) + strings.ToLower(m[1][1:]) + " " + m[2]
		}
	}
	if o, ok := get(CmdDB); ok {
		if m := mariaRe.FindStringSubmatch(o); m != nil {
			r.DB = "MariaDB " + m[1]
		} else if m := mysqlRe.FindStringSubmatch(o); m != nil {
			r.DB = "MySQL " + m[1]
		}
	}
	if o, _ := get(CmdFailed); o != "" {
		for _, l := range lines(o) {
			if f := strings.Fields(strings.TrimLeft(l, "●* ")); len(f) > 0 {
				r.FailedUnits = append(r.FailedUnits, f[0])
			}
		}
	}
	if o, ok := get(CmdDocker); ok {
		r.Containers = []Container{} // docker answered: empty, not nil, when nothing exists
		for _, l := range lines(o) {
			if p := strings.SplitN(l, "|", 4); len(p) == 4 {
				r.Containers = append(r.Containers, Container{Name: p[0], Image: p[1], Status: containerState(p[2]), Ports: p[3]})
			}
		}
	}

	if err := ctx.Err(); err != nil {
		return r, err
	}
	if answered == 0 {
		return r, errors.New("the server answered none of the analysis commands")
	}
	return r, nil
}

var (
	webRe   = regexp.MustCompile(`(?i)\b(nginx|apache)/(\d+(?:\.\d+)*)`)
	mariaRe = regexp.MustCompile(`(\d+(?:\.\d+)*)-MariaDB`)
	mysqlRe = regexp.MustCompile(`Ver (\d+(?:\.\d+)*)`)
)

// containerState maps docker's free-text status ("Up 3 days", "Restarting (1) 5 seconds ago") to three states.
func containerState(s string) string {
	switch {
	case strings.HasPrefix(s, "Up"):
		return "running"
	case strings.HasPrefix(s, "Restarting"):
		return "restarting"
	}
	return "exited"
}
