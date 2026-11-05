package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/KushalNaral/sshdeck/internal/audit"
	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/update"
	"github.com/KushalNaral/sshdeck/internal/vpn"
)

// App holds the Go side the UI calls through Wails bindings.
type App struct {
	ctx     context.Context
	st      *store.Store
	vpn     vpn.Manager
	version string // the running release, "dev" for a development build
	// The update steps, replaceable in tests.
	updCheck   func(context.Context) (update.Info, error)
	updInstall func(context.Context, update.Info) error
	starter    vpn.Starter // what runs VPN clients; closed at shutdown (on Windows that ends the elevated helper)

	// Seams so tests can run without a window or a network.
	emit     func(name string, data ...any)
	dial     func(ctx context.Context, srv store.Server, secret string) (sshx.Client, error)
	lookPath func(file string) (string, error)

	// The clipboard, and how long a copied password stays on it.
	clipSet func(string) error
	clipGet func() (string, error)
	clipTTL time.Duration

	// audit records an event (what happened, to what, a short detail). It never gets a secret.
	audit func(event, subject, detail string)
	alog  *audit.Log

	vaultPath, settingsPath, plainPath string
	secrets                            store.Secrets    // where passwords go when the passphrase lock is turned off
	clock                              func() time.Time // a seam for the idle timer
	vpnPoll                            time.Duration    // how often a live connection checks that its VPN tunnel is still up
	backoff                            []time.Duration  // waits before each reconnect attempt; its length is the number of attempts

	mu         sync.Mutex
	settings   Settings
	lastActive time.Time
	conns      map[string]*conn         // live connections by server id
	cancels    map[string]func()        // connects still in flight
	manual     map[string]chan struct{} // waiting on "I connected the VPN" by server id
	pending    *pending                 // the CSV waiting for the user to pick rows
}

func NewApp() (*App, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(dir, "sshdeck")
	if err := checkConfigDir(base); err != nil {
		return nil, err
	}
	var st *store.Store
	if _, serr := os.Stat(filepath.Join(base, "vault.enc")); serr == nil {
		st, err = store.OpenLocked(filepath.Join(base, "vault.enc")) // the passphrase lock is on: start locked
	} else {
		st, err = store.Open(filepath.Join(base, "data.json"), store.Keyring{})
	}
	if err != nil {
		return nil, err
	}
	vpn.SweepStale(vpn.StaleDir(), time.Minute) // a killed connect can leave a config file holding a VPN password
	starter := vpn.DefaultStarter()
	a := newApp(st, vpn.New(starter), sshx.Dial)
	a.starter = starter
	a.vaultPath, a.settingsPath, a.plainPath = filepath.Join(base, "vault.enc"), filepath.Join(base, "settings.json"), filepath.Join(base, "data.json")
	a.settings = loadSettings(a.settingsPath)
	actor := "unknown"
	if u, err := user.Current(); err == nil {
		actor = u.Username
	}
	if l, err := audit.Open(filepath.Join(base, "audit.log"), actor); err != nil {
		log.Printf("audit log unavailable: %v", err)
	} else {
		a.alog = l
		a.audit = func(event, subject, detail string) {
			if err := l.Add(event, subject, detail); err != nil {
				log.Printf("audit: %v", err)
			}
		}
	}
	a.emit = func(name string, data ...any) { runtime.EventsEmit(a.ctx, name, data...) }
	a.useSystemClipboard()
	return a, nil
}

func newApp(st *store.Store, m vpn.Manager, dial func(context.Context, store.Server, string) (sshx.Client, error)) *App {
	a := &App{
		ctx: context.Background(), st: st, vpn: m, dial: dial, lookPath: vpn.LookPath,
		emit: func(string, ...any) {}, audit: func(string, string, string) {},
		clipSet: func(string) error { return nil }, clipGet: func() (string, error) { return "", nil }, clipTTL: clipboardTTL,
		secrets: store.Keyring{}, clock: time.Now, vpnPoll: 5 * time.Second, backoff: []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 30 * time.Second}, settings: defaultSettings(), lastActive: time.Now(),
		conns: map[string]*conn{}, cancels: map[string]func(){}, manual: map[string]chan struct{}{}, version: version,
	}
	a.updCheck = func(ctx context.Context) (update.Info, error) { return a.updater().Check(ctx) }
	a.updInstall = a.realInstall
	return a
}

// secondLaunch runs in the first copy when the user starts sshdeck again: show the window instead.
func (a *App) secondLaunch(options.SecondInstanceData) {
	runtime.WindowUnminimise(a.ctx)
	runtime.WindowShow(a.ctx)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if exe, err := exePath(); err == nil {
		update.CleanupOld(exe) // the previous program left behind by a Windows update
	}
	a.checkUpdateOnStart(5 * time.Second)
	a.audit("app.start", "", "")
	go a.idleLoop(ctx)
}

// shutdown closes every shell and stops every VPN this app started.
func (a *App) shutdown(context.Context) {
	a.audit("app.stop", "", "")
	a.disconnectAll()
	if c, ok := a.starter.(io.Closer); ok {
		_ = c.Close()
	}
}

func (a *App) disconnectAll() {
	a.mu.Lock()
	ids := make([]string, 0, len(a.conns))
	for id := range a.conns {
		ids = append(ids, id)
	}
	a.mu.Unlock()
	for _, id := range ids {
		a.Disconnect(id)
	}
}

func (a *App) ListServers() []store.Server { return a.st.Servers() }
func (a *App) ListVPNs() []store.VPN       { return a.st.VPNs() }

func (a *App) SaveServer(s store.Server, password string) (store.Server, error) {
	saved, err := a.st.SaveServer(s, password)
	if err == nil {
		a.audit("server.save", saved.Name, "")
	}
	return saved, err
}
func (a *App) DeleteServer(id string) error {
	name := a.serverName(id)
	a.Disconnect(id)
	err := a.st.DeleteServer(id)
	if err == nil {
		a.audit("server.delete", name, "")
	}
	return err
}

// serverName is the name to put in the audit trail; it falls back to the id when the server is gone or the app is locked.
func (a *App) serverName(id string) string {
	if sv, ok := a.server(id); ok {
		return sv.Name
	}
	return id
}

func (a *App) vpnName(id string) string {
	for _, v := range a.st.VPNs() {
		if v.ID == id {
			return v.Name
		}
	}
	return id
}

func (a *App) SaveVPN(v store.VPN, password string) (store.VPN, error) {
	saved, err := a.st.SaveVPN(v, password)
	if err == nil {
		a.audit("vpn.save", saved.Name, "")
	}
	return saved, err
}
func (a *App) DeleteVPN(id string) error {
	name := a.vpnName(id)
	err := a.st.DeleteVPN(id)
	if err == nil {
		a.audit("vpn.delete", name, "")
	}
	return err
}

func (a *App) ListTasks() []store.Task { return a.st.Tasks() }
func (a *App) SaveTask(t store.Task) (store.Task, error) {
	saved, err := a.st.SaveTask(t)
	if err == nil {
		a.audit("task.save", saved.Title, "")
	}
	return saved, err
}
func (a *App) DeleteTask(id string) error {
	title := id
	for _, t := range a.st.Tasks() {
		if t.ID == id {
			title = t.Title
		}
	}
	err := a.st.DeleteTask(id)
	if err == nil {
		a.audit("task.delete", title, "")
	}
	return err
}

// beforeClose asks before closing a window that still has connections, since closing ends their shells and VPNs.
func (a *App) beforeClose(ctx context.Context) (prevent bool) {
	a.mu.Lock()
	n := len(a.conns)
	a.mu.Unlock()
	if n == 0 {
		return false
	}
	const closeIt = "Close and disconnect"
	pick, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Close sshdeck?",
		Message:       fmt.Sprintf("%d server(s) are connected. Closing ends their shells and stops their VPNs.", n),
		Buttons:       []string{closeIt, "Stay"},
		DefaultButton: "Stay",
		CancelButton:  "Stay",
	})
	return err != nil || pick != closeIt
}
