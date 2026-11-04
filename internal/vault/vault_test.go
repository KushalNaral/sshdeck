package vault

import (
	"bytes"
	"errors"
	"testing"
)

func init() { Default = Params{Time: 1, Memory: 8, Threads: 1} } // fast; real parameters are for real use

func TestRoundTripAndKeyReuse(t *testing.T) {
	k, err := NewKey("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	blob, err := k.Seal([]byte(`{"servers":[{"host":"10.9.8.7"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte("10.9.8.7")) || bytes.Contains(blob, []byte("servers")) {
		t.Fatal("plaintext visible in the sealed file")
	}
	k2, plain, err := Open("correct horse", blob)
	if err != nil || string(plain) != `{"servers":[{"host":"10.9.8.7"}]}` {
		t.Fatalf("open: %v %q", err, plain)
	}
	// The key from Open can seal again without paying for key derivation, and the result still opens.
	blob2, err := k2.Seal([]byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if _, p, err := Open("correct horse", blob2); err != nil || string(p) != "second" {
		t.Fatalf("reseal: %v %q", err, p)
	}
	if bytes.Equal(blob, blob2) {
		t.Fatal("two seals must differ (fresh nonce)")
	}
}

func TestWrongPassphraseAndTampering(t *testing.T) {
	k, _ := NewKey("right")
	blob, _ := k.Seal([]byte("secret data"))
	if _, _, err := Open("wrong", blob); !errors.Is(err, ErrWrongPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	for _, i := range []int{10, 14, 20, 40, len(blob) - 1} { // params, salt, nonce, ciphertext, tag
		bad := append([]byte(nil), blob...)
		bad[i] ^= 0x01
		if _, _, err := Open("right", bad); err == nil {
			t.Errorf("flipping byte %d went unnoticed", i)
		}
	}
}

func TestNotAVaultAndTruncated(t *testing.T) {
	if IsVault([]byte(`{"servers":[]}`)) {
		t.Fatal("plain JSON is not a vault")
	}
	k, _ := NewKey("p")
	blob, _ := k.Seal([]byte("x"))
	if !IsVault(blob) {
		t.Fatal("sealed blob not recognised")
	}
	if _, _, err := Open("p", []byte(`{"servers":[]}`)); !errors.Is(err, ErrNotVault) {
		t.Fatalf("got %v", err)
	}
	for n := 0; n < len(blob); n += 7 {
		if _, _, err := Open("p", blob[:n]); err == nil {
			t.Errorf("truncated to %d opened", n)
		}
	}
}

func TestCraftedParametersCannotExhaustMemory(t *testing.T) {
	k, _ := NewKey("p")
	blob, _ := k.Seal([]byte("x"))
	bad := append([]byte(nil), blob...)
	bad[12], bad[13], bad[14], bad[15] = 0xff, 0xff, 0xff, 0x7f // memory field: absurdly large
	if _, _, err := Open("p", bad); err == nil {
		t.Fatal("accepted absurd memory parameter")
	}
}

func TestZeroWipesTheKey(t *testing.T) {
	k, _ := NewKey("p")
	k.Zero()
	if _, err := k.Seal([]byte("x")); err == nil {
		t.Fatal("a wiped key must not seal")
	}
}
