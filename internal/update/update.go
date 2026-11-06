// Package update checks GitHub for a newer sshdeck release and installs it. A release is only installed when its
// SHA256SUMS file carries a valid ed25519 signature from the key built into the app, and the downloaded file matches
// the signed checksum. Nothing here runs without the user's click; the caller decides when to check.
package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxArchive = 250 << 20
	maxSums    = 64 << 10
	maxSig     = 1 << 10
)

// Updater looks for releases of Repo ("owner/name"). The zero values of the optional fields mean the real GitHub.
type Updater struct {
	Repo      string
	Current   string // the running version, such as v0.4.0; "dev" never updates
	PublicKey ed25519.PublicKey

	APIBase   string       // default https://api.github.com
	Client    *http.Client // default: a client with a 60 second timeout
	GOOS      string       // default: the running system
	GOARCH    string
	AllowHost func(string) bool // which hosts may be contacted; default githubHost
}

// Info says what a check found.
type Info struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Available bool   `json:"available"`
	Notes     string `json:"notes"`
	PageURL   string `json:"pageUrl"`
	// CanInstall is false when this release has no build for this system or no signature; then the user is sent to PageURL.
	CanInstall bool `json:"canInstall"`

	assetName, assetURL, sumsURL, sigURL string
}

type ghRelease struct {
	Tag        string `json:"tag_name"`
	Body       string `json:"body"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	HTMLURL    string `json:"html_url"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// NotWritableError means the folder holding the running program cannot be written to, so it cannot update itself.
type NotWritableError struct {
	Dir string
	Err error
}

func (e *NotWritableError) Error() string {
	return fmt.Sprintf("sshdeck cannot replace itself: %s is not writable (%v)", e.Dir, e.Err)
}

// githubHost is the only kind of host the updater talks to.
func githubHost(host string) bool {
	return host == "github.com" || host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

type ver struct {
	n   [3]int
	pre string
}

func parse(s string) (ver, bool) {
	var v ver
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	core, pre, _ := strings.Cut(s, "-")
	parts := strings.Split(core, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return v, false
		}
		v.n[i] = n
	}
	v.pre = pre
	return v, true
}

func less(a, b ver) bool {
	if a.n != b.n {
		for i := range a.n {
			if a.n[i] != b.n[i] {
				return a.n[i] < b.n[i]
			}
		}
	}
	switch {
	case a.pre == b.pre:
		return false
	case a.pre == "": // a release is newer than its candidates
		return false
	case b.pre == "":
		return true
	}
	return a.pre < b.pre
}

// Newer reports whether tag is a newer version than have. An unparsable version (such as "dev") is never newer.
func Newer(have, tag string) bool {
	h, ok1 := parse(have)
	t, ok2 := parse(tag)
	return ok1 && ok2 && less(h, t)
}

func (u *Updater) allow(raw string) (*url.URL, error) {
	p, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	allow := u.AllowHost
	if allow == nil {
		allow = githubHost
	}
	host := p.Hostname()
	loopback := host == "127.0.0.1" || host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
	if !allow(host) || (p.Scheme != "https" && !(p.Scheme == "http" && loopback)) {
		return nil, fmt.Errorf("refusing to contact %s", host)
	}
	return p, nil
}

func (u *Updater) get(ctx context.Context, raw string, limit int64) ([]byte, int, error) {
	if _, err := u.allow(raw); err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sshdeck-updater")
	c := u.Client
	if c == nil {
		c = &http.Client{Timeout: 60 * time.Second}
	}
	// A redirect (GitHub serves assets from another host) must stay on an allowed host too.
	cc := *c
	cc.CheckRedirect = func(r *http.Request, _ []*http.Request) error { _, err := u.allow(r.URL.String()); return err }
	resp, err := cc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if int64(len(b)) > limit {
		return nil, resp.StatusCode, errors.New("the download is larger than expected")
	}
	return b, resp.StatusCode, nil
}

func (u *Updater) goos() string {
	if u.GOOS != "" {
		return u.GOOS
	}
	return runtimeGOOS
}

func (u *Updater) goarch() string {
	if u.GOARCH != "" {
		return u.GOARCH
	}
	return runtimeGOARCH
}

// Check asks GitHub for the latest release. A stable build only looks at stable releases; a pre-release build also
// sees newer pre-releases.
func (u *Updater) Check(ctx context.Context) (Info, error) {
	info := Info{Current: u.Current}
	base := u.APIBase
	if base == "" {
		base = "https://api.github.com"
	}
	cur, ok := parse(u.Current)
	wantPre := ok && cur.pre != ""
	var rel ghRelease
	if wantPre {
		b, code, err := u.get(ctx, fmt.Sprintf("%s/repos/%s/releases?per_page=10", base, u.Repo), 1<<20)
		if err := statusErr(code, err, u.Repo); err != nil {
			return info, err
		}
		var list []ghRelease
		if err := json.Unmarshal(b, &list); err != nil {
			return info, err
		}
		sort.SliceStable(list, func(i, j int) bool {
			a, _ := parse(list[i].Tag)
			b, _ := parse(list[j].Tag)
			return less(b, a) // highest first
		})
		for _, r := range list {
			if !r.Draft {
				rel = r
				break
			}
		}
	} else {
		b, code, err := u.get(ctx, fmt.Sprintf("%s/repos/%s/releases/latest", base, u.Repo), 1<<20)
		if err := statusErr(code, err, u.Repo); err != nil {
			return info, err
		}
		if err := json.Unmarshal(b, &rel); err != nil {
			return info, err
		}
		if rel.Prerelease || rel.Draft {
			rel = ghRelease{}
		}
	}
	if rel.Tag == "" {
		return info, nil
	}
	info.Latest = rel.Tag
	info.Available = Newer(u.Current, rel.Tag)
	info.Notes = rel.Body
	if len(info.Notes) > 4000 {
		info.Notes = info.Notes[:4000] + "…"
	}
	info.PageURL = rel.HTMLURL
	want := fmt.Sprintf("sshdeck-%s-%s-%s.", rel.Tag, u.goos(), u.goarch())
	for _, a := range rel.Assets {
		switch {
		case strings.HasPrefix(a.Name, want) && (strings.HasSuffix(a.Name, ".tar.gz") || strings.HasSuffix(a.Name, ".zip")):
			info.assetName, info.assetURL = a.Name, a.URL
		case a.Name == "SHA256SUMS":
			info.sumsURL = a.URL
		case a.Name == "SHA256SUMS.sig":
			info.sigURL = a.URL
		}
	}
	info.CanInstall = info.assetURL != "" && info.sumsURL != "" && info.sigURL != ""
	return info, nil
}

func statusErr(code int, err error, repo string) error {
	switch {
	case err != nil:
		return err
	case code == http.StatusNotFound:
		return fmt.Errorf("cannot read the releases of %s: the repository may be private (updates need it to be public)", repo)
	case code != http.StatusOK:
		return fmt.Errorf("GitHub answered %d", code)
	}
	return nil
}

// Fetch downloads the release for this system, verifies it, and writes the new program into dir (next to the running
// one, so replacing it is a rename). It returns the path of the new program.
func (u *Updater) Fetch(ctx context.Context, info Info, dir string) (string, error) {
	if info.assetURL == "" {
		return "", fmt.Errorf("release %s has no build for %s/%s: download it from %s", info.Latest, u.goos(), u.goarch(), info.PageURL)
	}
	if info.sumsURL == "" || info.sigURL == "" {
		return "", fmt.Errorf("release %s is not signed, so sshdeck will not install it by itself: download it from %s", info.Latest, info.PageURL)
	}
	probe, err := os.CreateTemp(dir, ".sshdeck-probe-*")
	if err != nil {
		return "", &NotWritableError{Dir: dir, Err: err}
	}
	probe.Close()
	os.Remove(probe.Name())

	sums, code, err := u.get(ctx, info.sumsURL, maxSums)
	if err := statusErr(code, err, "SHA256SUMS"); err != nil {
		return "", err
	}
	sigB, code, err := u.get(ctx, info.sigURL, maxSig)
	if err := statusErr(code, err, "SHA256SUMS.sig"); err != nil {
		return "", err
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB)))
	if err != nil || len(u.PublicKey) != ed25519.PublicKeySize || !ed25519.Verify(u.PublicKey, sums, sig) {
		return "", errors.New("the release's signature is not valid, so it was not installed")
	}
	var want string
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && strings.TrimPrefix(f[1], "*") == info.assetName {
			want = strings.ToLower(f[0])
		}
	}
	if want == "" {
		return "", fmt.Errorf("the signed checksums do not list %s", info.assetName)
	}
	data, code, err := u.get(ctx, info.assetURL, maxArchive)
	if err := statusErr(code, err, info.assetName); err != nil {
		return "", err
	}
	if h := sha256.Sum256(data); hex.EncodeToString(h[:]) != want {
		return "", errors.New("the download does not match the signed checksum, so it was not installed")
	}
	return extract(data, info.assetName, u.goos(), dir)
}

// extract writes the program out of the archive into dir. Only a member named sshdeck (sshdeck.exe on Windows), at
// most one folder deep, is taken, and nothing is written anywhere else.
func extract(data []byte, name, goos, dir string) (string, error) {
	bin := "sshdeck"
	if goos == "windows" {
		bin = "sshdeck.exe"
	}
	ok := func(member string) bool {
		clean := path.Clean(strings.ReplaceAll(member, `\`, "/"))
		return !strings.Contains(member, "..") && !path.IsAbs(clean) && path.Base(clean) == bin && strings.Count(clean, "/") <= 1
	}
	write := func(r io.Reader, size int64) (string, error) {
		if size <= 0 || size > maxArchive {
			return "", errors.New("the program in the archive has an unexpected size")
		}
		f, err := os.CreateTemp(dir, ".sshdeck-update-*")
		if err != nil {
			return "", &NotWritableError{Dir: dir, Err: err}
		}
		if _, err := io.Copy(f, io.LimitReader(r, size)); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", err
		}
		if err := f.Chmod(0o755); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", err
		}
		return f.Name(), f.Close()
	}
	if strings.HasSuffix(name, ".zip") {
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", err
		}
		for _, f := range zr.File {
			if ok(f.Name) && !f.FileInfo().IsDir() {
				rc, err := f.Open()
				if err != nil {
					return "", err
				}
				defer rc.Close()
				return write(rc, int64(f.UncompressedSize64))
			}
		}
		return "", fmt.Errorf("%s does not contain %s", name, bin)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return "", fmt.Errorf("%s does not contain %s", name, bin)
		}
		if err != nil {
			return "", err
		}
		if h.Typeflag == tar.TypeReg && ok(h.Name) {
			return write(tr, h.Size)
		}
		if strings.Contains(h.Name, "..") {
			return "", errors.New("the archive contains a path outside its folder")
		}
	}
}

// Apply puts the new program in place of the running one. On Linux the running file can be replaced by a rename. On
// Windows a running program cannot be overwritten but can be renamed, so the old one is kept as sshdeck.exe.old until
// CleanupOld removes it at the next start.
func Apply(newPath, exe, goos string) error {
	if goos != "windows" {
		return os.Rename(newPath, exe)
	}
	old := exe + ".old"
	if os.Remove(old) != nil && exists(old) {
		// The last update's file is still held (it can be running, or a scanner has it open). Use another name
		// rather than fail; CleanupOld clears these at a later start.
		old = fmt.Sprintf("%s.%d.old", exe, time.Now().UnixNano())
	}
	var err error
	for i := 0; i < 5; i++ { // a virus scanner looking at a fresh download holds it for a moment
		if err = os.Rename(exe, old); err == nil {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if err != nil {
		return err
	}
	if err := os.Rename(newPath, exe); err != nil {
		_ = os.Rename(old, exe)
		return err
	}
	return nil
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// CleanupOld removes the previous program left by a Windows update.
func CleanupOld(exe string) {
	exe = filepath.Clean(exe)
	_ = os.Remove(exe + ".old")
	left, _ := filepath.Glob(exe + ".*.old")
	for _, f := range left {
		_ = os.Remove(f)
	}
}
