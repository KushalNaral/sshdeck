package store

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const keyringService = "sshdeck"

// Keyring stores secrets in the OS keyring (GNOME Keyring, KWallet, Keychain, Credential Manager).
type Keyring struct{}

func (Keyring) Set(key, secret string) error { return keyring.Set(keyringService, key, secret) }

func (Keyring) Get(key string) (string, error) {
	v, err := keyring.Get(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (Keyring) Delete(key string) error {
	err := keyring.Delete(keyringService, key)
	if errors.Is(err, keyring.ErrNotFound) {
		return ErrNotFound
	}
	return err
}
