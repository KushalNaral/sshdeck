package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
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
		"no name":  func(x *Server) { x.Name = "" },
		"no host":  func(x *Server) { x.Host = "" },
		"no user":  func(x *Server) { x.User = "" },
		"bad port": func(x *Server) { x.Port = 70000 },
		"bad auth": func(x *Server) { x.Auth = "magic" },
		"bad vpn":  func(x *Server) { x.VPNID = "nope" },
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

func TestTrustedCertMustBeASha256Digest(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "d.json"), memSecrets{})
	v := VPN{Name: "x", Kind: Forti, Gateway: "g", Port: 1, User: "u"}
	v.TrustedCert = "not-a-digest"
	if _, err := s.SaveVPN(v, ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want ErrInvalid, got %v", err)
	}
	v.TrustedCert = strings.Repeat("ab", 32)
	if _, err := s.SaveVPN(v, ""); err != nil {
		t.Fatal(err)
	}
}

func TestTaskValidationAndRoundTrip(t *testing.T) {
	s, _, path := open(t)
	if _, err := s.SaveTask(Task{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty title: %v", err)
	}
	if _, err := s.SaveTask(Task{Title: "x", Status: "later"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad status: %v", err)
	}
	if _, err := s.SaveTask(Task{Title: "x", ServerIDs: []string{"nope"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown server: %v", err)
	}
	if _, err := s.SaveTask(Task{Title: "x", Due: "next week"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad due date: %v", err)
	}
	sv, _ := s.SaveServer(good, "pw")
	tk, err := s.SaveTask(Task{Title: " Upgrade PHP ", Tags: []string{"PHP", "php", " urgent ", ""}, ServerIDs: []string{sv.ID}, Due: "2026-12-01"})
	if err != nil {
		t.Fatal(err)
	}
	if tk.ID == "" || tk.Status != "todo" || tk.Title != "Upgrade PHP" || strings.Join(tk.Tags, ",") != "php,urgent" {
		t.Fatalf("not normalised: %+v", tk)
	}
	s2, err := Open(path, memSecrets{})
	if err != nil || len(s2.Tasks()) != 1 {
		t.Fatalf("reopen: %v %v", err, s2.Tasks())
	}
}

func TestDeletingAServerUnlinksItsTasks(t *testing.T) {
	s, _, _ := open(t)
	sv, _ := s.SaveServer(good, "pw")
	tk, _ := s.SaveTask(Task{Title: "Patch", ServerIDs: []string{sv.ID}})
	if err := s.DeleteServer(sv.ID); err != nil {
		t.Fatal(err)
	}
	if got := s.Tasks(); len(got) != 1 || got[0].ID != tk.ID || len(got[0].ServerIDs) != 0 {
		t.Fatalf("task should stay, unlinked: %+v", got)
	}
	if err := s.DeleteTask(tk.ID); err != nil || len(s.Tasks()) != 0 {
		t.Fatalf("delete: %v", err)
	}
	if err := s.DeleteTask(tk.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestFieldsThatCouldInjectIntoAClientAreRefused(t *testing.T) {
	s, _, _ := open(t)
	vpn := VPN{Name: "V", Kind: Forti, Gateway: "gw.example.org", Port: 443, User: "u"}
	for name, mut := range map[string]func(v *VPN){
		"gateway starts with a dash": func(v *VPN) { v.Gateway = "--script=/tmp/x" },
		"gateway has a space":        func(v *VPN) { v.Gateway = "gw example" },
		"gateway has a newline":      func(v *VPN) { v.Gateway = "gw\nhost = evil" },
		"user has a newline":         func(v *VPN) { v.User = "u\npppd-plugin = x" },
		"user starts with a dash":    func(v *VPN) { v.User = "-u" },
	} {
		v := vpn
		mut(&v)
		if _, err := s.SaveVPN(v, "pw"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: saved (%v)", name, err)
		}
	}
	if _, err := s.SaveVPN(vpn, "line1\nline2"); !errors.Is(err, ErrInvalid) {
		t.Errorf("a VPN password with a newline was accepted: %v", err)
	}
	if _, err := s.SaveVPN(vpn, "p@ss w0rd!#'\"$"); err != nil {
		t.Errorf("an ordinary password with symbols must stay valid: %v", err)
	}
	for name, mut := range map[string]func(sv *Server){
		"host starts with a dash": func(sv *Server) { sv.Host = "-oProxyCommand=x" },
		"host has a space":        func(sv *Server) { sv.Host = "a b" },
		"user has a newline":      func(sv *Server) { sv.User = "root\nx" },
		"user starts with a dash": func(sv *Server) { sv.User = "-l" },
	} {
		sv := good
		mut(&sv)
		if _, err := s.SaveServer(sv, "pw"); !errors.Is(err, ErrInvalid) {
			t.Errorf("server, %s: saved (%v)", name, err)
		}
	}
	for _, host := range []string{"10.0.0.5", "web-1.example.org", "fe80::1", "host_name"} {
		sv := good
		sv.Host = host
		if _, err := s.SaveServer(sv, "pw"); err != nil {
			t.Errorf("host %q should be valid: %v", host, err)
		}
	}
}

func TestProductionFlagRoundTrips(t *testing.T) {
	s, _, path := open(t)
	sv := good
	sv.Production = true
	saved, err := s.SaveServer(sv, "pw")
	if err != nil || !saved.Production {
		t.Fatalf("%+v %v", saved, err)
	}
	s2, err := Open(path, memSecrets{})
	if err != nil || !s2.Servers()[0].Production {
		t.Fatalf("production flag lost on reopen: %v", err)
	}
	other := good
	other.Name = "Staging"
	if saved2, _ := s.SaveServer(other, "pw"); saved2.Production {
		t.Error("a server defaults to not production")
	}
}

func TestSoftwareRoundTripsAndIsValidated(t *testing.T) {
	s, _, _ := open(t)
	sv := Server{Name: "A", Host: "a.example.com", Port: 22, User: "u", Auth: "password",
		Software: []Software{{Name: "PHP", Version: "8.2.25", Target: "8.2.35"}, {Name: "Node", Version: "18.19"}}}
	saved, err := s.SaveServer(sv, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Servers()[0].Software; len(got) != 2 || got[0].Target != "8.2.35" {
		t.Fatalf("software = %+v", got)
	}
	sv = saved
	sv.Software = []Software{{Name: ""}}
	if _, err := s.SaveServer(sv, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty software name: err = %v, want ErrInvalid", err)
	}
	sv.Software = []Software{{Name: "PHP\n", Version: "1"}}
	if _, err := s.SaveServer(sv, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("control character in name: err = %v, want ErrInvalid", err)
	}
	sv.Software = make([]Software, 51)
	for i := range sv.Software {
		sv.Software[i] = Software{Name: "x"}
	}
	if _, err := s.SaveServer(sv, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("51 entries: err = %v, want ErrInvalid", err)
	}
}

func TestOldPHPAndWebserverFieldsBecomeSoftware(t *testing.T) {
	var sv Server
	old := `{"id":"x","name":"X","host":"h","port":22,"user":"u","auth":"password","vpnId":"","php":"8.2.25","phpTarget":"8.2.35","webserver":"Nginx 1.18.0"}`
	if err := json.Unmarshal([]byte(old), &sv); err != nil {
		t.Fatal(err)
	}
	want := []Software{{"PHP", "8.2.25", "8.2.35"}, {"Nginx", "1.18.0", ""}}
	if !reflect.DeepEqual(sv.Software, want) {
		t.Fatalf("software = %+v, want %+v", sv.Software, want)
	}
	b, _ := json.Marshal(sv)
	if strings.Contains(string(b), `"php"`) || strings.Contains(string(b), "webserver") {
		t.Errorf("old fields written back: %s", b)
	}
}

func TestLoginsKeepTheirSecretsOutOfTheFileAndGoWithTheServer(t *testing.T) {
	s, sec, path := open(t)
	sv, err := s.SaveServer(good, "ssh-pw")
	if err != nil {
		t.Fatal(err)
	}
	l, err := s.SaveLogin(sv.ID, Login{Label: "Main DB", Kind: "db", User: "root", Address: "127.0.0.1:3306"}, "db-secret")
	if err != nil || l.ID == "" {
		t.Fatalf("save login: %+v %v", l, err)
	}
	if pw, err := s.Password("login", sv.ID+"/"+l.ID); err != nil || pw != "db-secret" {
		t.Fatalf("secret = %q %v", pw, err)
	}
	if raw, _ := os.ReadFile(path); strings.Contains(string(raw), "db-secret") || !strings.Contains(string(raw), "Main DB") {
		t.Errorf("file should hold the label but not the secret: %s", raw)
	}
	// An empty secret keeps the stored one; editing the server leaves its logins alone.
	if _, err := s.SaveLogin(sv.ID, Login{ID: l.ID, Label: "Main DB", Kind: "db", User: "admin"}, ""); err != nil {
		t.Fatal(err)
	}
	if pw, _ := s.Password("login", sv.ID+"/"+l.ID); pw != "db-secret" {
		t.Errorf("blank secret replaced the stored one: %q", pw)
	}
	edited := sv
	edited.Logins = nil
	if _, err := s.SaveServer(edited, ""); err != nil {
		t.Fatal(err)
	}
	if got := s.Servers()[0].Logins; len(got) != 1 || got[0].User != "admin" {
		t.Fatalf("logins after editing the server = %+v", got)
	}
	// Deleting the login, and then the server, removes the secrets.
	if err := s.DeleteLogin(sv.ID, l.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := sec["login/"+sv.ID+"/"+l.ID]; ok {
		t.Error("login secret survived DeleteLogin")
	}
	l2, _ := s.SaveLogin(sv.ID, Login{Label: "cPanel", Kind: "cpanel", User: "c"}, "cp")
	if err := s.DeleteServer(sv.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := sec["login/"+sv.ID+"/"+l2.ID]; ok {
		t.Error("login secret survived DeleteServer")
	}
}

func TestLoginValidation(t *testing.T) {
	s, _, _ := open(t)
	sv, _ := s.SaveServer(good, "pw")
	for name, l := range map[string]Login{
		"no label":      {Kind: "db"},
		"unknown kind":  {Label: "x", Kind: "ftp"},
		"control chars": {Label: "x\n", Kind: "db"},
		"control user":  {Label: "x", Kind: "db", User: "a\tb"},
	} {
		if _, err := s.SaveLogin(sv.ID, l, "s"); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := s.SaveLogin("nope", Login{Label: "x", Kind: "db"}, "s"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown server: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteLogin(sv.ID, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown login: err = %v, want ErrNotFound", err)
	}
}

func TestTrustedCertAcceptsAnOpenconnectPin(t *testing.T) {
	v := VPN{Name: "G", Kind: GP, Gateway: "gw.example.org", Port: 443, User: "u"}
	for cert, ok := range map[string]bool{
		"pin-sha256:" + strings.Repeat("A", 43) + "=":            true,
		strings.Repeat("ab", 32):                                 true,
		"pin-sha256:short":                                       false,
		"pin-sha256:" + strings.Repeat("A", 43) + "= --script x": false,
	} {
		v.TrustedCert = cert
		if err := ValidateVPN(v, ""); (err == nil) != ok {
			t.Errorf("cert %q: err = %v, want ok=%v", cert, err, ok)
		}
	}
}
