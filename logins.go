package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/KushalNaral/sshdeck/internal/store"
)

// clipboardTTL is how long a copied password stays on the clipboard.
const clipboardTTL = 30 * time.Second

func (a *App) SaveLogin(serverID string, l store.Login, secret string) (store.Login, error) {
	saved, err := a.st.SaveLogin(serverID, l, secret)
	if err == nil {
		a.audit("login.save", a.serverName(serverID), saved.Label)
	}
	return saved, err
}

func (a *App) DeleteLogin(serverID, loginID string) error {
	label := loginID
	if sv, ok := a.server(serverID); ok {
		for _, l := range sv.Logins {
			if l.ID == loginID {
				label = l.Label
			}
		}
	}
	err := a.st.DeleteLogin(serverID, loginID)
	if err == nil {
		a.audit("login.delete", a.serverName(serverID), label)
	}
	return err
}

// CopyLogin puts a login's user name or secret on the clipboard. The secret goes straight from the store to the
// clipboard, never to the UI, and is wiped after clipboardTTL unless something else has been copied since.
func (a *App) CopyLogin(serverID, loginID, field string) error {
	sv, ok := a.server(serverID)
	if !ok {
		return fmt.Errorf("that server no longer exists")
	}
	var login *store.Login
	for i := range sv.Logins {
		if sv.Logins[i].ID == loginID {
			login = &sv.Logins[i]
		}
	}
	if login == nil {
		return fmt.Errorf("that login no longer exists")
	}
	switch field {
	case "user":
		if err := a.clipSet(login.User); err != nil {
			return err
		}
	case "secret":
		secret, err := a.st.Password("login", serverID+"/"+loginID)
		if err != nil {
			return fmt.Errorf("no password is stored for %s", login.Label)
		}
		if err := a.clipSet(secret); err != nil {
			return err
		}
		time.AfterFunc(a.clipTTL, func() {
			if cur, err := a.clipGet(); err == nil && cur == secret {
				_ = a.clipSet("")
			}
		})
	default:
		return fmt.Errorf("unknown field %q", field)
	}
	a.audit("login.copy", sv.Name, login.Label+" "+field)
	return nil
}

func (a *App) useSystemClipboard() {
	a.clipSet = func(s string) error { return runtime.ClipboardSetText(a.ctx, s) }
	a.clipGet = func() (string, error) { return runtime.ClipboardGetText(a.ctx) }
}

// sshCommand is what to type in a terminal to log in to the server yourself. It holds no secret.
func sshCommand(sv store.Server) string {
	cmd := "ssh"
	if sv.Port != 22 {
		cmd += fmt.Sprintf(" -p %d", sv.Port)
	}
	if sv.Auth == "key" && sv.KeyPath != "" {
		cmd += " -i '" + strings.ReplaceAll(sv.KeyPath, "'", `'\''`) + "'"
	}
	return cmd + " " + sv.User + "@" + sv.Host
}

// CopySSHCommand puts the ssh command for a server on the clipboard.
func (a *App) CopySSHCommand(serverID string) error {
	sv, ok := a.server(serverID)
	if !ok {
		return fmt.Errorf("that server no longer exists")
	}
	return a.clipSet(sshCommand(sv))
}
