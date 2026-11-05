package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// ownFiles are the files sshdeck keeps in its config directory; they hold host lists, keys or the encrypted vault.
var ownFiles = []string{"data.json", "vault.enc", "known_hosts", "audit.log", "settings.json"}

// checkConfigDir makes the config directory private (0700) and its files owner-only (0600) before anything
// is read from them. It refuses a file that is a symlink or belongs to someone else, because both let another
// user redirect or read the data.
func checkConfigDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("cannot make %s private: %w", dir, err)
	}
	for _, name := range ownFiles {
		p := filepath.Join(dir, name)
		st, err := os.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link; sshdeck will not follow it. Move the real file into place and try again", p)
		}
		if who, other := ownedByAnotherUser(st); other {
			return fmt.Errorf("%s belongs to another user (%s)", p, who)
		}
		if st.Mode().Perm() != 0o600 {
			if err := os.Chmod(p, 0o600); err != nil {
				return fmt.Errorf("cannot make %s private: %w", p, err)
			}
		}
	}
	return nil
}
