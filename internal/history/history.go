// Package history keeps earlier versions of files edited through sshdeck: what changed, who changed it and when.
// It is small on purpose, not a version-control system. Content is stored once per distinct text, everything is
// sealed with a key the caller provides, and old versions are pruned by count and age.
package history

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/chacha20poly1305"
)

type Kind string

const (
	Initial Kind = "initial" // the file as sshdeck first saw it, before any edit made here
	Save    Kind = "save"    // saved from the editor
	Restore Kind = "restore" // an older version put back
	Outside Kind = "outside" // changed on the server by something other than sshdeck
)

const (
	DefaultKeep = 50
	DefaultDays = 90
)

type Version struct {
	ID     string `json:"id"`
	Server string `json:"server"`
	Path   string `json:"path"`
	Time   int64  `json:"time"`   // unix milliseconds
	Author string `json:"author"` // user and machine that made the change, "" when unknown
	Login  string `json:"login"`  // the SSH user the file was written as
	Kind   Kind   `json:"kind"`
	Hash   string `json:"hash"`
	Size   int    `json:"size"`
	Note   string `json:"note,omitempty"`
	Pinned bool   `json:"pinned,omitempty"`
	// Remote marks a version that is on the server copy but whose text has not been fetched here yet.
	Remote bool `json:"remote,omitempty"`
}

type Store struct {
	Keep int // newest versions kept per file; 0 means DefaultKeep
	Days int // versions older than this are dropped; 0 means DefaultDays

	dir  string
	aead interface {
		Seal(dst, nonce, plaintext, additionalData []byte) []byte
		Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
		NonceSize() int
	}
	mac []byte
	mu  sync.Mutex
	now func() time.Time
}

// Open uses dir for its files and key (32 bytes) to seal them.
func Open(dir string, key []byte) (*Store, error) {
	if len(key) != chacha20poly1305.KeySize {
		return nil, errors.New("history: the key must be 32 bytes")
	}
	a, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, err
	}
	h := hmac.New(sha256.New, key)
	h.Write([]byte("sshdeck history blob names"))
	if err := os.MkdirAll(filepath.Join(dir, "index"), 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir, aead: a, mac: h.Sum(nil), now: time.Now}, nil
}

func (s *Store) seal(p []byte) ([]byte, error) {
	n := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(n); err != nil {
		return nil, err
	}
	return s.aead.Seal(n, n, p, nil), nil
}

func (s *Store) open(b []byte) ([]byte, error) {
	if len(b) < s.aead.NonceSize() {
		return nil, errors.New("history: damaged file")
	}
	return s.aead.Open(nil, b[:s.aead.NonceSize()], b[s.aead.NonceSize():], nil)
}

func writeFile(p string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

var idRe = regexp.MustCompile(`[^A-Za-z0-9_-]`)

func (s *Store) indexPath(server string) string {
	return filepath.Join(s.dir, "index", idRe.ReplaceAllString(server, "_")+".enc")
}

// load reads one server's versions. An unreadable index (wrong key, damaged) counts as empty.
func (s *Store) load(server string) []Version {
	b, err := os.ReadFile(s.indexPath(server))
	if err != nil {
		return nil
	}
	plain, err := s.open(b)
	if err != nil {
		return nil
	}
	var v []Version
	if json.Unmarshal(plain, &v) != nil {
		return nil
	}
	return v
}

func (s *Store) save(server string, v []Version) error {
	if len(v) == 0 {
		if err := os.Remove(s.indexPath(server)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	plain, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b, err := s.seal(plain)
	if err != nil {
		return err
	}
	return writeFile(s.indexPath(server), b)
}

func (s *Store) blobPath(hash string) string {
	h := hmac.New(sha256.New, s.mac)
	h.Write([]byte(hash))
	name := hex.EncodeToString(h.Sum(nil))
	return filepath.Join(s.dir, "blobs", name[:2], name)
}

func hashOf(b []byte) string { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }

func (s *Store) days() int {
	if s.Days > 0 {
		return s.Days
	}
	return DefaultDays
}
func (s *Store) keep() int {
	if s.Keep > 0 {
		return s.Keep
	}
	return DefaultKeep
}

// Record stores content as a new version of v.Path. It adds nothing when the newest version of the file already
// has this content, and reports whether it did.
func (s *Store) Record(v Version, content []byte) (Version, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.load(v.Server)
	v.Hash, v.Size = hashOf(content), len(content)
	if newest, ok := newestOf(all, v.Path); ok && newest.Hash == v.Hash {
		return newest, false, nil
	}
	var zb bytes.Buffer
	zw := gzip.NewWriter(&zb)
	zw.Write(content)
	zw.Close()
	bp := s.blobPath(v.Hash)
	if _, err := os.Stat(bp); err != nil {
		sealed, err := s.seal(zb.Bytes())
		if err != nil {
			return v, false, err
		}
		if err := writeFile(bp, sealed); err != nil {
			return v, false, err
		}
	}
	t := s.now()
	v.Time = t.UnixMilli()
	v.ID = fmt.Sprintf("%d-%s", t.UnixNano(), v.Hash[:8])
	all = append(all, v)
	all = s.prune(all, v.Path)
	if err := s.save(v.Server, all); err != nil {
		return v, false, err
	}
	s.gc()
	return v, true, nil
}

func newestOf(all []Version, p string) (Version, bool) {
	var best Version
	found := false
	for _, v := range all {
		if v.Path == p && (!found || v.Time > best.Time || (v.Time == best.Time && v.ID > best.ID)) {
			best, found = v, true
		}
	}
	return best, found
}

// prune drops what is beyond the newest Keep versions or older than Days, never the newest or a pinned one.
func (s *Store) prune(all []Version, p string) []Version {
	mine := byTime(filter(all, func(v Version) bool { return v.Path == p }))
	cut := s.now().AddDate(0, 0, -s.days()).UnixMilli()
	drop := map[string]bool{}
	for i, v := range mine {
		if i == 0 || v.Pinned {
			continue
		}
		if i >= s.keep() || v.Time < cut {
			drop[v.ID] = true
		}
	}
	return filter(all, func(v Version) bool { return !drop[v.ID] })
}

func filter(all []Version, keep func(Version) bool) []Version {
	var out []Version
	for _, v := range all {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

// byTime sorts newest first (a copy).
func byTime(v []Version) []Version {
	out := append([]Version(nil), v...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Time != out[j].Time {
			return out[i].Time > out[j].Time
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// gc removes blobs that no version of any server refers to. Caller holds mu.
func (s *Store) gc() {
	used := map[string]bool{}
	entries, _ := os.ReadDir(filepath.Join(s.dir, "index"))
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(s.dir, "index", e.Name()))
		if err != nil {
			return // cannot tell what is in use, so remove nothing
		}
		plain, err := s.open(b)
		if err != nil {
			return // cannot read one index, so cannot tell what it uses: remove nothing
		}
		var vs []Version
		json.Unmarshal(plain, &vs)
		for _, v := range vs {
			used[s.blobPath(v.Hash)] = true
		}
	}
	filepath.WalkDir(filepath.Join(s.dir, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && !used[p] {
			os.Remove(p)
		}
		return nil
	})
}

// List returns the versions of one file, newest first.
func (s *Store) List(server, p string) []Version {
	s.mu.Lock()
	defer s.mu.Unlock()
	return byTime(filter(s.load(server), func(v Version) bool { return v.Path == p }))
}

func (s *Store) Latest(server, p string) (Version, bool) {
	l := s.List(server, p)
	if len(l) == 0 {
		return Version{}, false
	}
	return l[0], true
}

// Recent returns the newest versions across all files of a server.
func (s *Store) Recent(server string, limit int) []Version {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := byTime(s.load(server))
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all
}

// Content returns the text of a version.
func (s *Store) Content(server, id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.load(server) {
		if v.ID != id {
			continue
		}
		b, err := os.ReadFile(s.blobPath(v.Hash))
		if err != nil {
			return nil, errors.New("that version's content is no longer stored")
		}
		z, err := s.open(b)
		if err != nil {
			return nil, err
		}
		zr, err := gzip.NewReader(bytes.NewReader(z))
		if err != nil {
			return nil, err
		}
		return io.ReadAll(zr)
	}
	return nil, errors.New("no such version")
}

// Pin keeps a version from being pruned and gives it a note.
func (s *Store) Pin(server, id string, pinned bool, note string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.load(server)
	for i := range all {
		if all[i].ID == id {
			all[i].Pinned, all[i].Note = pinned, strings.TrimSpace(note)
			return s.save(server, all)
		}
	}
	return errors.New("no such version")
}

// Delete forgets the history of one file, or of the whole server when p is empty.
func (s *Store) Delete(server, p string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	keep := filter(s.load(server), func(v Version) bool { return p != "" && v.Path != p })
	if err := s.save(server, keep); err != nil {
		return err
	}
	s.gc()
	return nil
}

// defaultDeny lists files that usually hold keys. Their history is never kept.
var defaultDeny = []string{"id_rsa*", "id_ed25519*", "id_ecdsa*", "id_dsa*", "*.pem", "*.key", "*.p12", "*.pfx", "shadow", "gshadow", "*.kdbx"}

// Denied reports whether the file's name matches a pattern that must not be recorded. extra are added to the
// built-in list (matched on the file name, case-insensitively).
func Denied(p string, extra []string) bool {
	base := strings.ToLower(path.Base(p))
	for _, pat := range append(append([]string(nil), defaultDeny...), extra...) {
		if ok, _ := path.Match(strings.ToLower(strings.TrimSpace(pat)), base); ok {
			return true
		}
	}
	return false
}

// Import adds a version found on the server copy to the local index, without its text (fetch it with Fetched when
// it is opened). It reports whether the version was new.
func (s *Store) Import(v Version) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.load(v.Server)
	for _, x := range all {
		if x.ID == v.ID {
			return false, nil
		}
	}
	v.Remote, v.Pinned = true, false
	return true, s.save(v.Server, s.prune(append(all, v), v.Path))
}

// Fetched stores the text of an imported version and marks it as present locally.
func (s *Store) Fetched(server, id string, content []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.load(server)
	for i := range all {
		if all[i].ID != id {
			continue
		}
		if hashOf(content) != all[i].Hash {
			return errors.New("the copy on the server does not match its record")
		}
		var zb bytes.Buffer
		zw := gzip.NewWriter(&zb)
		zw.Write(content)
		zw.Close()
		sealed, err := s.seal(zb.Bytes())
		if err != nil {
			return err
		}
		if err := writeFile(s.blobPath(all[i].Hash), sealed); err != nil {
			return err
		}
		all[i].Remote = false
		return s.save(server, all)
	}
	return errors.New("no such version")
}

// Get returns one version's record.
func (s *Store) Get(server, id string) (Version, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.load(server) {
		if v.ID == id {
			return v, true
		}
	}
	return Version{}, false
}
