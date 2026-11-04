// Package vault seals data under a passphrase: Argon2id turns the passphrase into a key and
// XChaCha20-Poly1305 encrypts and authenticates the data. The file header (format, parameters, salt)
// is authenticated too, so changing any byte of the file makes it refuse to open.
package vault

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"sync"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	magic     = "SSHDECK1"
	headerLen = len(magic) + 4 + 4 + 1 + 16 // magic, time, memory (KiB), threads, salt
	saltLen   = 16
)

// Params are the Argon2id cost settings. They are written into every file, so they can be raised later.
type Params struct {
	Time    uint32
	Memory  uint32 // KiB
	Threads uint8
}

// Default costs about a tenth of a second and 64 MiB per unlock. Tests lower it.
var Default = Params{Time: 3, Memory: 64 * 1024, Threads: 4}

var (
	ErrWrongPassphrase = errors.New("wrong passphrase, or the vault file is damaged")
	ErrNotVault        = errors.New("not a sshdeck vault file")
	ErrWiped           = errors.New("the vault key was wiped")
)

// Key is a derived key plus what is needed to write the same header again. Seal on it costs no key derivation.
type Key struct {
	mu   sync.Mutex
	key  [32]byte
	salt [saltLen]byte
	p    Params
	dead bool
}

func derive(pass string, salt []byte, p Params) [32]byte {
	var k [32]byte
	copy(k[:], argon2.IDKey([]byte(pass), salt, p.Time, p.Memory, p.Threads, 32))
	return k
}

// NewKey derives a key for a new passphrase with a fresh random salt.
func NewKey(pass string) (*Key, error) {
	k := &Key{p: Default}
	if _, err := rand.Read(k.salt[:]); err != nil {
		return nil, err
	}
	k.key = derive(pass, k.salt[:], k.p)
	return k, nil
}

// IsVault reports whether blob starts like a vault file.
func IsVault(blob []byte) bool { return len(blob) >= len(magic) && string(blob[:len(magic)]) == magic }

func (k *Key) header() []byte {
	h := make([]byte, headerLen)
	copy(h, magic)
	binary.BigEndian.PutUint32(h[8:], k.p.Time)
	binary.BigEndian.PutUint32(h[12:], k.p.Memory)
	h[16] = k.p.Threads
	copy(h[17:], k.salt[:])
	return h
}

// Seal encrypts plain with a fresh nonce.
func (k *Key) Seal(plain []byte) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.dead {
		return nil, ErrWiped
	}
	aead, err := chacha20poly1305.NewX(k.key[:])
	if err != nil {
		return nil, err
	}
	h := k.header()
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	out := append(h, nonce...)
	return aead.Seal(out, nonce, plain, h), nil
}

// Zero wipes the key from memory; the Key cannot seal afterwards.
func (k *Key) Zero() {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.key = [32]byte{}
	k.dead = true
}

// Open derives the key from pass and decrypts blob. The returned Key can seal new versions of the file.
func Open(pass string, blob []byte) (*Key, []byte, error) {
	if !IsVault(blob) {
		return nil, nil, ErrNotVault
	}
	if len(blob) < headerLen+chacha20poly1305.NonceSizeX+chacha20poly1305.Overhead {
		return nil, nil, ErrWrongPassphrase
	}
	k := &Key{p: Params{
		Time:    binary.BigEndian.Uint32(blob[8:]),
		Memory:  binary.BigEndian.Uint32(blob[12:]),
		Threads: blob[16],
	}}
	// A crafted file must not make us allocate gigabytes.
	if k.p.Time < 1 || k.p.Time > 32 || k.p.Memory < 8 || k.p.Memory > 1<<20 || k.p.Threads < 1 {
		return nil, nil, ErrWrongPassphrase
	}
	copy(k.salt[:], blob[17:headerLen])
	k.key = derive(pass, k.salt[:], k.p)
	aead, err := chacha20poly1305.NewX(k.key[:])
	if err != nil {
		return nil, nil, err
	}
	nonce := blob[headerLen : headerLen+aead.NonceSize()]
	plain, err := aead.Open(nil, nonce, blob[headerLen+aead.NonceSize():], blob[:headerLen])
	if err != nil {
		return nil, nil, ErrWrongPassphrase
	}
	return k, plain, nil
}
