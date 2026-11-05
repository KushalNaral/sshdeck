package vpn

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func tlsServer(t *testing.T) (host string, port int, cert *x509.Certificate) {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	t.Cleanup(srv.Close)
	h, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	n, _ := strconv.Atoi(p)
	return h, n, srv.Certificate()
}

func TestFetchCertReadsTheGatewaysCertificateAndBothFingerprints(t *testing.T) {
	host, port, cert := tlsServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := FetchCert(ctx, host, port)
	if err != nil {
		t.Fatal(err)
	}
	der := sha256.Sum256(cert.Raw)
	spki := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	if got.SHA256 != hex.EncodeToString(der[:]) {
		t.Errorf("sha256 = %s, want the digest of the certificate (what openfortivpn trusts)", got.SHA256)
	}
	if got.Pin != "pin-sha256:"+base64.StdEncoding.EncodeToString(spki[:]) {
		t.Errorf("pin = %s, want the key pin openconnect trusts", got.Pin)
	}
	if !got.SelfSigned || got.TrustedBySystem {
		t.Errorf("a test certificate is self-signed and not trusted by the system: %+v", got)
	}
	if got.NotAfter.IsZero() || got.Subject == "" {
		t.Errorf("details missing: %+v", got)
	}
	if !serverCertRe.MatchString(got.Pin) {
		t.Errorf("the pin %q would be refused by the helper's own check", got.Pin)
	}
}

func TestFetchCertFormatsTheFingerprintForComparing(t *testing.T) {
	host, port, _ := tlsServer(t)
	got, err := FetchCert(context.Background(), host, port)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(got.Fingerprint, ":")
	if len(parts) != 32 || got.Fingerprint != strings.ToUpper(got.Fingerprint) {
		t.Errorf("fingerprint = %q, want 32 upper-case hex pairs separated by colons", got.Fingerprint)
	}
	if strings.ReplaceAll(strings.ToLower(got.Fingerprint), ":", "") != got.SHA256 {
		t.Error("the readable fingerprint is not the same digest")
	}
}

func TestFetchCertFailsPlainlyWhenNothingListens(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := FetchCert(ctx, "127.0.0.1", port); err == nil || !strings.Contains(err.Error(), "reach") {
		t.Errorf("err = %v, want a message that the gateway could not be reached", err)
	}
}

func TestFetchCertRefusesValuesThatAreNotAHostAndPort(t *testing.T) {
	for _, host := range []string{"", "bad host", "a;b", "-x", "host/path"} {
		if _, err := FetchCert(context.Background(), host, 443); err == nil {
			t.Errorf("host %q was accepted", host)
		}
	}
	if _, err := FetchCert(context.Background(), "vpn.example.org", 0); err == nil {
		t.Error("port 0 was accepted")
	}
}

func TestFetchCertGivesUpWhenTheGatewayNeverAnswers(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0") // accepts the connection but never speaks TLS
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := FetchCert(ctx, "127.0.0.1", l.Addr().(*net.TCPAddr).Port); err == nil || time.Since(start) > 3*time.Second {
		t.Errorf("err = %v after %v: the fetch must respect its deadline", err, time.Since(start))
	}
}

func TestWindowsCommandPrefersTheFetchedPin(t *testing.T) {
	asWindows(t)
	pin := "pin-sha256:" + strings.Repeat("C", 43) + "="
	v := forti
	v.TrustedCert = strings.Repeat("ab", 32)
	v.TrustedPin = pin
	f := &fakeStarter{script: printThenStay("Connected as 10.0.0.2")}
	if err := New(f).Up(ctx(t), v, "pw"); err != nil {
		t.Fatal(err)
	}
	i := indexOf(f.argv, "--servercert")
	if i < 0 || f.argv[i+1] != pin {
		t.Errorf("argv %v, want --servercert %s", f.argv, pin)
	}
}

func TestLinuxOpenconnectFallbackPassesThePin(t *testing.T) {
	withClients(t, "openconnect")
	pin := "pin-sha256:" + strings.Repeat("D", 43) + "="
	v := gp
	v.TrustedPin = pin
	f := &fakeStarter{script: printThenStay("Connected as 10.0.0.2")}
	if err := New(f).Up(ctx(t), v, "pw"); err != nil {
		t.Fatal(err)
	}
	if i := indexOf(f.argv, "--servercert"); i < 0 || f.argv[i+1] != pin {
		t.Errorf("argv %v, want the pin passed to openconnect", f.argv)
	}
}

func indexOf(s []string, x string) int {
	for i, v := range s {
		if v == x {
			return i
		}
	}
	return -1
}
