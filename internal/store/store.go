// Package store keeps server and VPN metadata in a JSON file and their passwords in a Secrets backend (the OS keyring).
// Passwords never touch the JSON file.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/KushalNaral/sshdeck/internal/vault"
)

type VPNKind string

const (
	Forti     VPNKind = "forti"
	GP        VPNKind = "gp"
	Hillstone VPNKind = "hillstone"
)

type Server struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	User        string   `json:"user"`
	Auth        string   `json:"auth"`              // "password" or "key"
	KeyPath     string   `json:"keyPath,omitempty"` // key auth: private key file; empty means use ssh-agent. The passphrase, if any, is the stored secret.
	VPNID       string   `json:"vpnId"`             // empty means direct, unless VPNRequired
	VPNRequired bool     `json:"vpnRequired,omitempty"`
	Production  bool     `json:"production,omitempty"` // the UI asks before connecting and before root container shells
	OS          string   `json:"os,omitempty"`
	PHP         string   `json:"php,omitempty"`
	PHPTarget   string   `json:"phpTarget,omitempty"`
	Webserver   string   `json:"webserver,omitempty"`
	Projects    []string `json:"projects,omitempty"`
	Note        string   `json:"note,omitempty"`
}

type VPN struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Kind    VPNKind `json:"kind"`
	Gateway string  `json:"gateway"`
	Port    int     `json:"port"`
	User    string  `json:"user"`
	Note    string  `json:"note,omitempty"`
	// TrustedCert is the sha256 of the gateway certificate to accept (FortiClient gateways with a self-signed certificate).
	TrustedCert string `json:"trustedCert,omitempty"`
}

// Task is a to-do about one or more servers, such as "upgrade PHP on Atlas". Status is todo, doing or done.
type Task struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Note      string   `json:"note,omitempty"`
	Status    string   `json:"status"`
	Tags      []string `json:"tags"`
	ServerIDs []string `json:"serverIds"`
	Due       string   `json:"due,omitempty"` // YYYY-MM-DD
	Created   int64    `json:"created"`       // unix ms
	Updated   int64    `json:"updated"`
}

// Secrets stores passwords by key. Keys look like "server/<id>" and "vpn/<id>".
type Secrets interface {
	Set(key, secret string) error
	Get(key string) (string, error)
	Delete(key string) error
}

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
	ErrLocked   = errors.New("sshdeck is locked: enter the passphrase")
)

// VPNInUseError says which servers block a VPN delete.
type VPNInUseError struct{ Servers int }

func (e VPNInUseError) Error() string {
	return fmt.Sprintf("%d server(s) use this VPN; move them to another VPN first", e.Servers)
}

type fileData struct {
	Servers []Server `json:"servers"`
	VPNs    []VPN    `json:"vpns"`
	Tasks   []Task   `json:"tasks,omitempty"`
	// Secrets holds passwords when the vault is enabled (see vault.go); otherwise they live in the OS keyring.
	Secrets map[string]string `json:"secrets,omitempty"`
}

type Store struct {
	mu   sync.Mutex
	path string
	sec  Secrets
	d    fileData

	// Vault mode (vault.go): the file is encrypted, secrets live inside it, and the store can be locked.
	vaultMode bool
	locked    bool
	key       *vault.Key
	fails     int       // wrong passphrases in a row
	until     time.Time // no unlock attempts before this
}

// Open loads the file at path, or starts empty when it does not exist yet.
func Open(path string, sec Secrets) (*Store, error) {
	s := &Store{path: path, sec: sec}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(b, &s.d); err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	return s, nil
}

func (s *Store) Servers() []Server {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Server{}, s.d.Servers...)
}

func (s *Store) VPNs() []VPN {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]VPN{}, s.d.VPNs...)
}

// Password returns the stored password for a server or VPN. kind is "server" or "vpn".
func (s *Store) Password(kind, id string) (string, error) {
	s.mu.Lock()
	if s.locked {
		s.mu.Unlock()
		return "", ErrLocked
	}
	if s.vaultMode {
		defer s.mu.Unlock()
		if v, ok := s.d.Secrets[kind+"/"+id]; ok {
			return v, nil
		}
		return "", ErrNotFound
	}
	s.mu.Unlock()
	return s.sec.Get(kind + "/" + id)
}

// setSecret and delSecret are called with s.mu held. They write to the vault map or the keyring.
func (s *Store) setSecret(key, secret string) error {
	if s.vaultMode {
		if s.d.Secrets == nil {
			s.d.Secrets = map[string]string{}
		}
		s.d.Secrets[key] = secret
		return nil
	}
	return s.sec.Set(key, secret)
}

func (s *Store) delSecret(key string) error {
	if s.vaultMode {
		delete(s.d.Secrets, key)
		return nil
	}
	if err := s.sec.Delete(key); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return nil
}

// hostRe allows host names, IPv4 and IPv6 literals; it cannot start with "-", which a client would read as an option.
var hostRe = regexp.MustCompile(`^[A-Za-z0-9_.:%\[\]][A-Za-z0-9_.:%\[\]-]*$`)

func hasControl(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// checkUser refuses what could break out of a client's argument or config line.
func checkUser(user string) error {
	switch {
	case hasControl(user):
		return invalid("user must not contain control characters or line breaks")
	case strings.HasPrefix(user, "-"):
		return invalid("user must not start with a dash")
	}
	return nil
}

func checkHost(host string) error {
	if !hostRe.MatchString(host) {
		return invalid("host %q must be a name or address without spaces, and not start with a dash", host)
	}
	return nil
}

// CheckVPN validates the fields that reach a root VPN client, and the VPN password (a line break in it
// could add lines to the client's config file). The vpn package calls it again before starting a client.
func CheckVPN(v VPN, password string) error {
	if err := checkHost(v.Gateway); err != nil {
		return err
	}
	if err := checkUser(v.User); err != nil {
		return err
	}
	if strings.ContainsAny(password, "\r\n\x00") {
		return invalid("the VPN password must not contain line breaks")
	}
	return nil
}

var certRe = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func newID(name string) string {
	b := make([]byte, 3)
	_, _ = rand.Read(b)
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	return fmt.Sprintf("%s-%s", slug, hex.EncodeToString(b))
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func validPort(p int) bool { return p >= 1 && p <= 65535 }

// SaveServer adds or replaces a server. A non-empty password replaces the stored one; empty keeps it.
func (s *Store) SaveServer(sv Server, password string) (Server, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return sv, ErrLocked
	}
	switch {
	case sv.Name == "":
		return sv, invalid("name is required")
	case sv.Host == "":
		return sv, invalid("host is required")
	case sv.User == "":
		return sv, invalid("user is required")
	case checkHost(sv.Host) != nil:
		return sv, checkHost(sv.Host)
	case checkUser(sv.User) != nil:
		return sv, checkUser(sv.User)
	case hasControl(sv.KeyPath):
		return sv, invalid("key file path must not contain control characters")
	case !validPort(sv.Port):
		return sv, invalid("port must be 1-65535")
	case sv.Auth != "password" && sv.Auth != "key":
		return sv, invalid("auth must be password or key")
	case sv.VPNID != "" && s.vpnIndex(sv.VPNID) < 0:
		return sv, invalid("unknown VPN %q", sv.VPNID)
	}
	if sv.VPNID != "" {
		sv.VPNRequired = false
	}
	if sv.ID == "" {
		sv.ID = newID(sv.Name)
	}
	if password != "" {
		if err := s.setSecret("server/"+sv.ID, password); err != nil {
			return sv, fmt.Errorf("store password: %w", err)
		}
	}
	if i := s.serverIndex(sv.ID); i >= 0 {
		s.d.Servers[i] = sv
	} else {
		s.d.Servers = append(s.d.Servers, sv)
	}
	return sv, s.flush()
}

func (s *Store) DeleteServer(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return ErrLocked
	}
	i := s.serverIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	s.d.Servers = append(s.d.Servers[:i], s.d.Servers[i+1:]...)
	for j := range s.d.Tasks {
		s.d.Tasks[j].ServerIDs = without(s.d.Tasks[j].ServerIDs, id)
	}
	if err := s.delSecret("server/" + id); err != nil {
		return err
	}
	return s.flush()
}

func (s *Store) SaveVPN(v VPN, password string) (VPN, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return v, ErrLocked
	}
	switch {
	case v.Name == "":
		return v, invalid("name is required")
	case v.Gateway == "":
		return v, invalid("gateway is required")
	case !validPort(v.Port):
		return v, invalid("port must be 1-65535")
	case v.Kind != Forti && v.Kind != GP && v.Kind != Hillstone:
		return v, invalid("unknown VPN client %q", v.Kind)
	case v.TrustedCert != "" && !certRe.MatchString(v.TrustedCert):
		return v, invalid("trusted certificate must be a 64-character sha256 hex digest")
	}
	if err := CheckVPN(v, password); err != nil {
		return v, err
	}
	if v.ID == "" {
		v.ID = newID(v.Name)
	}
	if password != "" {
		if err := s.setSecret("vpn/"+v.ID, password); err != nil {
			return v, fmt.Errorf("store password: %w", err)
		}
	}
	if i := s.vpnIndex(v.ID); i >= 0 {
		s.d.VPNs[i] = v
	} else {
		s.d.VPNs = append(s.d.VPNs, v)
	}
	return v, s.flush()
}

func (s *Store) DeleteVPN(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return ErrLocked
	}
	i := s.vpnIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	n := 0
	for _, sv := range s.d.Servers {
		if sv.VPNID == id {
			n++
		}
	}
	if n > 0 {
		return VPNInUseError{Servers: n}
	}
	s.d.VPNs = append(s.d.VPNs[:i], s.d.VPNs[i+1:]...)
	if err := s.delSecret("vpn/" + id); err != nil {
		return err
	}
	return s.flush()
}

func (s *Store) Tasks() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Task{}, s.d.Tasks...)
}

// SaveTask adds or replaces a task. Tags are lower-cased and de-duplicated; a new task starts as todo.
func (s *Store) SaveTask(t Task) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return t, ErrLocked
	}
	t.Title = strings.TrimSpace(t.Title)
	if t.Status == "" {
		t.Status = "todo"
	}
	switch {
	case t.Title == "":
		return t, invalid("title is required")
	case t.Status != "todo" && t.Status != "doing" && t.Status != "done":
		return t, invalid("status must be todo, doing or done")
	}
	if t.Due != "" {
		if _, err := time.Parse("2006-01-02", t.Due); err != nil {
			return t, invalid("due date must look like 2026-12-31")
		}
	}
	for _, id := range t.ServerIDs {
		if s.serverIndex(id) < 0 {
			return t, invalid("unknown server %q", id)
		}
	}
	seen := map[string]bool{}
	tags := []string{}
	for _, g := range t.Tags {
		if g = strings.ToLower(strings.TrimSpace(g)); g != "" && !seen[g] {
			seen[g] = true
			tags = append(tags, g)
		}
	}
	t.Tags = tags
	if t.ServerIDs == nil {
		t.ServerIDs = []string{}
	}
	now := time.Now().UnixMilli()
	t.Updated = now
	if i := s.taskIndex(t.ID); t.ID != "" && i >= 0 {
		t.Created = s.d.Tasks[i].Created
		s.d.Tasks[i] = t
	} else {
		if t.ID == "" {
			t.ID = newID("task")
		}
		t.Created = now
		s.d.Tasks = append(s.d.Tasks, t)
	}
	return t, s.flush()
}

func (s *Store) DeleteTask(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.locked {
		return ErrLocked
	}
	i := s.taskIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	s.d.Tasks = append(s.d.Tasks[:i], s.d.Tasks[i+1:]...)
	return s.flush()
}

func (s *Store) taskIndex(id string) int {
	for i, x := range s.d.Tasks {
		if x.ID == id {
			return i
		}
	}
	return -1
}

func without(ids []string, id string) []string {
	out := []string{}
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

func (s *Store) serverIndex(id string) int {
	for i, x := range s.d.Servers {
		if x.ID == id {
			return i
		}
	}
	return -1
}

func (s *Store) vpnIndex(id string) int {
	for i, x := range s.d.VPNs {
		if x.ID == id {
			return i
		}
	}
	return -1
}

// flush saves everything. In vault mode the JSON is encrypted first.
func (s *Store) flush() error {
	b, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	if s.vaultMode {
		if b, err = s.key.Seal(b); err != nil {
			return err
		}
	}
	return writeAtomic(s.path, b)
}

// writeAtomic writes with owner-only permissions: write a temp file, then rename over the real one.
func writeAtomic(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".data-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
