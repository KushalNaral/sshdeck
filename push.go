package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// PushResult says what happened for one server of a push.
type PushResult struct {
	Server    string   `json:"server"`
	Name      string   `json:"name"`
	Started   int      `json:"started"`
	Conflicts []string `json:"conflicts,omitempty"` // names that already existed there
	Error     string   `json:"error,omitempty"`
}

// PushTo copies local files or folders into remoteDir on each given server, through the normal transfer queue.
// Only connected servers are used; the others come back with an error and are never dialled. The conflict policy
// (skip, replace or keep) is chosen once: asking per server does not work across many.
func (a *App) PushTo(serverIDs []string, remoteDir string, locals []string, policy string) ([]PushResult, error) {
	switch {
	case len(serverIDs) == 0:
		return nil, errors.New("pick at least one server")
	case len(locals) == 0:
		return nil, errors.New("pick at least one file or folder")
	case policy != "skip" && policy != "replace" && policy != "keep":
		return nil, errors.New("choose what to do when a file already exists: skip, replace or keep both")
	}
	if err := checkRemote(remoteDir); err != nil {
		return nil, err
	}
	out := make([]PushResult, 0, len(serverIDs))
	for _, id := range serverIDs {
		r := PushResult{Server: id, Name: a.serverName(id)}
		u, err := a.Upload(id, remoteDir, locals, policy)
		switch {
		case err != nil && err.Error() == "not connected":
			r.Error = "Not connected. Connect to this server first."
		case err != nil:
			r.Error = err.Error()
		default:
			r.Started, r.Conflicts = u.Started, u.Conflicts
		}
		out = append(out, r)
	}
	names := make([]string, len(locals))
	for i, l := range locals {
		names[i] = filepath.Base(l)
	}
	a.audit("push", strings.Join(names, ", "), fmt.Sprintf("%d servers, into %s", len(serverIDs), remoteDir))
	return out, nil
}

// PickPush asks for files (or a folder) on this machine, then pushes them to every server in serverIDs.
func (a *App) PickPush(serverIDs []string, remoteDir string, folder bool, policy string) ([]PushResult, error) {
	var picks []string
	var err error
	if folder {
		var d string
		d, err = runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Copy a folder to the servers"})
		if d != "" {
			picks = []string{d}
		}
	} else {
		picks, err = runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "Copy files to the servers"})
	}
	if err != nil {
		return nil, err
	}
	if len(picks) == 0 {
		return nil, nil
	}
	return a.PushTo(serverIDs, remoteDir, picks, policy)
}
