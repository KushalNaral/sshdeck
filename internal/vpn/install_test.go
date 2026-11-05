package vpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type installRig struct {
	env    installEnv
	ran    []string
	order  []string // prepare, run and undo, in the order they happened
	logs   []string
	target string
}

func newInstallRig(t *testing.T, body []byte, hash string) *installRig {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(body) }))
	t.Cleanup(srv.Close)
	r := &installRig{target: filepath.Join(t.TempDir(), "openconnect.exe")}
	r.env = installEnv{
		client: srv.Client(), url: srv.URL + "/openconnect-gui.exe", sha: hash, tmpRoot: t.TempDir(),
		run: func(_ context.Context, path string, args ...string) error {
			r.ran = append(r.ran, path+" "+strings.Join(args, " "))
			r.order = append(r.order, "run")
			return os.WriteFile(r.target, []byte("exe"), 0o755) // what a real installer would leave behind
		},
		found: func() bool { _, err := os.Stat(r.target); return err == nil },
		log:   func(s string) { r.logs = append(r.logs, s) },
		prepare: func() (func(), error) {
			r.order = append(r.order, "prepare")
			return func() { r.order = append(r.order, "undo") }, nil
		},
	}
	return r
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func TestInstallerIsCheckedThenRunSilently(t *testing.T) {
	body := []byte("installer bytes")
	r := newInstallRig(t, body, sum(body))
	if err := installOpenconnect(context.Background(), r.env); err != nil {
		t.Fatal(err)
	}
	if len(r.ran) != 1 || !strings.HasSuffix(r.ran[0], " /S") {
		t.Errorf("ran %v, want one silent run", r.ran)
	}
	if ents, _ := os.ReadDir(r.env.tmpRoot); len(ents) != 0 {
		t.Errorf("the downloaded installer was left behind: %v", ents)
	}
}

func TestInstallerWithTheWrongHashIsNeverRun(t *testing.T) {
	r := newInstallRig(t, []byte("tampered"), sum([]byte("the real one")))
	err := installOpenconnect(context.Background(), r.env)
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v", err)
	}
	if len(r.ran) != 0 {
		t.Error("an installer that failed the check was run")
	}
}

func TestInstallerThatLeavesNoOpenconnectIsReported(t *testing.T) {
	body := []byte("x")
	r := newInstallRig(t, body, sum(body))
	r.env.run = func(context.Context, string, ...string) error { return nil } // finished, but the component was not installed
	err := installOpenconnect(context.Background(), r.env)
	if err == nil || !strings.Contains(err.Error(), "openconnect.exe") {
		t.Errorf("err = %v, want an explanation that openconnect.exe is missing", err)
	}
}

func TestInstallerFailureIsReturned(t *testing.T) {
	body := []byte("x")
	r := newInstallRig(t, body, sum(body))
	r.env.run = func(context.Context, string, ...string) error { return errors.New("exit status 2") }
	if err := installOpenconnect(context.Background(), r.env); err == nil || !strings.Contains(err.Error(), "exit status 2") {
		t.Errorf("err = %v", err)
	}
}

func TestOversizedDownloadIsRefused(t *testing.T) {
	big := make([]byte, 300)
	r := newInstallRig(t, big, sum(big))
	r.env.maxSize = 100
	if err := installOpenconnect(context.Background(), r.env); err == nil {
		t.Error("a download over the size limit was accepted")
	}
}

func TestInstallGoesThroughTheHelperAndOnlyAsTheFixedJob(t *testing.T) {
	// The helper side: the install job is known by name and takes no arguments; anything else still needs the allow-list.
	if err := allowHelper([]string{InstallOpenconnectMarker}); err != nil {
		t.Errorf("the install job was refused: %v", err)
	}
	for _, bad := range [][]string{
		{InstallOpenconnectMarker, "extra"},
		{InstallOpenconnectMarker, "--url", "http://evil"},
		{"cmd.exe", "/c", InstallOpenconnectMarker},
	} {
		if err := allowHelper(bad); err == nil {
			t.Errorf("%v was allowed", bad)
		}
	}

	var ran bool
	under := &fakeStarter{script: printThenStay("unused")}
	hs := &HelperStarter{Launch: func(addr, token string) error {
		go func() {
			_ = RunHelperWith(addr, token, &helperExec{inner: under, install: func(ctx context.Context, log func(string)) error {
				ran = true
				log("Downloading…")
				log("Done.")
				return nil
			}}, allowHelper)
		}()
		return nil
	}}
	defer func() { _ = hs.Close() }()
	ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var got []string
	if err := RunInstall(ctx2, hs, func(l string) { got = append(got, l) }); err != nil {
		t.Fatal(err)
	}
	if !ran || len(got) != 2 || got[1] != "Done." {
		t.Errorf("ran=%v lines=%v", ran, got)
	}
	if under.n != 0 {
		t.Error("the install job must not start a client process")
	}
}

func TestInstallErrorsReachTheApp(t *testing.T) {
	hs := &HelperStarter{Launch: func(addr, token string) error {
		go func() {
			_ = RunHelperWith(addr, token, &helperExec{inner: &fakeStarter{script: printThenStay("")}, install: func(context.Context, func(string)) error {
				return errors.New("hash mismatch")
			}}, allowHelper)
		}()
		return nil
	}}
	defer func() { _ = hs.Close() }()
	ctx2, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := RunInstall(ctx2, hs, func(string) {}); err == nil || !strings.Contains(err.Error(), "hash mismatch") {
		t.Errorf("err = %v", err)
	}
}

func TestConsoleComponentIsSelectedBeforeTheInstallerRunsAndKeptOnSuccess(t *testing.T) {
	body := []byte("installer bytes")
	r := newInstallRig(t, body, sum(body))
	if err := installOpenconnect(context.Background(), r.env); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.order, ","); got != "prepare,run" {
		t.Errorf("order = %s: select the console component first, and keep it once the install worked", got)
	}
}

func TestConsoleComponentSettingIsUndoneWhenTheInstallFails(t *testing.T) {
	body := []byte("x")
	for name, fail := range map[string]func(*installRig){
		"installer error": func(r *installRig) {
			r.env.run = func(context.Context, string, ...string) error {
				r.order = append(r.order, "run")
				return errors.New("exit status 2")
			}
		},
		"no openconnect.exe afterwards": func(r *installRig) {
			r.env.run = func(context.Context, string, ...string) error { r.order = append(r.order, "run"); return nil }
		},
	} {
		r := newInstallRig(t, body, sum(body))
		fail(r)
		if err := installOpenconnect(context.Background(), r.env); err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if got := strings.Join(r.order, ","); got != "prepare,run,undo" {
			t.Errorf("%s: order = %s, want the registry put back", name, got)
		}
	}
}

func TestInstallStopsIfTheRegistryCannotBePrepared(t *testing.T) {
	body := []byte("x")
	r := newInstallRig(t, body, sum(body))
	r.env.prepare = func() (func(), error) { return nil, errors.New("access denied") }
	if err := installOpenconnect(context.Background(), r.env); err == nil || !strings.Contains(err.Error(), "access denied") {
		t.Errorf("err = %v", err)
	}
	if len(r.ran) != 0 {
		t.Error("the installer ran although it could not be set up to include openconnect.exe")
	}
}
