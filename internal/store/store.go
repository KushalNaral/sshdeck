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
	Auth        string   `json:"auth"`  // "password" or "key"
	VPNID       string   `json:"vpnId"` // empty means direct, unless VPNRequired
	VPNRequired bool     `json:"vpnRequired,omitempty"`
	OS          string   `json:"os,omitempty"`
	PHP         string   `json:"php,omitempty"`
	PHPTarget   string   `json:"phpTarget,omitempty"`
	Webserver   string   `json:"webserver,omitempty"`
	Projects    []string `json:"projects,omitempty"`
	Stage       int      `json:"stage"` // patch journey, 0..5
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
)

// VPNInUseError says which servers block a VPN delete.
type VPNInUseError struct{ Servers int }

func (e VPNInUseError) Error() string {
	return fmt.Sprintf("%d server(s) use this VPN; move them to another VPN first", e.Servers)
}

type fileData struct {
	Servers []Server `json:"servers"`
	VPNs    []VPN    `json:"vpns"`
}

type Store struct {
	mu   sync.Mutex
	path string
	sec  Secrets
	d    fileData
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
	return s.sec.Get(kind + "/" + id)
}

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
	switch {
	case sv.Name == "":
		return sv, invalid("name is required")
	case sv.Host == "":
		return sv, invalid("host is required")
	case sv.User == "":
		return sv, invalid("user is required")
	case !validPort(sv.Port):
		return sv, invalid("port must be 1-65535")
	case sv.Auth != "password" && sv.Auth != "key":
		return sv, invalid("auth must be password or key")
	case sv.Stage < 0 || sv.Stage > 5:
		return sv, invalid("stage must be 0-5")
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
		if err := s.sec.Set("server/"+sv.ID, password); err != nil {
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
	i := s.serverIndex(id)
	if i < 0 {
		return ErrNotFound
	}
	s.d.Servers = append(s.d.Servers[:i], s.d.Servers[i+1:]...)
	if err := s.sec.Delete("server/" + id); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return s.flush()
}

func (s *Store) SaveVPN(v VPN, password string) (VPN, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case v.Name == "":
		return v, invalid("name is required")
	case v.Gateway == "":
		return v, invalid("gateway is required")
	case !validPort(v.Port):
		return v, invalid("port must be 1-65535")
	case v.Kind != Forti && v.Kind != GP && v.Kind != Hillstone:
		return v, invalid("unknown VPN client %q", v.Kind)
	}
	if v.ID == "" {
		v.ID = newID(v.Name)
	}
	if password != "" {
		if err := s.sec.Set("vpn/"+v.ID, password); err != nil {
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
	if err := s.sec.Delete("vpn/" + id); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return s.flush()
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

// flush writes atomically with owner-only permissions: write a temp file, then rename over the real one.
func (s *Store) flush() error {
	b, err := json.MarshalIndent(s.d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".data-*.json")
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
	return os.Rename(tmp.Name(), s.path)
}
