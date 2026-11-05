package update

import (
	"crypto/ed25519"
	"encoding/base64"
)

// publicKeyB64 is the key that releases must be signed with (see cmd/sign). To rotate it, make a new key with
// "go run ./cmd/sign keygen", replace this value, and set the UPDATE_SIGNING_KEY secret; the first release after a
// rotation has to be installed by hand, since running copies still trust the old key.
const publicKeyB64 = "+KC0U3hpHf/gYI3+zYlPw7I/m7KZBx+2MmebsRIC8aA="

// PublicKey returns the release signing key.
func PublicKey() ed25519.PublicKey {
	k, _ := base64.StdEncoding.DecodeString(publicKeyB64)
	return ed25519.PublicKey(k)
}
