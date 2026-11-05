package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSignedFileVerifiesWithThePublicKey(t *testing.T) {
	pub, priv, err := keygen()
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "SHA256SUMS")
	os.WriteFile(f, []byte("abc  file\n"), 0o644)
	if err := signFile(f, priv); err != nil {
		t.Fatal(err)
	}
	sig, _ := os.ReadFile(f + ".sig")
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	pk, _ := base64.StdEncoding.DecodeString(pub)
	if !ed25519.Verify(pk, []byte("abc  file\n"), raw) {
		t.Error("the signature does not verify")
	}
	os.WriteFile(f, []byte("tampered\n"), 0o644)
	data, _ := os.ReadFile(f)
	if ed25519.Verify(pk, data, raw) {
		t.Error("a changed file still verifies")
	}
}

func TestBadKeyIsRefused(t *testing.T) {
	if err := signFile("x", "not-a-key"); err == nil {
		t.Error("a bad key was accepted")
	}
}
