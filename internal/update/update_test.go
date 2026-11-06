package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionOrdering(t *testing.T) {
	for _, c := range []struct {
		have, tag string
		newer     bool
	}{
		{"v0.3.0", "v0.4.0", true},
		{"v0.4.0", "v0.4.0", false},
		{"v0.4.0", "v0.3.9", false},
		{"v0.4.0-rc1", "v0.4.0", true},  // a release is newer than its candidate
		{"v0.4.0", "v0.4.0-rc2", false}, // and a candidate is not newer than the release
		{"v0.4.0-rc1", "v0.4.0-rc2", true},
		{"v0.9.0", "v0.10.0", true}, // numbers, not text
		{"dev", "v0.1.0", false},    // a development build never offers itself an update
		{"v0.3.0", "nonsense", false},
	} {
		if got := Newer(c.have, c.tag); got != c.newer {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.have, c.tag, got, c.newer)
		}
	}
}

// fixture is a fake GitHub: a release API, release assets, and the files a real release carries.
type fixture struct {
	t       *testing.T
	srv     *httptest.Server
	pub     ed25519.PublicKey
	priv    ed25519.PrivateKey
	tag     string
	pre     bool
	binary  []byte
	sums    string
	sig     string
	archive []byte
	name    string
	hits    map[string]int
}

func newFixture(t *testing.T, goos, tag string) *fixture {
	t.Helper()
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	f := &fixture{t: t, pub: pub, priv: priv, tag: tag, binary: []byte("#!new-binary-" + tag), hits: map[string]int{}}
	switch goos {
	case "windows":
		f.name = fmt.Sprintf("sshdeck-%s-windows-amd64.zip", tag)
		var b bytes.Buffer
		zw := zip.NewWriter(&b)
		w, _ := zw.Create(fmt.Sprintf("sshdeck-%s-windows-amd64/sshdeck.exe", tag))
		w.Write(f.binary)
		zw.Close()
		f.archive = b.Bytes()
	default:
		f.name = fmt.Sprintf("sshdeck-%s-linux-amd64.tar.gz", tag)
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		tw := tar.NewWriter(gz)
		for _, e := range []struct {
			name string
			body []byte
		}{{fmt.Sprintf("sshdeck-%s-linux-amd64/install.sh", tag), []byte("#!/bin/sh")}, {fmt.Sprintf("sshdeck-%s-linux-amd64/sshdeck", tag), f.binary}} {
			tw.WriteHeader(&tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body))})
			tw.Write(e.body)
		}
		tw.Close()
		gz.Close()
		f.archive = b.Bytes()
	}
	h := sha256.Sum256(f.archive)
	f.sums = hex.EncodeToString(h[:]) + "  " + f.name + "\n"
	f.sig = base64.StdEncoding.EncodeToString(ed25519.Sign(priv, []byte(f.sums)))
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fixture) serve(w http.ResponseWriter, r *http.Request) {
	f.hits[r.URL.Path]++
	switch {
	case strings.HasPrefix(r.URL.Path, "/nope"):
		http.NotFound(w, r)
	case strings.HasSuffix(r.URL.Path, "/releases/latest"), strings.HasSuffix(r.URL.Path, "/releases"):
		rel := map[string]any{
			"tag_name": f.tag, "body": "Fixes things.", "prerelease": f.pre, "draft": false, "html_url": f.srv.URL + "/page",
			"assets": []map[string]any{
				{"name": f.name, "browser_download_url": f.srv.URL + "/dl/" + f.name, "size": len(f.archive)},
				{"name": "SHA256SUMS", "browser_download_url": f.srv.URL + "/dl/SHA256SUMS", "size": len(f.sums)},
				{"name": "SHA256SUMS.sig", "browser_download_url": f.srv.URL + "/dl/SHA256SUMS.sig", "size": len(f.sig)},
			},
		}
		if strings.HasSuffix(r.URL.Path, "/releases") {
			json.NewEncoder(w).Encode([]any{rel})
		} else {
			json.NewEncoder(w).Encode(rel)
		}
	case r.URL.Path == "/dl/"+f.name:
		w.Write(f.archive)
	case r.URL.Path == "/dl/SHA256SUMS":
		w.Write([]byte(f.sums))
	case r.URL.Path == "/dl/SHA256SUMS.sig":
		w.Write([]byte(f.sig))
	default:
		http.NotFound(w, r)
	}
}

func (f *fixture) updater(current, goos string) *Updater {
	return &Updater{Repo: "o/r", Current: current, APIBase: f.srv.URL, Client: f.srv.Client(), PublicKey: f.pub, GOOS: goos, GOARCH: "amd64",
		AllowHost: func(string) bool { return true }}
}

func TestCheckReportsANewerRelease(t *testing.T) {
	f := newFixture(t, "linux", "v0.5.0")
	info, err := f.updater("v0.4.0", "linux").Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !info.Available || info.Latest != "v0.5.0" || info.Notes != "Fixes things." || info.PageURL == "" {
		t.Errorf("info = %+v", info)
	}
	info, _ = f.updater("v0.5.0", "linux").Check(context.Background())
	if info.Available {
		t.Error("the same version is not an update")
	}
}

func TestCheckForAMissingRepositoryIsAPlainError(t *testing.T) {
	f := newFixture(t, "linux", "v0.5.0")
	u := f.updater("v0.4.0", "linux")
	u.APIBase = f.srv.URL + "/nope"
	if _, err := u.Check(context.Background()); err == nil || !strings.Contains(err.Error(), "public") {
		t.Errorf("err = %v, want a message that mentions the repository may be private", err)
	}
}

func TestPreReleaseBuildsSeePreReleasesAndStableBuildsDoNot(t *testing.T) {
	f := newFixture(t, "linux", "v0.5.0-rc1")
	f.pre = true
	// a stable build asks /releases/latest, which the fixture answers with the pre-release: it must be ignored
	if info, _ := f.updater("v0.4.0", "linux").Check(context.Background()); info.Available {
		t.Error("a stable build must not be offered a pre-release")
	}
	if info, _ := f.updater("v0.4.0-rc1", "linux").Check(context.Background()); !info.Available {
		t.Error("a pre-release build should be offered a newer pre-release")
	}
}

func TestDownloadVerifiesTheSignatureAndExtractsTheBinary(t *testing.T) {
	for _, goos := range []string{"linux", "windows"} {
		f := newFixture(t, goos, "v0.5.0")
		u := f.updater("v0.4.0", goos)
		info, err := u.Check(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		p, err := u.Fetch(context.Background(), info, dir)
		if err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		got, _ := os.ReadFile(p)
		if !bytes.Equal(got, f.binary) {
			t.Errorf("%s: extracted %q", goos, got)
		}
		if filepath.Dir(p) != dir {
			t.Errorf("%s: the new binary must sit beside the old one so the rename is atomic, got %s", goos, p)
		}
	}
}

func TestDownloadRefusesWhatIsNotSignedByTheKey(t *testing.T) {
	f := newFixture(t, "linux", "v0.5.0")
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	u := f.updater("v0.4.0", "linux")
	u.PublicKey = other
	info, _ := u.Check(context.Background())
	if _, err := u.Fetch(context.Background(), info, t.TempDir()); err == nil || !strings.Contains(err.Error(), "signature") {
		t.Errorf("err = %v, want a signature failure", err)
	}
}

func TestDownloadRefusesAnArchiveThatDoesNotMatchTheSignedChecksum(t *testing.T) {
	f := newFixture(t, "linux", "v0.5.0")
	f.archive = append(f.archive, 'x') // the file was swapped after signing
	u := f.updater("v0.4.0", "linux")
	info, _ := u.Check(context.Background())
	if _, err := u.Fetch(context.Background(), info, t.TempDir()); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("err = %v, want a checksum failure", err)
	}
}

func TestDownloadRefusesAReleaseWithoutASignature(t *testing.T) {
	f := newFixture(t, "linux", "v0.5.0")
	f.sig = ""
	u := f.updater("v0.4.0", "linux")
	info, _ := u.Check(context.Background())
	info.sigURL = ""
	if _, err := u.Fetch(context.Background(), info, t.TempDir()); err == nil || !strings.Contains(err.Error(), "signed") {
		t.Errorf("err = %v, want a refusal because the release is not signed", err)
	}
}

func TestOnlyGitHubHostsAreTrusted(t *testing.T) {
	for host, ok := range map[string]bool{
		"github.com": true, "api.github.com": true, "objects.githubusercontent.com": true, "release-assets.githubusercontent.com": true,
		"evil.com": false, "github.com.evil.com": false, "githubusercontent.com.evil.org": false,
	} {
		if githubHost(host) != ok {
			t.Errorf("githubHost(%q) = %v", host, !ok)
		}
	}
	f := newFixture(t, "linux", "v0.5.0")
	u := f.updater("v0.4.0", "linux")
	u.AllowHost = githubHost // the real rule: the test server is not GitHub
	info, err := u.Check(context.Background())
	if err == nil {
		t.Errorf("a release API on %s was accepted: %+v", f.srv.URL, info)
	}
}

func TestArchiveMembersCannotEscapeTheDirectory(t *testing.T) {
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	tw.WriteHeader(&tar.Header{Name: "../../sshdeck", Mode: 0o755, Size: 3})
	tw.Write([]byte("bad"))
	tw.Close()
	gz.Close()
	if _, err := extract(b.Bytes(), "x.tar.gz", "linux", t.TempDir()); err == nil {
		t.Error("an archive with a path outside the folder was accepted")
	}
}

func TestApplyReplacesTheRunningBinaryOnLinux(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sshdeck")
	os.WriteFile(exe, []byte("old"), 0o755)
	nb := filepath.Join(dir, ".sshdeck-update-1")
	os.WriteFile(nb, []byte("new"), 0o755)
	if err := Apply(nb, exe, "linux"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Errorf("exe = %q", b)
	}
	if _, err := os.Stat(nb); !os.IsNotExist(err) {
		t.Error("the temporary file was left behind")
	}
}

func TestApplyOnWindowsRenamesTheRunningExeAndCleansUpLater(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sshdeck.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	nb := filepath.Join(dir, ".sshdeck-update-1")
	os.WriteFile(nb, []byte("new"), 0o755)
	if err := Apply(nb, exe, "windows"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Errorf("exe = %q", b)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "old" {
		t.Errorf("the old exe must be kept as .old while it runs, got %q", b)
	}
	CleanupOld(exe)
	if _, err := os.Stat(exe + ".old"); !os.IsNotExist(err) {
		t.Error("CleanupOld left the .old file")
	}
}

func TestApplyOnWindowsWorksWhenAnOldFileIsStillLocked(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "sshdeck.exe")
	os.WriteFile(exe, []byte("old"), 0o755)
	// A leftover .old that cannot be removed (on Windows: still running or held by a scanner). A non-empty folder
	// of that name makes Remove fail here.
	os.MkdirAll(filepath.Join(exe+".old", "held"), 0o755)
	nb := filepath.Join(dir, ".sshdeck-update-1")
	os.WriteFile(nb, []byte("new"), 0o755)
	if err := Apply(nb, exe, "windows"); err != nil {
		t.Fatalf("a stuck .old must not stop the update: %v", err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "new" {
		t.Errorf("exe = %q", b)
	}
	if left, _ := filepath.Glob(exe + ".*.old"); len(left) != 1 {
		t.Errorf("the running program should have been moved aside under another name: %v", left)
	}
	os.RemoveAll(exe + ".old") // as if the lock had ended
	CleanupOld(exe)
	if left, _ := filepath.Glob(exe + ".*.old"); len(left) != 0 {
		t.Errorf("CleanupOld should remove those too: %v", left)
	}
}

func TestApplyIntoAFolderThatCannotBeWrittenSaysSo(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write anywhere")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "sshdeck")
	os.WriteFile(exe, []byte("old"), 0o755)
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)
	f := newFixture(t, "linux", "v0.5.0")
	u := f.updater("v0.4.0", "linux")
	info, _ := u.Check(context.Background())
	_, err := u.Fetch(context.Background(), info, dir)
	var nw *NotWritableError
	if !errorsAs(err, &nw) {
		t.Errorf("err = %v, want NotWritableError", err)
	}
}
