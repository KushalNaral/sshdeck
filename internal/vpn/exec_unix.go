//go:build !windows

package vpn

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func defaultLookPath(file string) (string, error) { return exec.LookPath(file) }

// prepare puts the client in its own process group, and ends it if sshdeck dies.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}

// Stop ends the client. pkexec runs the client as root, so a plain signal from this user is refused (EPERM);
// then the client's processes are ended through pkexec itself (polkit may ask for the password again).
// shortcut: needs a real VPN to confirm end to end; unit-tested only with plain processes.
func (p *execProc) Stop() error {
	select {
	case <-p.done:
		return nil
	default:
	}
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		p.killAsRoot()
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
	}
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		p.killAsRoot()
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("the VPN client (pid %d) did not stop; end it with: sudo kill %d", p.cmd.Process.Pid, p.cmd.Process.Pid)
	}
}

// killAsRoot sends SIGTERM to pkexec's children, which are the root-owned client.
func (p *execProc) killAsRoot() {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(p.cmd.Process.Pid)).Output()
	if err != nil {
		return
	}
	pids := strings.Fields(string(out))
	if len(pids) == 0 {
		return
	}
	_ = exec.Command("pkexec", append([]string{"kill", "-TERM"}, pids...)...).Run()
}
