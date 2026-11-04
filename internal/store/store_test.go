package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type memSecrets map[string]string

func (m memSecrets) Set(k, v string) error { m[k] = v; return nil }
func (m memSecrets) Get(k string) (string, error) {
	v, ok := m[k]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}
func (m memSecrets) Delete(k string) error {
	if _, ok := m[k]; !ok {
		return ErrNotFound
	}
	delete(m, k)
	return nil
}

func open(t *testing.T) (*Store, memSecrets, string) {
	t.Helper()
	sec := memSecrets{}
	path := filepath.Join(t.TempDir(), "data.json")
	s, err := Open(path, sec)
	if err != nil {
		t.Fatal(err)
	}
	return s, sec, path
}

var good = Server{Name: "Atlas Server", Host: "10.0.0.5", Port: 22, User: "deploy", Auth: "password"}

func TestPasswordsStayOutOfTheFile(t *testing.T) {
	s, sec, path := open(t)
	sv, err := s.SaveServer(good, "hunter2-secret")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "hunter2-secret") {
		t.Fatal("password leaked into data file")
	}
	if got, _ := s.Password("server", sv.ID); got != "hunter2-secret" {
		t.Fatalf("password not retrievable, got %q", got)
	}
	if sec["server/"+sv.ID] == "" {
		t.Fatal("password not stored in secrets backend")
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("data file mode %v, want 0600", fi.Mode().Perm())
	}
}

func TestPersistsAcrossReopenAndBlankPasswordKeepsOld(t *testing.T) {
	s, sec, path := open(t)
	sv, _ := s.SaveServer(good, "first")
	sv.Name = "Atlas Renamed"
	if _, err := s.SaveServer(sv, ""); err != nil {
		t.Fatal(err)
	}
	if sec["server/"+sv.ID] != "first" {
		t.Fatal("blank password should keep the stored one")
	}
	s2, err := Open(path, sec)
	if err != nil {
		t.Fatal(err)
	}
	if got := s2.Servers(); len(got) != 1 || got[0].Name != "Atlas Renamed" {
		t.Fatalf("reopen lost data: %+v", got)
	}
}

func TestValidation(t *testing.T) {
	s, _, _ := open(t)
	for name, mut := range map[string]func(*Server){
		"no name":   func(x *Server) { x.Name = "" },
		"no host":   func(x *Server) { x.Host = "" },
		"no user":   func(x *Server) { x.User = "" },
		"bad port":  func(x *Server) { x.Port = 70000 },
		"bad auth":  func(x *Server) { x.Auth = "magic" },
		"bad vpn":   func(x *Server) { x.VPNID = "nope" },
		"bad stage": func(x *Server) { x.Stage = 9 },
	} {
		sv := good
		mut(&sv)
		if _, err := s.SaveServer(sv, ""); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: want ErrInvalid, got %v", name, err)
		}
	}
}

func TestVPNInUseCannotBeDeleted(t *testing.T) {
	s, sec, _ := open(t)
	v, err := s.SaveVPN(VPN{Name: "HQ FortiClient", Kind: Forti, Gateway: "203.0.113.10", Port: 10443}, "vpnpw")
	if err != nil {
		t.Fatal(err)
	}
	sv := good
	sv.VPNID = v.ID
	sv, _ = s.SaveServer(sv, "")
	var inUse VPNInUseError
	if err := s.DeleteVPN(v.ID); !errors.As(err, &inUse) || inUse.Servers != 1 {
		t.Fatalf("want VPNInUseError{1}, got %v", err)
	}
	if err := s.DeleteServer(sv.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteVPN(v.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := sec["vpn/"+v.ID]; ok {
		t.Fatal("deleting a VPN must delete its stored password")
	}
}

func TestChoosingAVPNClearsVPNRequired(t *testing.T) {
	s, _, _ := open(t)
	v, _ := s.SaveVPN(VPN{Name: "GP", Kind: GP, Gateway: "gp.example.gov", Port: 443}, "")
	sv := good
	sv.VPNRequired = true
	sv.VPNID = v.ID
	got, _ := s.SaveServer(sv, "")
	if got.VPNRequired {
		t.Fatal("VPNRequired should clear once a VPN is chosen")
	}
}
