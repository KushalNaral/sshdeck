//go:build !windows

package main

import "os/exec"

// relaunch starts the program again after a second, so this copy has quit and released the single-instance lock.
func relaunch(exe string) error {
	return exec.Command("sh", "-c", `sleep 1; exec "$0"`, exe).Start()
}
