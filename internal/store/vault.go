package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/KushalNaral/sshdeck/internal/vault"
)

var now = time.Now // swapped in tests

// TooSoonError means too many wrong passphrases in a row: wait before trying again.
type TooSoonError struct{ Wait time.Duration }

func (e *TooSoonError) Error() string {
	return fmt.Sprintf("too many wrong passphrases; try again in %d seconds", int(e.Wait.Seconds())+1)
}

// OpenLocked opens an encrypted store. It starts locked: nothing is readable until Unlock.
func OpenLocked(path string) (*Store, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !vault.IsVault(b) {
		return nil, vault.ErrNotVault
	}
	return &Store{path: path, vaultMode: true, locked: true}, nil
}

// State says whether the passphrase lock is enabled, and whether it is locked right now.
func (s *Store) State() (enabled, locked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vaultMode, s.locked
}

// EnableLock moves everything under a passphrase: data and passwords go into an encrypted file at vaultPath,
// which is opened again to check it before anything old is removed. Then the plaintext data file and the
// keyring entries are deleted. leftover names keyring entries that could not be deleted.
func (s *Store) EnableLock(vaultPath, pass string) (leftover []string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.vaultMode:
		return nil, invalid("the passphrase lock is already enabled")
	case pass == "":
		return nil, invalid("the passphrase must not be empty")
	}
	var keys []string
	secrets := map[string]string{}
	collect := func(key string) error {
		v, err := s.sec.Get(key)
		switch {
		case err == nil:
			secrets[key] = v
			keys = append(keys, key)
		case !errors.Is(err, ErrNotFound):
			return fmt.Errorf("read %s from the keyring: %w", key, err)
		}
		return nil
	}
	for _, sv := range s.d.Servers {
		if err := collect("server/" + sv.ID); err != nil {
			return nil, err
		}
	}
	for _, v := range s.d.VPNs {
		if err := collect("vpn/" + v.ID); err != nil {
			return nil, err
		}
	}
	d := s.d
	d.Secrets = secrets
	plain, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	key, err := vault.NewKey(pass)
	if err != nil {
		return nil, err
	}
	blob, err := key.Seal(plain)
	if err != nil {
		return nil, err
	}
	if err := writeAtomic(vaultPath, blob); err != nil {
		return nil, err
	}
	// Prove the new file opens with this passphrase and holds the same data before deleting anything.
	back, err := os.ReadFile(vaultPath)
	if err != nil {
		return nil, err
	}
	if _, got, err := vault.Open(pass, back); err != nil || !bytes.Equal(got, plain) {
		os.Remove(vaultPath)
		return nil, errors.New("the encrypted copy did not verify; nothing was changed")
	}
	oldPath := s.path
	s.d, s.path, s.key, s.vaultMode = d, vaultPath, key, true
	if oldPath != vaultPath {
		_ = os.Remove(oldPath)
	}
	for _, k := range keys {
		if err := s.sec.Delete(k); err != nil && !errors.Is(err, ErrNotFound) {
			leftover = append(leftover, k)
		}
	}
	return leftover, nil
}

// Unlock decrypts the vault. Repeated wrong passphrases make later attempts wait.
func (s *Store) Unlock(pass string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !s.vaultMode:
		return invalid("the passphrase lock is not enabled")
	case !s.locked:
		return nil
	}
	if wait := s.until.Sub(now()); wait > 0 {
		return &TooSoonError{Wait: wait}
	}
	blob, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	key, plain, err := vault.Open(pass, blob)
	if err != nil {
		if errors.Is(err, vault.ErrWrongPassphrase) {
			if s.fails++; s.fails >= 3 {
				back := 500 * time.Millisecond << min(s.fails-3, 6) // 0.5 s, 1 s, 2 s ... capped near 30 s
				s.until = now().Add(back)
			}
		}
		return err
	}
	var d fileData
	if err := json.Unmarshal(plain, &d); err != nil {
		key.Zero()
		return fmt.Errorf("read the vault: %w", err)
	}
	s.d, s.key, s.locked, s.fails = d, key, false, 0
	return nil
}

// Lock wipes the key and drops everything from memory. Go cannot promise to erase every copy of a string,
// so this is best effort, which is why the key itself is zeroed.
func (s *Store) Lock() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.vaultMode || s.locked {
		return
	}
	s.key.Zero()
	s.key, s.d, s.locked = nil, fileData{}, true
}

// ChangePassphrase re-encrypts the vault under a new passphrase after checking the old one.
func (s *Store) ChangePassphrase(oldPass, newPass string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !s.vaultMode:
		return invalid("the passphrase lock is not enabled")
	case s.locked:
		return ErrLocked
	case newPass == "":
		return invalid("the new passphrase must not be empty")
	}
	blob, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	if _, _, err := vault.Open(oldPass, blob); err != nil {
		return err
	}
	nk, err := vault.NewKey(newPass)
	if err != nil {
		return err
	}
	old := s.key
	s.key = nk
	if err := s.flush(); err != nil {
		s.key = old
		nk.Zero()
		return err
	}
	old.Zero()
	return nil
}

// DisableLock returns to the plain setup: passwords go back into sec (the OS keyring), the data goes to
// plainPath, and the vault file is deleted. The passphrase is checked first. The vault is removed last, so a
// failure at any earlier step leaves it intact.
func (s *Store) DisableLock(pass, plainPath string, sec Secrets) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !s.vaultMode:
		return invalid("the passphrase lock is not enabled")
	case s.locked:
		return ErrLocked
	}
	blob, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	if _, _, err := vault.Open(pass, blob); err != nil {
		return err
	}
	var done []string
	undo := func() {
		for _, k := range done {
			_ = sec.Delete(k)
		}
	}
	for k, v := range s.d.Secrets {
		if err := sec.Set(k, v); err != nil {
			undo()
			return fmt.Errorf("move %s back to the keyring: %w", k, err)
		}
		done = append(done, k)
	}
	d := s.d
	d.Secrets = nil
	plain, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		undo()
		return err
	}
	if err := writeAtomic(plainPath, plain); err != nil {
		undo()
		return err
	}
	vaultPath := s.path
	s.key.Zero()
	s.d, s.path, s.sec, s.key, s.vaultMode = d, plainPath, sec, nil, false
	_ = os.Remove(vaultPath)
	return nil
}
