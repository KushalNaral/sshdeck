package main

import (
	"context"
	"time"

	"github.com/KushalNaral/sshdeck/internal/vpn"
)

// FetchGatewayCert connects to a VPN gateway, reads the certificate it presents and returns the details for the user to
// check. Nothing is stored: the UI fills in the profile's trusted certificate only after the user approves it.
func (a *App) FetchGatewayCert(host string, port int) (vpn.CertInfo, error) {
	if err := a.guard(); err != nil {
		return vpn.CertInfo{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer cancel()
	info, err := vpn.FetchCert(ctx, host, port)
	if err == nil {
		a.audit("vpn.cert.fetch", host, "")
	}
	return info, err
}
