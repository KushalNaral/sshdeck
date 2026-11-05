//go:build !windows

package main

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

// ownedByAnotherUser reports whether a file belongs to someone other than the user running sshdeck.
func ownedByAnotherUser(st fs.FileInfo) (string, bool) {
	if sys, ok := st.Sys().(*syscall.Stat_t); ok && sys.Uid != uint32(os.Geteuid()) {
		return fmt.Sprintf("uid %d", sys.Uid), true
	}
	return "", false
}
