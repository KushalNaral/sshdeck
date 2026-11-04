package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/KushalNaral/sshdeck/internal/sshx"
	"github.com/KushalNaral/sshdeck/internal/store"
	"github.com/KushalNaral/sshdeck/internal/vpn"
)

// App holds the Go side the UI calls through Wails bindings.
type App struct {
	ctx context.Context
	st  *store.Store
	vpn vpn.Manager

	// Seams so tests can run without a window or a network.
	emit     func(name string, data ...any)
	open     func(ctx context.Context, srv store.Server, secret string) (sshx.Session, error)
	lookPath func(file string) (string, error)

	mu      sync.Mutex
	conns   map[string]*conn         // live connections by server id
	cancels map[string]func()        // connects still in flight
	manual  map[string]chan struct{} // waiting on "I connected the VPN" by server id
}

func NewApp() (*App, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(filepath.Join(dir, "sshdeck", "data.json"), store.Keyring{})
	if err != nil {
		return nil, err
	}
	a := newApp(st, vpn.New(vpn.ExecStarter{}), sshx.Open)
	a.emit = func(name string, data ...any) { runtime.EventsEmit(a.ctx, name, data...) }
	return a, nil
}

func newApp(st *store.Store, m vpn.Manager, open func(context.Context, store.Server, string) (sshx.Session, error)) *App {
	return &App{
		ctx: context.Background(), st: st, vpn: m, open: open, lookPath: exec.LookPath,
		emit:  func(string, ...any) {},
		conns: map[string]*conn{}, cancels: map[string]func(){}, manual: map[string]chan struct{}{},
	}
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// shutdown closes every shell and stops every VPN this app started.
func (a *App) shutdown(context.Context) {
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
	return a.st.SaveServer(s, password)
}
func (a *App) DeleteServer(id string) error {
	a.Disconnect(id)
	return a.st.DeleteServer(id)
}

func (a *App) SaveVPN(v store.VPN, password string) (store.VPN, error) {
	return a.st.SaveVPN(v, password)
}
func (a *App) DeleteVPN(id string) error { return a.st.DeleteVPN(id) }

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
