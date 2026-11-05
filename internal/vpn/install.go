package vpn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The Windows VPN client, openconnect, comes in the OpenConnect-GUI installer. sshdeck can install it from the
// elevated helper: it downloads exactly this file, checks the SHA-256 below, runs it silently, and checks that
// openconnect.exe ended up where sshdeck looks. The job takes no parameters, so a process that learned the helper's
// token can only ever make it install this one pinned file. To move to a newer version, change both constants after
// downloading and checking the new installer.
const (
	openconnectInstallerURL    = "https://www.infradead.org/openconnect-gui/download/openconnect-gui-1.6.2-win64.exe"
	openconnectInstallerSHA256 = "de08d8968e40e219932d01025521f879178ec99246802db488c0fdac9fcef11a"
	openconnectInstallerSize   = 40 << 20

	// InstallOpenconnectMarker names the install job in a helper request. It stands where a program would.
	InstallOpenconnectMarker = "sshdeck:install-openconnect"
)

type installEnv struct {
	client  *http.Client
	url     string
	sha     string
	maxSize int64
	tmpRoot string // a folder that ordinary users cannot write to: the installer is run from here
	run     func(ctx context.Context, path string, args ...string) error
	found   func() bool // openconnect.exe is where sshdeck looks for it
	log     func(string)
}

func installOpenconnect(ctx context.Context, env installEnv) error {
	if env.maxSize == 0 {
		env.maxSize = openconnectInstallerSize
	}
	env.log("Downloading the OpenConnect installer…")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, env.url, nil)
	if err != nil {
		return err
	}
	resp, err := env.client.Do(req)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: the server answered %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, env.maxSize+1))
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if int64(len(data)) > env.maxSize {
		return errors.New("the installer is larger than expected, so it was not used")
	}
	h := sha256.Sum256(data)
	if hex.EncodeToString(h[:]) != env.sha {
		return errors.New("the downloaded installer does not match the checksum sshdeck expects, so nothing was installed")
	}
	env.log("Checksum matches.")

	if err := os.MkdirAll(env.tmpRoot, 0o755); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(env.tmpRoot, "install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "openconnect-gui-setup.exe")
	if err := os.WriteFile(path, data, 0o700); err != nil {
		return err
	}
	env.log("Running the installer (this can take a minute)…")
	rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	if err := env.run(rctx, path, "/S"); err != nil {
		return fmt.Errorf("the installer did not finish: %w", err)
	}
	if !env.found() {
		return errors.New("the installer finished, but openconnect.exe is not where sshdeck looks for it. Run the OpenConnect-GUI installer yourself and tick the openconnect.exe component, or install OpenConnect for all users another way")
	}
	env.log("OpenConnect is installed.")
	return nil
}

// defaultInstall is the real job, run by the elevated helper.
func defaultInstall(ctx context.Context, log func(string)) error {
	root := os.Getenv("ProgramData")
	if root == "" {
		root = os.TempDir()
	}
	return installOpenconnect(ctx, installEnv{
		client:  &http.Client{Timeout: 5 * time.Minute},
		url:     openconnectInstallerURL,
		sha:     openconnectInstallerSHA256,
		tmpRoot: filepath.Join(root, "sshdeck"),
		run: func(ctx context.Context, path string, args ...string) error {
			return exec.CommandContext(ctx, path, args...).Run()
		},
		found: func() bool { _, err := defaultLookPath("openconnect"); return err == nil },
		log:   log,
	})
}

// funcProc runs a function as if it were a client process: its log lines are the output, its error the exit status.
type funcProc struct {
	lines  chan string
	done   chan struct{}
	err    error
	cancel context.CancelFunc
}

func newFuncProc(fn func(ctx context.Context, log func(string)) error) *funcProc {
	ctx, cancel := context.WithCancel(context.Background())
	p := &funcProc{lines: make(chan string, 64), done: make(chan struct{}), cancel: cancel}
	go func() {
		p.err = fn(ctx, func(s string) { p.lines <- s })
		close(p.lines)
		close(p.done)
	}()
	return p
}

func (p *funcProc) Lines() <-chan string { return p.lines }
func (p *funcProc) Wait() error          { <-p.done; return p.err }
func (p *funcProc) Stop() error          { p.cancel(); <-p.done; return nil }

// helperExec is what the elevated helper runs requests with: the install job by name, anything else as a process.
type helperExec struct {
	inner   Starter
	install func(ctx context.Context, log func(string)) error
}

func (h *helperExec) Start(ctx context.Context, argv []string, stdin string) (Proc, error) {
	if len(argv) == 1 && argv[0] == InstallOpenconnectMarker {
		return newFuncProc(h.install), nil
	}
	return h.inner.Start(ctx, argv, stdin)
}

// allowHelper is the helper's rule for what it will do: the install job with no arguments, or openconnect with the
// options validateOpenconnectArgs accepts. Nothing else.
func allowHelper(argv []string) error {
	if len(argv) > 0 && argv[0] == InstallOpenconnectMarker {
		if len(argv) != 1 {
			return errors.New("the install job takes no arguments")
		}
		return nil
	}
	return allowOpenconnect(argv)
}

// RunInstall asks the elevated helper to install openconnect, passing its progress lines to onLog. It returns the job's
// error, or nil when openconnect is installed.
func RunInstall(ctx context.Context, s Starter, onLog func(string)) error {
	p, err := s.Start(ctx, []string{InstallOpenconnectMarker}, "")
	if err != nil {
		return err
	}
	go func() { // a cancelled context stops the job
		select {
		case <-ctx.Done():
			_ = p.Stop()
		case <-time.After(15 * time.Minute):
		}
	}()
	for l := range p.Lines() {
		onLog(strings.TrimRight(l, "\r\n"))
	}
	return p.Wait()
}
