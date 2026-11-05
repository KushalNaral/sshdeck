package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestFetchGatewayCertIsAuditedAndGuardedByTheLock(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	tr := &trail{}
	a.audit = tr.add
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	host, p, _ := net.SplitHostPort(srv.Listener.Addr().String())
	port, _ := strconv.Atoi(p)
	info, err := a.FetchGatewayCert(host, port)
	if err != nil || info.SHA256 == "" {
		t.Fatalf("%+v %v", info, err)
	}
	if e := tr.find("vpn.cert.fetch"); e == nil || e.Subject != host {
		t.Errorf("audit = %+v", e)
	}
	if _, err := a.FetchGatewayCert("bad host", 443); err == nil {
		t.Error("a bad host was accepted")
	}
}
