package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const csvSample = `type,name,host,port,user,auth,vpn,vpn_kind,password,production,software,server,kind,address
vpn,Gate,gw.example.org,10443,vu,,,forti,vpn-pw,,,,,
server,Imported,10.9.9.9,,deploy,,Gate,,srv-pw,yes,PHP 8.2.25>8.2.35,,,
login,Main DB,,,root,,,,db-pw,,,Imported,db,127.0.0.1:3306
server,,10.9.9.8,,u,,,,,,,,,
`

func writeCSV(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "in.csv")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportPreviewHasNoSecretsAndApplySavesEverything(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	tr := &trail{}
	a.audit = tr.add
	pv, err := a.importFile(writeCSV(t, csvSample))
	if err != nil {
		t.Fatal(err)
	}
	if len(pv.Rows) != 4 || pv.Rows[3].Action != "error" || pv.Rows[3].Error == "" {
		t.Fatalf("preview = %+v", pv.Rows)
	}
	for _, r := range pv.Rows[:3] {
		if r.Action != "new" {
			t.Errorf("row %+v should be new", r)
		}
	}
	if !pv.Rows[1].HasPassword {
		t.Error("server row should say it has a password")
	}
	res, err := a.ApplyImport(pv.Token, []int{0, 1, 2, 3})
	if err != nil || res.Added != 3 || res.Updated != 0 || len(res.Failures) != 0 {
		t.Fatalf("apply = %+v %v", res, err)
	}
	var found bool
	for _, s := range a.st.Servers() {
		if s.Name == "Imported" {
			found = true
			if s.VPNID == "" || !s.Production || len(s.Software) != 1 || len(s.Logins) != 1 {
				t.Errorf("server = %+v", s)
			}
			if pw, _ := a.st.Password("server", s.ID); pw != "srv-pw" {
				t.Errorf("password = %q", pw)
			}
			if pw, _ := a.st.Password("login", s.ID+"/"+s.Logins[0].ID); pw != "db-pw" {
				t.Errorf("login secret = %q", pw)
			}
		}
	}
	if !found {
		t.Fatal("server not imported")
	}
	if strings.Contains(tr.all(), "pw") && strings.Contains(tr.all(), "srv-pw") {
		t.Error("secret in audit trail")
	}
	if tr.find("import") == nil {
		t.Error("import not audited")
	}
	if _, err := a.ApplyImport(pv.Token, []int{0}); err == nil {
		t.Error("a token must work once")
	}
}

func TestReimportUpdatesAndBlankPasswordKeepsTheStoredOne(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	first, _ := a.importFile(writeCSV(t, csvSample))
	if _, err := a.ApplyImport(first.Token, []int{0, 1, 2}); err != nil {
		t.Fatal(err)
	}
	n := len(a.st.Servers())
	again := strings.ReplaceAll(csvSample, "srv-pw", "") // same rows, no passwords
	again = strings.ReplaceAll(again, "10.9.9.9", "10.9.9.7")
	pv, _ := a.importFile(writeCSV(t, again))
	for _, r := range pv.Rows[:3] {
		if r.Action != "update" {
			t.Errorf("row %+v should be update", r)
		}
	}
	res, err := a.ApplyImport(pv.Token, []int{0, 1, 2})
	if err != nil || res.Updated != 3 || res.Added != 0 {
		t.Fatalf("apply = %+v %v", res, err)
	}
	if len(a.st.Servers()) != n {
		t.Errorf("servers %d -> %d: re-import must not duplicate", n, len(a.st.Servers()))
	}
	for _, s := range a.st.Servers() {
		if s.Name == "Imported" {
			if s.Host != "10.9.9.7" {
				t.Errorf("host = %s", s.Host)
			}
			if pw, _ := a.st.Password("server", s.ID); pw != "srv-pw" {
				t.Errorf("blank password cell replaced the stored one: %q", pw)
			}
			if len(s.Logins) != 1 {
				t.Errorf("logins = %+v", s.Logins)
			}
		}
	}
}

func TestOnlyPickedRowsAreImportedAndUnknownVPNIsReported(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	body := "type,name,host,user,vpn\nserver,One,h1.example.org,u,Nope\nserver,Two,h2.example.org,u,\n"
	pv, _ := a.importFile(writeCSV(t, body))
	res, err := a.ApplyImport(pv.Token, []int{0})
	if err != nil || res.Added != 0 || len(res.Failures) != 1 || !strings.Contains(res.Failures[0], "Nope") {
		t.Fatalf("apply = %+v %v", res, err)
	}
	for _, s := range a.st.Servers() {
		if s.Name == "Two" {
			t.Error("an unpicked row was imported")
		}
	}
}

func TestExportWritesNoPasswordsAndMode0600(t *testing.T) {
	a, _, _ := rig(t, okOpen(newShell()))
	if _, err := a.SaveLogin("direct", loginFor("Main DB"), "db-secret-7"); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "out.csv")
	if err := a.exportTo(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	for _, secret := range []string{"pw", "vpw", "db-secret-7"} {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, ","+secret+",") {
				t.Errorf("export contains secret %q: %s", secret, line)
			}
		}
	}
	if !strings.Contains(string(b), "Main DB") {
		t.Error("login missing from export")
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
}
