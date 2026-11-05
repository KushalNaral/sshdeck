package vpn

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// CertInfo describes the certificate a VPN gateway presents, for the user to check before trusting it.
type CertInfo struct {
	Subject         string    `json:"subject"`
	Issuer          string    `json:"issuer"`
	Names           []string  `json:"names"`
	NotBefore       time.Time `json:"notBefore"`
	NotAfter        time.Time `json:"notAfter"`
	Expired         bool      `json:"expired"`
	SelfSigned      bool      `json:"selfSigned"`
	TrustedBySystem bool      `json:"trustedBySystem"` // this computer already accepts it, so pinning is optional
	SHA256          string    `json:"sha256"`          // digest of the certificate: what openfortivpn's trusted-cert takes
	Pin             string    `json:"pin"`             // pin-sha256 of the public key: what openconnect's --servercert takes
	Fingerprint     string    `json:"fingerprint"`     // the same digest as SHA256, in the form administrators publish
}

// FetchCert connects to host:port, reads the certificate it presents and describes it. It does not log in, send
// anything, or decide whether to trust it: that is the user's call, made by comparing the fingerprint with one from
// whoever runs the gateway. Trusting a certificate seen only through this connection protects against later changes,
// not against someone who is already in the way now.
func FetchCert(ctx context.Context, host string, port int) (CertInfo, error) {
	var info CertInfo
	if strings.TrimSpace(host) == "" {
		return info, errors.New("enter the gateway address first")
	}
	if port < 1 || port > 65535 {
		return info, errors.New("port must be from 1 to 65535")
	}
	if err := store.CheckVPN(store.VPN{Gateway: host, Port: port, User: "x"}, ""); err != nil {
		return info, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cfg := &tls.Config{InsecureSkipVerify: true} // only to read the certificate; whether to trust it is decided later
	isIP := net.ParseIP(host) != nil
	if !isIP {
		cfg.ServerName = host
	}
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 10 * time.Second}, Config: cfg}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(port)))
	if err != nil {
		return info, fmt.Errorf("cannot reach %s:%d over TLS: %w", host, port, err)
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return info, errors.New("the gateway presented no certificate")
	}
	leaf := certs[0]

	der := sha256.Sum256(leaf.Raw)
	spki := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	info.SHA256 = hex.EncodeToString(der[:])
	info.Pin = "pin-sha256:" + base64.StdEncoding.EncodeToString(spki[:])
	pairs := make([]string, len(der))
	for i, b := range der {
		pairs[i] = strings.ToUpper(hex.EncodeToString([]byte{b}))
	}
	info.Fingerprint = strings.Join(pairs, ":")
	info.Subject, info.Issuer = leaf.Subject.String(), leaf.Issuer.String()
	info.NotBefore, info.NotAfter = leaf.NotBefore, leaf.NotAfter
	info.Expired = time.Now().After(leaf.NotAfter)
	info.Names = append(info.Names, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		info.Names = append(info.Names, ip.String())
	}
	info.SelfSigned = leaf.Subject.String() == leaf.Issuer.String() && leaf.CheckSignatureFrom(leaf) == nil
	pool := x509.NewCertPool()
	for _, c := range certs[1:] {
		pool.AddCert(c)
	}
	opts := x509.VerifyOptions{Intermediates: pool}
	if !isIP {
		opts.DNSName = host
	}
	_, verr := leaf.Verify(opts)
	info.TrustedBySystem = verr == nil
	return info, nil
}
