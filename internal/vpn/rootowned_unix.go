//go:build !windows

package vpn

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// checkRootOwned refuses a program that an ordinary user could replace. The VPN clients run as root after one
// password prompt, so a binary in a user-writable place (or in a user-writable directory) must not be run.
func checkRootOwned(path string) error {
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	for p := real; ; p = filepath.Dir(p) {
		st, err := os.Stat(p)
		if err != nil {
			return err
		}
		if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid != 0 {
			return fmt.Errorf("%s is not owned by root", p)
		}
		if st.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("%s can be changed by someone other than root", p)
		}
		if p == "/" {
			return nil
		}
	}
}
