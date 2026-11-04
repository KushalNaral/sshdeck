package main

import (
	"context"
	"os"
	"path/filepath"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// App holds the Go side the UI calls through Wails bindings.
type App struct {
	ctx context.Context
	st  *store.Store
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
	return &App{st: st}, nil
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) ListServers() []store.Server { return a.st.Servers() }
func (a *App) ListVPNs() []store.VPN       { return a.st.VPNs() }

func (a *App) SaveServer(s store.Server, password string) (store.Server, error) {
	return a.st.SaveServer(s, password)
}
func (a *App) DeleteServer(id string) error { return a.st.DeleteServer(id) }

func (a *App) SaveVPN(v store.VPN, password string) (store.VPN, error) {
	return a.st.SaveVPN(v, password)
}
func (a *App) DeleteVPN(id string) error { return a.st.DeleteVPN(id) }
