package main

// File history for the editor: every save keeps the earlier text, so a change can be seen and put back.
// internal/history stores it; this file decides when a version is recorded and who made it.

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"time"

	"github.com/pkg/sftp"

	"github.com/KushalNaral/sshdeck/internal/history"
)

// who names the person at this keyboard: user and machine. The SSH login is recorded separately.
func who() string {
	u, err := user.Current()
	name := "unknown"
	if err == nil {
		name = u.Username
	}
	host, _ := os.Hostname()
	if host == "" {
		return name
	}
	return name + "@" + host
}

// hist opens the history store on first use. It returns nil when history is off or unavailable.
func (a *App) hist() *history.Store {
	a.mu.Lock()
	off, keep, days := a.settings.NoHistory, a.settings.HistoryKeep, a.settings.HistoryDays
	hs := a.hs
	a.mu.Unlock()
	if off || a.histDir == "" || a.isLocked() {
		return nil
	}
	if hs == nil {
		key, err := a.st.HistoryKey()
		if err != nil {
			return nil
		}
		if hs, err = history.Open(a.histDir, key); err != nil {
			return nil
		}
		a.mu.Lock()
		if a.hs == nil {
			a.hs = hs
		}
		hs = a.hs
		a.mu.Unlock()
	}
	hs.Keep, hs.Days = keep, days
	return hs
}

func (a *App) denied(p string) bool {
	a.mu.Lock()
	extra := a.settings.HistoryDeny
	a.mu.Unlock()
	return history.Denied(p, extra)
}

// track records content as the file's current text. author is "" for a change sshdeck did not make.
// It never fails the caller's operation: a problem is only written to the audit trail.
func (a *App) track(serverID, p string, kind history.Kind, author string, content []byte) {
	hs := a.hist()
	if hs == nil || a.denied(p) {
		return
	}
	login := ""
	if sv, ok := a.server(serverID); ok {
		login = sv.User
	}
	v, added, err := hs.Record(history.Version{Server: serverID, Path: p, Author: author, Login: login, Kind: kind}, content)
	if err != nil {
		a.audit("history.fail", a.serverName(serverID), p+": "+err.Error())
		return
	}
	if added {
		a.share(serverID, hs, v, content)
	}
}

// shared returns the server's SFTP client when its history is also kept on the server itself.
func (a *App) shared(serverID string) *sftp.Client {
	a.mu.Lock()
	off := a.settings.NoSharedHistory
	a.mu.Unlock()
	sv, ok := a.server(serverID)
	if off || !ok {
		return nil
	}
	switch sv.HistoryCopy {
	case "off":
		return nil
	case "on":
	default:
		if sv.Production {
			return nil // production servers only get the copy once someone turns it on
		}
	}
	cl, err := a.ftp(serverID)
	if err != nil {
		return nil
	}
	return cl
}

// share writes a version to the server's copy. A failure only goes to the audit trail.
func (a *App) share(serverID string, hs *history.Store, v history.Version, content []byte) {
	cl := a.shared(serverID)
	if cl == nil {
		return
	}
	keep, days := hs.Keep, hs.Days
	if keep == 0 {
		keep = history.DefaultKeep
	}
	if days == 0 {
		days = history.DefaultDays
	}
	if err := history.WriteRemote(cl, v, content); err != nil {
		a.audit("history.share.fail", a.serverName(serverID), v.Path+": "+err.Error())
		return
	}
	history.PruneRemote(cl, v.Path, keep, days, time.Now())
}

// pull adds what teammates (or this person on another machine) saved to the server's copy to the local list.
func (a *App) pull(serverID, p string, hs *history.Store) {
	cl := a.shared(serverID)
	if cl == nil {
		return
	}
	var vs []history.Version
	var err error
	if p == "" {
		vs, err = history.RecentRemote(cl, serverID, 200)
	} else {
		vs, err = history.ListRemote(cl, serverID, p)
	}
	if err != nil {
		return
	}
	for _, v := range vs {
		hs.Import(v)
	}
}

// SetHistoryCopy turns the copy on the server on or off for one server ("" goes back to the default).
func (a *App) SetHistoryCopy(serverID, mode string) error {
	if err := a.guard(); err != nil {
		return err
	}
	if err := a.st.SetHistoryCopy(serverID, mode); err != nil {
		return err
	}
	a.audit("settings.change", a.serverName(serverID), "history copy="+mode)
	return nil
}

// noteCurrent compares what is on the server now with the newest recorded version. A file never seen before is
// recorded as it is (so the first edit can be undone); a file that differs was changed by something else.
func (a *App) noteCurrent(serverID, p string, content []byte) {
	hs := a.hist()
	if hs == nil || a.denied(p) {
		return
	}
	if _, ok := hs.Latest(serverID, p); !ok {
		a.track(serverID, p, history.Initial, "", content)
		return
	}
	a.track(serverID, p, history.Outside, "", content) // adds nothing when the text is the same
}

// FileHistory lists the versions of one file, newest first. It needs no connection.
func (a *App) FileHistory(serverID, p string) ([]history.Version, error) {
	if err := a.guard(); err != nil {
		return nil, err
	}
	if hs := a.hist(); hs != nil {
		a.pull(serverID, p, hs)
		return hs.List(serverID, p), nil
	}
	return nil, nil
}

// ServerHistory lists the latest edits across every file of a server: who changed what, and when.
func (a *App) ServerHistory(serverID string, limit int) ([]history.Version, error) {
	if err := a.guard(); err != nil {
		return nil, err
	}
	if hs := a.hist(); hs != nil {
		a.pull(serverID, "", hs)
		return hs.Recent(serverID, limit), nil
	}
	return nil, nil
}

func (a *App) HistoryContent(serverID, versionID string) (string, error) {
	if err := a.guard(); err != nil {
		return "", err
	}
	hs := a.hist()
	if hs == nil {
		return "", errors.New("file history is off")
	}
	b, err := hs.Content(serverID, versionID)
	if err != nil {
		// A version a teammate saved: its text is on the server's copy until the first time it is opened here.
		if v, ok := hs.Get(serverID, versionID); ok && v.Remote {
			if cl := a.shared(serverID); cl != nil {
				if b, err = history.ContentRemote(cl, v); err == nil {
					if err = hs.Fetched(serverID, versionID, b); err == nil {
						return string(b), nil
					}
				}
			}
		}
		return "", err
	}
	return string(b), nil
}

func (a *App) PinVersion(serverID, versionID string, pinned bool, note string) error {
	if err := a.guard(); err != nil {
		return err
	}
	hs := a.hist()
	if hs == nil {
		return errors.New("file history is off")
	}
	return hs.Pin(serverID, versionID, pinned, note)
}

// RestoreVersion writes an older version back to the server. It is an ordinary save (so it is refused if the
// file changed meanwhile and itself becomes a new version); history is never rewritten.
func (a *App) RestoreVersion(serverID, p, versionID string) (string, error) {
	text, err := a.HistoryContent(serverID, versionID)
	if err != nil {
		return "", err
	}
	cl, err := a.ftp(serverID)
	if err != nil {
		return "", err
	}
	cur := ""
	if fi, err := cl.Stat(p); err == nil {
		cur = versionOf(fi)
	}
	v, err := a.writeText(serverID, p, text, cur, history.Restore)
	if err == nil {
		a.audit("file.restore", a.serverName(serverID), p)
	}
	return v, err
}

// DeleteHistory forgets the history of one file, or of the whole server when p is empty.
func (a *App) DeleteHistory(serverID, p string) error {
	if err := a.guard(); err != nil {
		return err
	}
	hs := a.hist()
	if hs == nil {
		return nil
	}
	if err := hs.Delete(serverID, p); err != nil {
		return err
	}
	if cl := a.shared(serverID); cl != nil {
		if err := history.DeleteRemote(cl, p); err != nil {
			a.audit("history.share.fail", a.serverName(serverID), "delete: "+err.Error())
		}
	}
	what := p
	if what == "" {
		what = "all files"
	}
	a.audit("history.delete", a.serverName(serverID), what)
	return nil
}

type HistorySettings struct {
	Off          bool     `json:"off"`
	NoServerCopy bool     `json:"noServerCopy"`
	Keep         int      `json:"keep"`
	Days         int      `json:"days"`
	Deny         []string `json:"deny"`
}

func (a *App) GetHistorySettings() HistorySettings {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.settings
	h := HistorySettings{Off: s.NoHistory, NoServerCopy: s.NoSharedHistory, Keep: s.HistoryKeep, Days: s.HistoryDays, Deny: s.HistoryDeny}
	if h.Keep == 0 {
		h.Keep = history.DefaultKeep
	}
	if h.Days == 0 {
		h.Days = history.DefaultDays
	}
	return h
}

func (a *App) SetHistorySettings(h HistorySettings) error {
	if h.Keep < 1 || h.Keep > 1000 || h.Days < 1 || h.Days > 3650 {
		return errors.New("keep between 1 and 1000 versions, for 1 to 3650 days")
	}
	a.mu.Lock()
	a.settings.NoHistory, a.settings.NoSharedHistory, a.settings.HistoryKeep, a.settings.HistoryDays, a.settings.HistoryDeny = h.Off, h.NoServerCopy, h.Keep, h.Days, h.Deny
	s := a.settings
	a.mu.Unlock()
	a.audit("settings.change", "", fmt.Sprintf("history off=%t keep=%d days=%d", h.Off, h.Keep, h.Days))
	return saveSettings(a.settingsPath, s)
}
