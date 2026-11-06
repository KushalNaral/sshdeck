package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/KushalNaral/sshdeck/internal/importer"
	"github.com/KushalNaral/sshdeck/internal/store"
)

// ImportRow is what the review screen shows for one line of the file. It never carries a password.
type ImportRow struct {
	Index       int    `json:"index"`
	Line        int    `json:"line"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Action      string `json:"action"` // new, update or error
	HasPassword bool   `json:"hasPassword"`
	Error       string `json:"error,omitempty"`
}

type ImportPreview struct {
	Token string      `json:"token"`
	File  string      `json:"file"`
	Rows  []ImportRow `json:"rows"`
}

type ImportResult struct {
	Added    int      `json:"added"`
	Updated  int      `json:"updated"`
	Failures []string `json:"failures"`
}

// pending is the parsed file waiting for the user to pick rows. Passwords stay here, in Go, and never reach the UI.
type pending struct {
	token string
	rows  []importer.Row
}

func same(a, b string) bool { return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) }

func (a *App) serverByName(name string) (store.Server, bool) {
	for _, s := range a.st.Servers() {
		if same(s.Name, name) {
			return s, true
		}
	}
	return store.Server{}, false
}

func (a *App) vpnByName(name string) (store.VPN, bool) {
	for _, v := range a.st.VPNs() {
		if same(v.Name, name) {
			return v, true
		}
	}
	return store.VPN{}, false
}

// PickImport asks for a CSV file and returns a preview of what importing it would do. Nothing is saved yet.
func (a *App) PickImport() (ImportPreview, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Import servers and VPNs",
		Filters: []runtime.FileFilter{{DisplayName: "CSV files", Pattern: "*.csv"}},
	})
	if err != nil || path == "" {
		return ImportPreview{}, err
	}
	return a.importFile(path)
}

func (a *App) importFile(path string) (ImportPreview, error) {
	if a.isLocked() {
		return ImportPreview{}, store.ErrLocked
	}
	f, err := os.Open(path)
	if err != nil {
		return ImportPreview{}, err
	}
	defer f.Close()
	rows, err := importer.ParseCSV(f)
	if err != nil {
		return ImportPreview{}, err
	}
	return a.stage(path, rows), nil
}

// PickSSHConfig asks for an OpenSSH config file (starting in ~/.ssh) and previews the hosts in it.
func (a *App) PickSSHConfig() (ImportPreview, error) {
	home, _ := os.UserHomeDir()
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Import hosts from an SSH config", DefaultDirectory: filepath.Join(home, ".ssh")})
	if err != nil || path == "" {
		return ImportPreview{}, err
	}
	return a.importSSHConfig(path)
}

func (a *App) importSSHConfig(path string) (ImportPreview, error) {
	if a.isLocked() {
		return ImportPreview{}, store.ErrLocked
	}
	f, err := os.Open(path)
	if err != nil {
		return ImportPreview{}, err
	}
	defer f.Close()
	name, home := "", ""
	if u, err := user.Current(); err == nil {
		name, home = u.Username, u.HomeDir
	}
	rows := importer.ParseSSHConfig(f, name, home)
	if len(rows) == 0 {
		return ImportPreview{}, errors.New("no Host entries found; wildcard hosts and Match blocks are skipped")
	}
	return a.stage(path, rows), nil
}

// stage keeps parsed rows until ApplyImport and returns the preview of them.
func (a *App) stage(path string, rows []importer.Row) ImportPreview {
	tok := make([]byte, 8)
	_, _ = rand.Read(tok)
	p := &pending{token: hex.EncodeToString(tok), rows: rows}
	a.mu.Lock()
	a.pending = p
	a.mu.Unlock()

	pv := ImportPreview{Token: p.token, File: filepath.Base(path)}
	for i, r := range rows {
		row := ImportRow{Index: i, Line: r.Line, Type: r.Type, Name: r.Name, HasPassword: r.Secret != "", Error: r.Err, Action: "new"}
		switch {
		case r.Err != "":
			row.Action = "error"
		case a.exists(r):
			row.Action = "update"
		}
		pv.Rows = append(pv.Rows, row)
	}
	return pv
}

func (a *App) exists(r importer.Row) bool {
	switch r.Type {
	case "vpn":
		_, ok := a.vpnByName(r.Name)
		return ok
	case "server":
		_, ok := a.serverByName(r.Name)
		return ok
	case "login":
		if sv, ok := a.serverByName(r.ServerName); ok {
			for _, l := range sv.Logins {
				if same(l.Label, r.Login.Label) {
					return true
				}
			}
		}
	}
	return false
}

// ApplyImport saves the picked rows of the last previewed file: VPNs first, then servers, then logins, whatever
// order the file has them in. An existing item with the same name is updated and keeps its stored password when
// the file leaves the password blank. A row that fails is reported and the others still go in.
func (a *App) ApplyImport(token string, picked []int) (ImportResult, error) {
	a.mu.Lock()
	p := a.pending
	if p == nil || p.token != token {
		a.mu.Unlock()
		return ImportResult{}, errors.New("that import has expired; choose the file again")
	}
	a.pending = nil
	a.mu.Unlock()

	use := map[int]bool{}
	for _, i := range picked {
		if i >= 0 && i < len(p.rows) && p.rows[i].Err == "" {
			use[i] = true
		}
	}
	var res ImportResult
	fail := func(r importer.Row, err error) {
		res.Failures = append(res.Failures, fmt.Sprintf("line %d, %s: %v", r.Line, r.Name, strings.TrimPrefix(err.Error(), store.ErrInvalid.Error()+": ")))
	}
	count := func(existed bool) {
		if existed {
			res.Updated++
		} else {
			res.Added++
		}
	}
	for _, typ := range []string{"vpn", "server", "login"} {
		for i, r := range p.rows {
			if !use[i] || r.Type != typ {
				continue
			}
			existed := a.exists(r)
			var err error
			switch typ {
			case "vpn":
				v := r.VPN
				if old, ok := a.vpnByName(r.Name); ok {
					v.ID = old.ID
				}
				_, err = a.st.SaveVPN(v, r.Secret)
			case "server":
				err = a.importServer(r)
			case "login":
				err = a.importLogin(r)
			}
			if err != nil {
				fail(r, err)
				continue
			}
			count(existed)
		}
	}
	for i, r := range p.rows { // a jump host can be anywhere in the file, so link them once every server is saved
		if !use[i] || r.Type != "server" || r.JumpName == "" {
			continue
		}
		sv, ok := a.serverByName(r.Name)
		hop, hok := a.serverByName(r.JumpName)
		switch {
		case !ok:
		case !hok:
			fail(r, fmt.Errorf("jump host %q was not imported or saved", r.JumpName))
		default:
			sv.JumpID = hop.ID
			if _, err := a.st.SaveServer(sv, ""); err != nil {
				fail(r, err)
			}
		}
	}
	a.audit("import", "csv", fmt.Sprintf("%d added, %d updated, %d failed", res.Added, res.Updated, len(res.Failures)))
	return res, nil
}

func (a *App) importServer(r importer.Row) error {
	sv := r.Server
	if r.VPNName != "" {
		v, ok := a.vpnByName(r.VPNName)
		if !ok {
			return fmt.Errorf("unknown VPN %q (add it to the file or to Admin first)", r.VPNName)
		}
		sv.VPNID = v.ID
	}
	if old, ok := a.serverByName(r.Name); ok { // keep what the file does not carry: OS and the VPN flag
		merged := old
		merged.Name, merged.Host, merged.Port, merged.User, merged.Auth, merged.KeyPath = sv.Name, sv.Host, sv.Port, sv.User, sv.Auth, sv.KeyPath
		merged.Production, merged.AutoConnect, merged.Projects, merged.Software, merged.Note, merged.VPNID = sv.Production, sv.AutoConnect, sv.Projects, sv.Software, sv.Note, sv.VPNID
		sv = merged
	}
	_, err := a.st.SaveServer(sv, r.Secret)
	return err
}

func (a *App) importLogin(r importer.Row) error {
	sv, ok := a.serverByName(r.ServerName)
	if !ok {
		return fmt.Errorf("unknown server %q (add it to the file or to Admin first)", r.ServerName)
	}
	l := r.Login
	for _, old := range sv.Logins {
		if same(old.Label, l.Label) {
			l.ID = old.ID
		}
	}
	_, err := a.st.SaveLogin(sv.ID, l, r.Secret)
	return err
}

// SaveTemplate asks where to save a blank CSV with one example of each row type.
func (a *App) SaveTemplate() (string, error) {
	return a.saveCSV("sshdeck-template.csv", func(b *bytes.Buffer) error { return importer.WriteTemplate(b) })
}

// ExportCSV asks where to save every VPN, server and login in the import format. Passwords are never included.
func (a *App) ExportCSV() (string, error) {
	if a.isLocked() {
		return "", store.ErrLocked
	}
	path, err := a.askSavePath("sshdeck-export.csv")
	if err != nil || path == "" {
		return "", err
	}
	if err := a.exportTo(path); err != nil {
		return "", err
	}
	a.audit("export", "csv", filepath.Base(path))
	return path, nil
}

func (a *App) askSavePath(name string) (string, error) {
	return runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title: "Save CSV", DefaultFilename: name, Filters: []runtime.FileFilter{{DisplayName: "CSV files", Pattern: "*.csv"}},
	})
}

func (a *App) saveCSV(name string, fill func(*bytes.Buffer) error) (string, error) {
	path, err := a.askSavePath(name)
	if err != nil || path == "" {
		return "", err
	}
	var b bytes.Buffer
	if err := fill(&b); err != nil {
		return "", err
	}
	return path, writeFile0600(path, b.Bytes())
}

func writeFile0600(path string, b []byte) error {
	if err := os.WriteFile(path, b, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

func (a *App) exportTo(path string) error {
	var b bytes.Buffer
	if err := importer.WriteCSV(&b, a.st.VPNs(), a.st.Servers()); err != nil {
		return err
	}
	return writeFile0600(path, b.Bytes())
}
