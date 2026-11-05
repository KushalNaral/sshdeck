// Command sign makes and uses the key that signs sshdeck releases.
//
//	sign keygen            prints a new public key and private key (base64)
//	sign sign FILE         writes FILE.sig, a base64 ed25519 signature, using the key in $UPDATE_SIGNING_KEY
//
// The release workflow runs "sign sign dist/SHA256SUMS"; the app checks that signature with the public key in
// internal/update/key.go before installing an update. Keep the private key out of the repository.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
)

func keygen() (pub, priv string, err error) {
	p, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(p), base64.StdEncoding.EncodeToString(k), nil
}

func signFile(path, privB64 string) error {
	raw, err := base64.StdEncoding.DecodeString(privB64)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		return errors.New("UPDATE_SIGNING_KEY is not a valid private key")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(ed25519.PrivateKey(raw), data))
	return os.WriteFile(path+".sig", []byte(sig+"\n"), 0o644)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: sign keygen | sign sign FILE")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "keygen":
		pub, priv, err := keygen()
		if err != nil {
			fatal(err)
		}
		fmt.Println("public: ", pub)
		fmt.Println("private:", priv)
	case "sign":
		if len(os.Args) != 3 {
			fatal(errors.New("usage: sign sign FILE"))
		}
		if err := signFile(os.Args[2], os.Getenv("UPDATE_SIGNING_KEY")); err != nil {
			fatal(err)
		}
	default:
		fatal(fmt.Errorf("unknown command %q", os.Args[1]))
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "sign:", err)
	os.Exit(1)
}
