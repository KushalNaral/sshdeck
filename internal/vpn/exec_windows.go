package vpn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// prepare runs the client without a console window.
func prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}

// defaultLookPath finds a program on PATH, or in the folders the OpenConnect installers use.
func defaultLookPath(file string) (string, error) {
	if p, err := exec.LookPath(file); err == nil {
		return p, nil
	}
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)"} {
		base := os.Getenv(env)
		if base == "" {
			continue
		}
		for _, dir := range []string{"OpenConnect", "OpenConnect-GUI"} {
			p := filepath.Join(base, dir, file+".exe")
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
	}
	return "", exec.ErrNotFound
}

// Stop ends the client process. The helper runs elevated like the client, so a plain kill works; the tunnel adapter
// goes away with the process.
func (p *execProc) Stop() error {
	select {
	case <-p.done:
		return nil
	default:
	}
	_ = p.cmd.Process.Kill()
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("the VPN client (pid %d) did not stop", p.cmd.Process.Pid)
	}
}
