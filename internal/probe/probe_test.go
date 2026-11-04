package probe

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// fixture is what a server prints, keyed by command. A missing key means "command not found".
type fixture map[string]string

func (f fixture) run(cmd string) (string, error) {
	if out, ok := f[cmd]; ok {
		return out, nil
	}
	return "", errors.New("exit status 127")
}

var ubuntu2204 = fixture{
	CmdOSRelease: `PRETTY_NAME="Ubuntu 22.04.4 LTS"
NAME="Ubuntu"
VERSION_ID="22.04"
ID=ubuntu
`,
	CmdKernel: "5.15.0-107-generic\n",
	CmdUptime: "up 3 weeks, 2 days, 4 hours\n",
	CmdCPU:    "4\n",
	CmdMem: `               total        used        free      shared  buff/cache   available
Mem:            7961        2600         812         101        4549        5003
Swap:           2047           0        2047
`,
	CmdDisk: `Filesystem      Size  Used Avail Use% Mounted on
tmpfs           797M  1.1M  796M   1% /run
/dev/vda1        78G   22G   56G  29% /
tmpfs           3.9G     0  3.9G   0% /dev/shm
/dev/vdb1       100G   94G  6.0G  94% /var
`,
	CmdPHP: `PHP 8.2.25 (cli) (built: Oct 29 2024 10:00:00) (NTS)
Copyright (c) The PHP Group
Zend Engine v4.2.25, Copyright (c) Zend Technologies
`,
	CmdWeb: "nginx version: nginx/1.18.0 (Ubuntu)\n",
	CmdDB:  "mysql  Ver 8.0.36-0ubuntu0.22.04.1 for Linux on x86_64 ((Ubuntu))\n",
	CmdFailed: `php8.2-fpm.service loaded failed failed The PHP 8.2 FastCGI Process Manager
supervisor-queue.service loaded failed failed Queue workers
`,
	CmdDocker: `app|php:8.2-fpm|Up 3 days|9000/tcp
redis|redis:7|Up 3 days|0.0.0.0:6379->6379/tcp
queue-worker|php:8.2-cli|Restarting (1) 5 seconds ago|
old-job|alpine:3|Exited (0) 2 days ago|
`,
}

// A CentOS 7 box: Apache, MariaDB, nothing failed, and no docker.
var centos7 = fixture{
	CmdOSRelease: `NAME="CentOS Linux"
VERSION="7 (Core)"
PRETTY_NAME="CentOS Linux 7 (Core)"
`,
	CmdKernel: "3.10.0-1160.el7.x86_64\n",
	CmdUptime: "up 41 weeks, 1 day\n",
	CmdCPU:    "2\n",
	CmdMem: `              total        used        free      shared  buff/cache   available
Mem:           3790        1204         210         120        2375        2250
Swap:          2047         312        1735
`,
	CmdDisk: `Filesystem      Size  Used Avail Use% Mounted on
devtmpfs        1.9G     0  1.9G   0% /dev
/dev/sda3        40G   31G  9.0G  78% /
`,
	CmdPHP: `PHP 7.4.27 (cli) (built: Dec 17 2021 12:00:00) ( NTS )
Copyright (c) The PHP Group
`,
	CmdWeb:    "Server version: Apache/2.4.6 (CentOS)\nServer built:   Nov 16 2023\n",
	CmdDB:     "mysql  Ver 15.1 Distrib 10.6.16-MariaDB, for Linux (x86_64) using readline 5.1\n",
	CmdFailed: "",
}

func TestUbuntuWithDocker(t *testing.T) {
	got, err := Analyze(context.Background(), ubuntu2204.run)
	if err != nil {
		t.Fatal(err)
	}
	want := Report{
		OS: "Ubuntu 22.04.4 LTS", Kernel: "5.15.0-107-generic", Uptime: "up 3 weeks, 2 days, 4 hours",
		CPU: 4, MemTotalMB: 7961, MemUsedMB: 2600,
		Disks: []Disk{{"/", 29}, {"/var", 94}},
		PHP:   "8.2.25", WebServer: "Nginx 1.18.0", DB: "MySQL 8.0.36",
		FailedUnits: []string{"php8.2-fpm.service", "supervisor-queue.service"},
		Containers: []Container{
			{"app", "php:8.2-fpm", "running", "9000/tcp"},
			{"redis", "redis:7", "running", "0.0.0.0:6379->6379/tcp"},
			{"queue-worker", "php:8.2-cli", "restarting", ""},
			{"old-job", "alpine:3", "exited", ""},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestCentOSWithoutDocker(t *testing.T) {
	got, err := Analyze(context.Background(), centos7.run)
	if err != nil {
		t.Fatal(err)
	}
	if got.OS != "CentOS Linux 7 (Core)" || got.PHP != "7.4.27" || got.WebServer != "Apache 2.4.6" || got.DB != "MariaDB 10.6.16" {
		t.Fatalf("got %+v", got)
	}
	if got.Containers != nil {
		t.Errorf("Containers = %v, want nil when docker is missing", got.Containers)
	}
	if len(got.FailedUnits) != 0 {
		t.Errorf("FailedUnits = %v, want none", got.FailedUnits)
	}
	if want := []Disk{{"/", 78}}; !reflect.DeepEqual(got.Disks, want) {
		t.Errorf("Disks = %v, want %v (devtmpfs is not a real disk)", got.Disks, want)
	}
}

func TestDockerInstalledButNoContainersIsEmptyNotNil(t *testing.T) {
	f := fixture{CmdOSRelease: ubuntu2204[CmdOSRelease], CmdDocker: ""}
	got, err := Analyze(context.Background(), f.run)
	if err != nil {
		t.Fatal(err)
	}
	if got.Containers == nil || len(got.Containers) != 0 {
		t.Errorf("Containers = %#v, want an empty non-nil slice (docker works, nothing runs)", got.Containers)
	}
}

func TestMissingToolsLeaveFieldsEmpty(t *testing.T) {
	f := fixture{CmdOSRelease: ubuntu2204[CmdOSRelease], CmdKernel: "6.8.0-45-generic\n"}
	got, err := Analyze(context.Background(), f.run)
	if err != nil {
		t.Fatalf("a server without PHP or a database is not an error: %v", err)
	}
	if got.OS != "Ubuntu 22.04.4 LTS" || got.Kernel != "6.8.0-45-generic" || got.PHP != "" || got.DB != "" || got.WebServer != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestEveryCommandFailingIsAnError(t *testing.T) {
	_, err := Analyze(context.Background(), fixture{}.run)
	if err == nil {
		t.Fatal("want an error when the server answers nothing at all")
	}
}

func TestOnlyReadOnlyCommandsAreRun(t *testing.T) {
	var ran []string
	run := func(cmd string) (string, error) { ran = append(ran, cmd); return ubuntu2204.run(cmd) }
	if _, err := Analyze(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{CmdOSRelease: true, CmdKernel: true, CmdUptime: true, CmdCPU: true, CmdMem: true, CmdDisk: true,
		CmdPHP: true, CmdWeb: true, CmdDB: true, CmdFailed: true, CmdDocker: true}
	for _, c := range ran {
		if !allowed[c] {
			t.Errorf("ran %q, which is not in the read-only command list", c)
		}
	}
}

func TestCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Analyze(ctx, ubuntu2204.run); err == nil {
		t.Fatal("want ctx.Err() when ctx is already cancelled")
	}
}
