package main

import (
	"os/exec"
	"syscall"
)

// relaunch starts the program again after about two seconds, so this copy has quit and released the instance lock.
func relaunch(exe string) error {
	cmd := exec.Command("cmd")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CmdLine: `cmd /c ping -n 3 127.0.0.1 >nul & start "" "` + exe + `"`}
	return cmd.Start()
}
