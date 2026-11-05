package importer

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/KushalNaral/sshdeck/internal/store"
)

const sample = `type,name,host,port,user,auth,key_path,vpn,vpn_kind,trusted_cert,password,production,projects,software,server,kind,address,note
vpn,HQ Forti,gw.example.org,10443,vu,,,,forti,,vpn-secret,,,,,,,
server,Atlas,10.0.0.5,,deploy,,,HQ Forti,,,srv-secret,yes,Site A; Site B,PHP 8.2.25>8.2.35; Node 18.19,,,,Runs Jenkins
login,Main DB,,,root,,,,,,db-secret,,,,Atlas,db,127.0.0.1:3306,
`

func TestParseSample(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader(sample))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d", len(rows))
	}
	for _, r := range rows {
		if r.Err != "" {
			t.Fatalf("line %d: %s", r.Line, r.Err)
		}
	}
	v := rows[0]
	if v.Type != "vpn" || v.VPN.Gateway != "gw.example.org" || v.VPN.Kind != store.Forti || v.VPN.Port != 10443 || v.Secret != "vpn-secret" {
		t.Errorf("vpn row = %+v %+v", v, v.VPN)
	}
	s := rows[1]
	if s.Server.Port != 22 || s.Server.Auth != "password" || !s.Server.Production || s.VPNName != "HQ Forti" || s.Secret != "srv-secret" {
		t.Errorf("server row = %+v %+v", s, s.Server)
	}
	if !reflect.DeepEqual(s.Server.Projects, []string{"Site A", "Site B"}) {
		t.Errorf("projects = %v", s.Server.Projects)
	}
	if want := []store.Software{{Name: "PHP", Version: "8.2.25", Target: "8.2.35"}, {Name: "Node", Version: "18.19"}}; !reflect.DeepEqual(s.Server.Software, want) {
		t.Errorf("software = %+v", s.Server.Software)
	}
	l := rows[2]
	if l.ServerName != "Atlas" || l.Login.Kind != "db" || l.Login.Address != "127.0.0.1:3306" || l.Secret != "db-secret" {
		t.Errorf("login row = %+v %+v", l, l.Login)
	}
}

func TestBadRowsReportLineAndReasonAndTheRestStillParse(t *testing.T) {
	in := "type,name,host,port,user\n" +
		"server,,h.example.org,22,u\n" + // line 2: no name
		"server,Ok,h.example.org,22,u\n" + // line 3
		"server,Bad port,h.example.org,99999,u\n" + // line 4
		"vpn,V,gw,443,u\n" + // line 5: no vpn_kind column at all
		"cat,X,,,\n" // line 6: unknown type
	rows, err := ParseCSV(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	var bad []int
	for _, r := range rows {
		if r.Err != "" {
			bad = append(bad, r.Line)
		}
	}
	if !reflect.DeepEqual(bad, []int{2, 4, 5, 6}) {
		t.Fatalf("bad lines = %v, rows %+v", bad, rows)
	}
	if rows[0].Err == "" || !strings.Contains(rows[0].Err, "name") {
		t.Errorf("reason = %q", rows[0].Err)
	}
}

func TestHeaderMatchingIsLenientAndNeedsType(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader("Type, Name ,HOST,User,Extra\nserver,A,a.example.org,u,ignored\n"))
	if err != nil || len(rows) != 1 || rows[0].Err != "" {
		t.Fatalf("%v %+v", err, rows)
	}
	if _, err := ParseCSV(strings.NewReader("name,host\nA,b\n")); err == nil {
		t.Error("a file without a type column must be refused")
	}
	if _, err := ParseCSV(strings.NewReader("")); err == nil {
		t.Error("an empty file must be refused")
	}
}

func TestExportNeverContainsPasswordsAndRoundTrips(t *testing.T) {
	vpns := []store.VPN{{ID: "v1", Name: "HQ Forti", Kind: store.Forti, Gateway: "gw.example.org", Port: 10443, User: "vu"}}
	servers := []store.Server{{ID: "s1", Name: "Atlas", Host: "10.0.0.5", Port: 22, User: "deploy", Auth: "password", VPNID: "v1", Production: true,
		Projects: []string{"A", "B"}, Software: []store.Software{{Name: "PHP", Version: "8.2.25", Target: "8.2.35"}},
		Logins: []store.Login{{ID: "l1", Label: "Main DB", Kind: "db", User: "root", Address: "127.0.0.1:3306"}}}}
	var buf bytes.Buffer
	if err := WriteCSV(&buf, vpns, servers); err != nil {
		t.Fatal(err)
	}
	rows, err := ParseCSV(&buf)
	if err != nil || len(rows) != 3 {
		t.Fatalf("%v %d rows", err, len(rows))
	}
	for _, r := range rows {
		if r.Err != "" || r.Secret != "" {
			t.Errorf("row %+v", r)
		}
	}
	if rows[1].VPNName != "HQ Forti" || !rows[1].Server.Production || rows[1].Server.Software[0].Target != "8.2.35" || rows[2].ServerName != "Atlas" {
		t.Errorf("round trip lost data: %+v", rows)
	}
}

func TestTemplateParses(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteTemplate(&buf); err != nil {
		t.Fatal(err)
	}
	rows, err := ParseCSV(&buf)
	if err != nil || len(rows) != 3 {
		t.Fatalf("%v %d", err, len(rows))
	}
	for _, r := range rows {
		if r.Err != "" {
			t.Errorf("template row %d: %s", r.Line, r.Err)
		}
	}
}

func TestAutoConnectColumn(t *testing.T) {
	rows, err := ParseCSV(strings.NewReader("type,name,host,user,auto_connect\nserver,A,a.example.org,u,yes\nserver,B,b.example.org,u,\n"))
	if err != nil || !rows[0].Server.AutoConnect || rows[1].Server.AutoConnect {
		t.Fatalf("%v %+v", err, rows)
	}
}
