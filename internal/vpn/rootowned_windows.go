package vpn

import (
	"fmt"
	"os"
	"path/filepath"
)

// checkRootOwned is the Windows version of "an ordinary user cannot replace this program": it must be inside
// Program Files or the Windows folder, which only administrators can write to. The program then runs elevated.
func checkRootOwned(path string) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	for _, env := range []string{"ProgramFiles", "ProgramW6432", "ProgramFiles(x86)", "SystemRoot"} {
		if d := os.Getenv(env); d != "" && underDir(real, d) {
			return nil
		}
	}
	return fmt.Errorf("%s is not in Program Files, so a user could replace it; install OpenConnect for all users", real)
}
