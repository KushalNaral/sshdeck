package importer

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// Columns of the sshdeck CSV, in the order the template and the export write them. Every row has a type column:
// vpn, server or login. Columns a row does not use stay empty; unknown columns in a file are ignored.
var Columns = []string{"type", "name", "host", "port", "user", "auth", "key_path", "vpn", "vpn_kind", "trusted_cert", "manual", "login_at", "password",
	"production", "auto_connect", "projects", "software", "server", "kind", "address", "note"}

// Row is one parsed line. Err explains why the line cannot be imported; the other fields are then incomplete.
type Row struct {
	Line       int // line number in the file, for messages
	Type       string
	Name       string
	VPN        store.VPN
	Server     store.Server
	Login      store.Login
	Secret     string // the password cell; never shown in a preview
	VPNName    string // a server row: the VPN it uses, by name
	ServerName string // a login row: the server it belongs to, by name
	Err        string
}

// ParseCSV reads the whole file. It fails only when the file cannot be read as a table with a type column;
// a bad line becomes a Row with Err set, so one typo does not block the rest.
func ParseCSV(r io.Reader) ([]Row, error) {
	br := bufio.NewReader(r)
	if b, _ := br.Peek(3); string(b) == "\xef\xbb\xbf" { // spreadsheets add a byte order mark
		_, _ = br.Discard(3)
	}
	cr := csv.NewReader(br)
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	head, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, errors.New("the file is empty")
	}
	if err != nil {
		return nil, fmt.Errorf("read the header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			if _, dup := col[h]; !dup {
				col[h] = i
			}
		}
	}
	if _, ok := col["type"]; !ok {
		return nil, errors.New(`the first line must name the columns, including "type" (vpn, server or login); download the template to see them`)
	}
	var rows []Row
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			return rows, nil
		}
		line, _ := cr.FieldPos(0)
		if err != nil {
			var pe *csv.ParseError
			if errors.As(err, &pe) {
				line = pe.StartLine
			}
			rows = append(rows, Row{Line: line, Err: err.Error()})
			continue
		}
		get := func(name string) string {
			if i, ok := col[name]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		if empty(rec) {
			continue
		}
		rows = append(rows, parseRow(line, get))
	}
}

func empty(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func parseRow(line int, get func(string) string) Row {
	r := Row{Line: line, Type: strings.ToLower(get("type")), Name: get("name"), Secret: get("password")}
	bad := func(err error) Row {
		r.Err = strings.TrimPrefix(err.Error(), store.ErrInvalid.Error()+": ")
		return r
	}
	port, perr := atoi(get("port"))
	if perr != nil {
		return bad(fmt.Errorf("port %q is not a number", get("port")))
	}
	switch r.Type {
	case "vpn":
		if port == 0 {
			port = 443
			if get("vpn_kind") == string(store.Forti) {
				port = 10443
			}
		}
		r.VPN = store.VPN{Name: r.Name, Kind: store.VPNKind(strings.ToLower(get("vpn_kind"))), Gateway: get("host"), Port: port,
			User: get("user"), TrustedCert: get("trusted_cert"), Manual: truthy(get("manual")), LoginAt: strings.ToLower(get("login_at")), Note: get("note")}
		if err := store.ValidateVPN(r.VPN, r.Secret); err != nil {
			return bad(err)
		}
	case "server":
		if port == 0 {
			port = 22
		}
		auth := strings.ToLower(get("auth"))
		if auth == "" {
			auth = "password"
		}
		sw, err := parseSoftware(get("software"))
		if err != nil {
			return bad(err)
		}
		r.VPNName = get("vpn")
		r.Server = store.Server{Name: r.Name, Host: get("host"), Port: port, User: get("user"), Auth: auth, KeyPath: get("key_path"),
			Production: truthy(get("production")), AutoConnect: truthy(get("auto_connect")), Projects: splitList(get("projects")), Software: sw, Note: get("note")}
		if err := store.CheckServer(r.Server); err != nil {
			return bad(err)
		}
	case "login":
		r.ServerName = get("server")
		r.Login = store.Login{Label: r.Name, Kind: strings.ToLower(get("kind")), User: get("user"), Address: get("address"), Note: get("note")}
		if r.ServerName == "" {
			return bad(errors.New("a login needs the name of its server in the server column"))
		}
		if err := store.CheckLogin(r.Login); err != nil {
			return bad(err)
		}
	default:
		return bad(fmt.Errorf("type %q is not vpn, server or login", r.Type))
	}
	return r
}

func atoi(s string) (int, error) {
	if s == "" {
		return 0, nil
	}
	return strconv.Atoi(s)
}

func truthy(s string) bool {
	switch strings.ToLower(s) {
	case "yes", "y", "true", "1", "x":
		return true
	}
	return false
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ";") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseSoftware reads "PHP 8.2.25>8.2.35; Node 18.19": the last word before ">" is the version, the rest the name.
func parseSoftware(s string) ([]store.Software, error) {
	var out []store.Software
	for _, item := range splitList(s) {
		left, target, _ := strings.Cut(item, ">")
		left = strings.TrimSpace(left)
		x := store.Software{Target: strings.TrimSpace(target)}
		if i := strings.LastIndex(left, " "); i > 0 {
			x.Name, x.Version = strings.TrimSpace(left[:i]), left[i+1:]
		} else {
			x.Name = left
		}
		if x.Name == "" {
			return nil, fmt.Errorf("software %q has no name", item)
		}
		out = append(out, x)
	}
	return out, nil
}

func formatSoftware(list []store.Software) string {
	var parts []string
	for _, x := range list {
		p := strings.TrimSpace(x.Name + " " + x.Version)
		if x.Target != "" {
			p += ">" + x.Target
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, "; ")
}

func record(m map[string]string) []string {
	out := make([]string, len(Columns))
	for i, c := range Columns {
		out[i] = m[c]
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func yes(b bool) string {
	if b {
		return "yes"
	}
	return ""
}

// WriteCSV writes every VPN, server and login in the import format. The password column is always empty.
func WriteCSV(w io.Writer, vpns []store.VPN, servers []store.Server) error {
	cw := csv.NewWriter(w)
	_ = cw.Write(Columns)
	names := map[string]string{}
	for _, v := range vpns {
		names[v.ID] = v.Name
		_ = cw.Write(record(map[string]string{"type": "vpn", "name": v.Name, "host": v.Gateway, "port": strconv.Itoa(v.Port), "user": v.User,
			"vpn_kind": string(v.Kind), "trusted_cert": firstNonEmpty(v.TrustedCert, v.TrustedPin), "manual": yes(v.Manual), "login_at": v.LoginAt, "note": v.Note}))
	}
	for _, s := range servers {
		_ = cw.Write(record(map[string]string{"type": "server", "name": s.Name, "host": s.Host, "port": strconv.Itoa(s.Port), "user": s.User,
			"auth": s.Auth, "key_path": s.KeyPath, "vpn": names[s.VPNID], "production": yes(s.Production), "auto_connect": yes(s.AutoConnect),
			"projects": strings.Join(s.Projects, "; "), "software": formatSoftware(s.Software), "note": s.Note}))
		for _, l := range s.Logins {
			_ = cw.Write(record(map[string]string{"type": "login", "name": l.Label, "user": l.User, "server": s.Name, "kind": l.Kind,
				"address": l.Address, "note": l.Note}))
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteTemplate writes the header and one example of each type, to be edited in a spreadsheet.
func WriteTemplate(w io.Writer) error {
	cw := csv.NewWriter(w)
	_ = cw.Write(Columns)
	_ = cw.Write(record(map[string]string{"type": "vpn", "name": "Office VPN", "host": "vpn.example.org", "port": "10443", "user": "me", "vpn_kind": "forti", "password": "change-me"}))
	_ = cw.Write(record(map[string]string{"type": "server", "name": "Web 1", "host": "203.0.113.10", "port": "22", "user": "deploy", "auth": "password", "vpn": "Office VPN",
		"password": "change-me", "production": "yes", "projects": "Site A; Site B", "software": "PHP 8.2.25>8.2.35; Node 18.19>20.11"}))
	_ = cw.Write(record(map[string]string{"type": "login", "name": "Main database", "user": "root", "password": "change-me", "server": "Web 1", "kind": "db", "address": "127.0.0.1:3306"}))
	cw.Flush()
	return cw.Error()
}
