package main

// Uploads and downloads. Each item you start becomes one Transfer in a queue (three run at a time) that the
// UI follows through "transfers" events. A folder is one transfer that copies everything inside it.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type Transfer struct {
	ID       string `json:"id"`
	ServerID string `json:"serverId"`
	Name     string `json:"name"`
	Up       bool   `json:"up"`
	Done     int64  `json:"done"`
	Total    int64  `json:"total"`
	State    string `json:"state"` // queued, running, done, failed, cancelled
	Error    string `json:"error,omitempty"`
}

type xfer struct {
	Transfer
	cancel context.CancelFunc
	run    func(ctx context.Context, x *xfer) error
	done   atomic.Int64
	total  atomic.Int64
	last   time.Time
}

// UploadResult says what Upload or Download did. With policy "ask" and a name already taken, nothing starts and
// Conflicts lists the names; call again with the same Paths and a policy of skip, replace or keep.
type UploadResult struct {
	Started   int      `json:"started"`
	Conflicts []string `json:"conflicts,omitempty"`
	Paths     []string `json:"paths,omitempty"`
	Dest      string   `json:"dest,omitempty"`
	Cancelled bool     `json:"cancelled,omitempty"`
}

// unique returns name, or "name (2).ext" and so on, whichever does not exist yet.
func unique(name string, exists func(string) bool) string {
	if !exists(name) {
		return name
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		if n := fmt.Sprintf("%s (%d)%s", stem, i, ext); !exists(n) {
			return n
		}
	}
}

// safeName reports whether a file name received from the server is a single plain component. A hostile server
// could otherwise send "../../.bashrc" and have a download write outside the chosen folder.
func safeName(n string) bool {
	return n != "" && n != "." && n != ".." && !strings.ContainsAny(n, `/\`) && !strings.ContainsRune(n, 0)
}

// snapshot copies the queue for the UI. Caller holds xmu.
func (a *App) snapshot() []Transfer {
	out := make([]Transfer, len(a.xfers))
	for i, x := range a.xfers {
		t := x.Transfer
		t.Done, t.Total = x.done.Load(), x.total.Load()
		out[i] = t
	}
	return out
}

func (a *App) publish() {
	a.xmu.Lock()
	s := a.snapshot()
	a.xmu.Unlock()
	a.emit("transfers", s)
}

// progress is called as bytes move; it tells the UI at most ten times a second.
func (a *App) progress(x *xfer, n int64) {
	x.done.Add(n)
	a.xmu.Lock()
	due := time.Since(x.last) > 100*time.Millisecond
	if due {
		x.last = time.Now()
	}
	a.xmu.Unlock()
	if due {
		a.publish()
	}
}

func (a *App) enqueue(serverID, name string, up bool, run func(ctx context.Context, x *xfer) error) {
	ctx, cancel := context.WithCancel(a.ctx)
	a.xmu.Lock()
	a.xseq++
	x := &xfer{Transfer: Transfer{ID: fmt.Sprint("t", a.xseq), ServerID: serverID, Name: name, Up: up, State: "queued"}, cancel: cancel, run: run}
	a.xfers = append(a.xfers, x)
	a.xmu.Unlock()
	a.publish()
	go a.drive(ctx, x)
}

func (a *App) drive(ctx context.Context, x *xfer) {
	select {
	case a.xsem <- struct{}{}:
		defer func() { <-a.xsem }()
	case <-ctx.Done():
	}
	a.setState(x, "running", "")
	err := ctx.Err()
	if err == nil {
		err = x.run(ctx, x)
	}
	switch {
	case ctx.Err() != nil:
		a.setState(x, "cancelled", "")
	case err != nil:
		a.setState(x, "failed", err.Error())
	default:
		a.setState(x, "done", "")
	}
	verb := "file.download"
	if x.Up {
		verb = "file.upload"
	}
	a.audit(verb, a.serverName(x.ServerID), x.Name+" "+x.State)
}

func (a *App) setState(x *xfer, s, msg string) {
	a.xmu.Lock()
	if s == "running" && x.State != "queued" {
		a.xmu.Unlock()
		return
	}
	x.State, x.Error = s, msg
	a.xmu.Unlock()
	a.publish()
}

// Transfers returns the queue as it is now.
func (a *App) Transfers() []Transfer {
	a.xmu.Lock()
	defer a.xmu.Unlock()
	return a.snapshot()
}

func (a *App) CancelTransfer(id string) {
	a.xmu.Lock()
	for _, x := range a.xfers {
		if x.ID == id && (x.State == "queued" || x.State == "running") {
			x.cancel()
		}
	}
	a.xmu.Unlock()
}

// ClearTransfers forgets the finished ones.
func (a *App) ClearTransfers() {
	a.xmu.Lock()
	keep := a.xfers[:0]
	for _, x := range a.xfers {
		if x.State == "queued" || x.State == "running" {
			keep = append(keep, x)
		}
	}
	a.xfers = keep
	a.xmu.Unlock()
	a.publish()
}

// RetryTransfer starts a failed or cancelled transfer again as a new entry.
func (a *App) RetryTransfer(id string) {
	a.xmu.Lock()
	var old *xfer
	for _, x := range a.xfers {
		if x.ID == id && (x.State == "failed" || x.State == "cancelled") {
			old = x
		}
	}
	a.xmu.Unlock()
	if old != nil {
		a.enqueue(old.ServerID, old.Name, old.Up, old.run)
	}
}

// copyCtx copies src to dst, counting bytes and stopping when ctx ends.
func (a *App) copyCtx(ctx context.Context, x *xfer, dst io.Writer, src io.Reader) error {
	buf := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			a.progress(x, int64(n))
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			return rerr
		}
	}
}

// ---- upload ----

// Upload queues local files and folders to be copied into the server folder remoteDir.
func (a *App) Upload(serverID, remoteDir string, locals []string, policy string) (UploadResult, error) {
	cl, err := a.ftp(serverID)
	if err != nil {
		return UploadResult{}, err
	}
	if err := checkRemote(remoteDir); err != nil {
		return UploadResult{}, err
	}
	res := UploadResult{Paths: locals}
	exists := func(n string) bool { _, err := cl.Lstat(path.Join(remoteDir, n)); return err == nil }
	for _, l := range locals {
		if exists(filepath.Base(l)) {
			res.Conflicts = append(res.Conflicts, filepath.Base(l))
		}
	}
	if policy == "ask" && len(res.Conflicts) > 0 {
		return res, nil
	}
	for _, l := range locals {
		name := filepath.Base(l)
		if exists(name) {
			switch policy {
			case "skip":
				continue
			case "keep":
				name = unique(name, exists)
			}
		}
		local, dest := l, path.Join(remoteDir, name)
		a.enqueue(serverID, name, true, func(ctx context.Context, x *xfer) error { return a.uploadTree(ctx, x, cl, local, dest) })
		res.Started++
	}
	return res, nil
}

func (a *App) uploadTree(ctx context.Context, x *xfer, cl *sftp.Client, local, dest string) error {
	fi, err := os.Stat(local)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		x.total.Store(fi.Size())
		return a.putFile(ctx, x, cl, local, dest, fi.Mode().Perm())
	}
	var total int64
	filepath.WalkDir(local, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() {
			if i, e := d.Info(); e == nil {
				total += i.Size()
			}
		}
		return nil
	})
	x.total.Store(total)
	return filepath.WalkDir(local, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(local, p)
		target := path.Join(dest, filepath.ToSlash(rel))
		switch {
		case d.IsDir():
			if err := cl.MkdirAll(target); err != nil {
				return err
			}
		case d.Type().IsRegular():
			i, err := d.Info()
			if err != nil {
				return err
			}
			return a.putFile(ctx, x, cl, p, target, i.Mode().Perm())
		} // links and devices are not followed
		return nil
	})
}

func (a *App) putFile(ctx context.Context, x *xfer, cl *sftp.Client, local, dest string, mode os.FileMode) error {
	src, err := os.Open(local)
	if err != nil {
		return err
	}
	defer src.Close()
	tmp := dest + ".sshdeck-part"
	dst, err := cl.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return friendly(err, dest)
	}
	err = a.copyCtx(ctx, x, dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cl.Remove(tmp)
		return err
	}
	cl.Chmod(tmp, mode)
	if err := replace(cl, tmp, dest); err != nil {
		cl.Remove(tmp)
		return err
	}
	return nil
}

// ---- download ----

// Download queues server files and folders to be copied into the local folder dir.
func (a *App) Download(serverID string, remotes []string, dir, policy string) (UploadResult, error) {
	cl, err := a.ftp(serverID)
	if err != nil {
		return UploadResult{}, err
	}
	res := UploadResult{Dest: dir}
	exists := func(n string) bool { _, err := os.Lstat(filepath.Join(dir, n)); return err == nil }
	for _, r := range remotes {
		if err := checkRemote(r); err != nil {
			return res, err
		}
		if n := path.Base(r); !safeName(n) {
			return res, fmt.Errorf("%q cannot be downloaded", r)
		} else if exists(n) {
			res.Conflicts = append(res.Conflicts, n)
		}
	}
	if policy == "ask" && len(res.Conflicts) > 0 {
		return res, nil
	}
	for _, r := range remotes {
		name := path.Base(r)
		if exists(name) {
			switch policy {
			case "skip":
				continue
			case "keep":
				name = unique(name, exists)
			}
		}
		a.queueDownload(serverID, cl, r, filepath.Join(dir, name), name)
		res.Started++
	}
	return res, nil
}

func (a *App) queueDownload(serverID string, cl *sftp.Client, remote, local, name string) {
	a.enqueue(serverID, name, false, func(ctx context.Context, x *xfer) error { return a.downloadTree(ctx, x, cl, remote, local) })
}

func (a *App) downloadTree(ctx context.Context, x *xfer, cl *sftp.Client, remote, local string) error {
	fi, err := cl.Stat(remote)
	if err != nil {
		return friendly(err, remote)
	}
	if !fi.IsDir() {
		x.total.Store(fi.Size())
		return a.getFile(ctx, x, cl, remote, local)
	}
	var total int64
	for w := cl.Walk(remote); w.Step(); {
		if w.Err() == nil && w.Stat() != nil && w.Stat().Mode().IsRegular() {
			total += w.Stat().Size()
		}
	}
	x.total.Store(total)
	for w := cl.Walk(remote); w.Step(); {
		if err := ctx.Err(); err != nil {
			return err
		}
		if w.Err() != nil {
			return w.Err()
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(w.Path(), remote), "/")
		parts := strings.Split(rel, "/")
		ok := true
		for _, p := range parts {
			if rel != "" && !safeName(p) {
				ok = false
			}
		}
		if !ok {
			continue // a name that would climb out of the chosen folder
		}
		target := filepath.Join(local, filepath.FromSlash(rel))
		switch st := w.Stat(); {
		case st.IsDir():
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case st.Mode().IsRegular():
			if err := a.getFile(ctx, x, cl, w.Path(), target); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *App) getFile(ctx context.Context, x *xfer, cl *sftp.Client, remote, local string) error {
	src, err := cl.Open(remote)
	if err != nil {
		return friendly(err, remote)
	}
	defer src.Close()
	tmp := local + ".sshdeck-part"
	dst, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	err = a.copyCtx(ctx, x, dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, local)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// ---- native dialogs ----

// PickUpload asks for files (or a folder) on this machine, then uploads them to remoteDir.
func (a *App) PickUpload(serverID, remoteDir string, folder bool) (UploadResult, error) {
	if _, err := a.ftp(serverID); err != nil {
		return UploadResult{}, err
	}
	var picks []string
	var err error
	if folder {
		var d string
		d, err = runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Upload a folder"})
		if d != "" {
			picks = []string{d}
		}
	} else {
		picks, err = runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{Title: "Upload files"})
	}
	if err != nil {
		return UploadResult{}, err
	}
	if len(picks) == 0 {
		return UploadResult{Cancelled: true}, nil
	}
	return a.Upload(serverID, remoteDir, picks, "ask")
}

// PickDownload asks where to put remotes, then downloads them. One plain file gets a Save dialog; anything
// else asks for a folder.
func (a *App) PickDownload(serverID string, remotes []string) (UploadResult, error) {
	cl, err := a.ftp(serverID)
	if err != nil {
		return UploadResult{}, err
	}
	if len(remotes) == 0 {
		return UploadResult{Cancelled: true}, nil
	}
	if len(remotes) == 1 {
		if fi, err := cl.Stat(remotes[0]); err == nil && !fi.IsDir() {
			if !safeName(path.Base(remotes[0])) {
				return UploadResult{}, errors.New("that file cannot be downloaded")
			}
			to, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "Save file", DefaultFilename: path.Base(remotes[0])})
			if err != nil {
				return UploadResult{}, err
			}
			if to == "" {
				return UploadResult{Cancelled: true}, nil
			}
			a.queueDownload(serverID, cl, remotes[0], to, filepath.Base(to)) // the dialog already asked about replacing
			return UploadResult{Started: 1}, nil
		}
	}
	d, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Download into folder"})
	if err != nil {
		return UploadResult{}, err
	}
	if d == "" {
		return UploadResult{Cancelled: true}, nil
	}
	return a.Download(serverID, remotes, d, "ask")
}
