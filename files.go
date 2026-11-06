package main

// Remote files: browse, edit and rename over SFTP on the connection's shared SSH login. Transfers live in
// transfers.go. Paths on the server are always slash paths; paths on this machine use the OS separator.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"

	"github.com/KushalNaral/sshdeck/internal/history"
	"github.com/KushalNaral/sshdeck/internal/sshx"
)

// maxEdit is the biggest file the editor opens; bigger files are for download, not for a text widget.
const maxEdit = 2 << 20

type FileEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	Dir   bool   `json:"dir"`
	Link  bool   `json:"link"`
	Size  int64  `json:"size"`
	Mode  string `json:"mode"` // like "rwxr-xr-x"
	Mtime int64  `json:"mtime"`
}

type Listing struct {
	Path    string      `json:"path"`
	Entries []FileEntry `json:"entries"`
}

type TextFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Version string `json:"version"` // pass back to WriteText to notice that someone else changed the file
	Mode    string `json:"mode"`
	// NoHistory is set when history is off or this kind of file (keys, shadow) is never recorded.
	NoHistory bool `json:"noHistory,omitempty"`
}

// live returns the SSH client of a connected server.
func (a *App) live(serverID string) (sshx.Client, error) {
	if err := a.guard(); err != nil {
		return nil, err
	}
	a.mu.Lock()
	c := a.conns[serverID]
	var cl sshx.Client
	if c != nil && c.client != nil {
		select {
		case <-c.client.Done():
		default:
			cl = c.client
		}
	}
	a.mu.Unlock()
	if cl == nil {
		return nil, errors.New("not connected")
	}
	return cl, nil
}

// ftp returns the SFTP client of a live connection (the server's own files).
func (a *App) ftp(serverID string) (*sftp.Client, error) {
	cl, err := a.live(serverID)
	if err != nil {
		return nil, err
	}
	return cl.SFTP()
}

type cfsEntry struct {
	host *sftp.Client
	fs   fsys
	at   time.Time
}

// fs returns the file operations for a target: a server id for the server itself, or "serverID@container".
// How a container is reached is worked out once and kept for a short while.
func (a *App) fs(target string) (fsys, error) {
	sid, ctr := splitTarget(target)
	cl, err := a.live(sid)
	if err != nil {
		return nil, err
	}
	host, err := cl.SFTP()
	if err != nil {
		return nil, err
	}
	if ctr == "" {
		return sftpFS{host}, nil
	}
	a.mu.Lock()
	e, ok := a.cfs[target]
	a.mu.Unlock()
	if ok && e.host == host && time.Since(e.at) < 30*time.Second {
		return e.fs, nil
	}
	f, err := a.containerFS(host, cl, ctr)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	if a.cfs == nil {
		a.cfs = map[string]cfsEntry{}
	}
	a.cfs[target] = cfsEntry{host: host, fs: f, at: time.Now()}
	a.mu.Unlock()
	return f, nil
}

// Containers lists the running containers of a connected server, for the Files tab's location picker.
func (a *App) Containers(serverID string) ([]string, error) {
	cl, err := a.live(serverID)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	var out strings.Builder
	if err := cl.Exec(ctx, "docker ps --format '{{.Names}}'", nil, &out); err != nil {
		out.Reset()
		if err2 := cl.Exec(ctx, "sudo -n docker ps --format '{{.Names}}'", nil, &out); err2 != nil {
			return nil, errors.New("docker is not available to this login")
		}
	}
	var names []string
	for _, n := range strings.Fields(out.String()) {
		if containerRe.MatchString(n) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	return names, nil
}

func checkRemote(p string) error {
	if p == "" || strings.ContainsRune(p, 0) {
		return errors.New("that is not a valid path")
	}
	return nil
}

// ListDir lists a folder. An empty dir means the login's home folder. The reply carries the resolved path.
func (a *App) ListDir(serverID, dir string) (Listing, error) {
	cl, err := a.fs(serverID)
	if err != nil {
		return Listing{}, err
	}
	if dir == "" {
		if dir, err = cl.Getwd(); err != nil {
			dir = "/"
		}
	}
	if err := checkRemote(dir); err != nil {
		return Listing{}, err
	}
	real, err := cl.RealPath(dir)
	if err != nil {
		return Listing{}, friendly(err, dir)
	}
	infos, err := cl.ReadDir(real)
	if err != nil {
		return Listing{}, friendly(err, real)
	}
	out := make([]FileEntry, 0, len(infos))
	for _, fi := range infos {
		e := FileEntry{Name: fi.Name(), Path: path.Join(real, fi.Name()), Dir: fi.IsDir(), Size: fi.Size(), Mode: perm(fi.Mode()), Mtime: fi.ModTime().Unix()}
		if fi.Mode()&os.ModeSymlink != 0 {
			e.Link = true
			if t, err := cl.Stat(e.Path); err == nil { // follow the link to learn whether it opens like a folder
				e.Dir, e.Size = t.IsDir(), t.Size()
			}
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return Listing{Path: real, Entries: out}, nil
}

func perm(m os.FileMode) string {
	const rwx = "rwxrwxrwx"
	b := []byte("---------")
	for i := 0; i < 9; i++ {
		if m&(1<<uint(8-i)) != 0 {
			b[i] = rwx[i]
		}
	}
	return string(b)
}

func friendly(err error, p string) error {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("%s does not exist", p)
	case errors.Is(err, os.ErrPermission):
		return fmt.Errorf("you do not have permission for %s", p)
	}
	return err
}

func versionOf(fi os.FileInfo) string {
	return strconv.FormatInt(fi.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(fi.Size(), 10)
}

// ReadText opens a file for the editor. Large files and files that are not UTF-8 text are refused.
func (a *App) ReadText(serverID, p string) (TextFile, error) {
	cl, err := a.fs(serverID)
	if err != nil {
		return TextFile{}, err
	}
	if err := checkRemote(p); err != nil {
		return TextFile{}, err
	}
	f, err := cl.Open(p)
	if err != nil {
		return TextFile{}, friendly(err, p)
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return TextFile{}, err
	}
	if fi.IsDir() {
		return TextFile{}, errors.New("that is a folder")
	}
	if fi.Size() > maxEdit {
		return TextFile{}, fmt.Errorf("this file is %s, which is too big to edit here (limit 2 MB). Download it instead", size(fi.Size()))
	}
	b, err := io.ReadAll(io.LimitReader(f, maxEdit+1))
	if err != nil {
		return TextFile{}, err
	}
	if len(b) > maxEdit || !utf8.Valid(b) || strings.ContainsRune(string(b), 0) {
		return TextFile{}, errors.New("this does not look like a text file. Download it instead")
	}
	a.audit("file.open", a.auditName(serverID), p)
	a.noteCurrent(serverID, p, b)
	return TextFile{Path: p, Content: string(b), Version: versionOf(fi), Mode: perm(fi.Mode()), NoHistory: a.hist() == nil || a.denied(p)}, nil
}

func size(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}

// ErrChanged is the message WriteText returns when the file changed on the server after it was opened.
const ErrChanged = "changed on the server since you opened it"

// WriteText saves editor content. version is what ReadText returned ("" for a new file); if the file on the
// server no longer matches it nothing is written and the error says so. The file is written beside the target
// and renamed over it, so a dropped connection never leaves a half-written file, and its permissions stay.
func (a *App) WriteText(serverID, p, content, version string) (string, error) {
	return a.writeText(serverID, p, content, version, history.Save)
}

func (a *App) writeText(serverID, p, content, version string, kind history.Kind) (string, error) {
	cl, err := a.fs(serverID)
	if err != nil {
		return "", err
	}
	if err := checkRemote(p); err != nil {
		return "", err
	}
	mode := os.FileMode(0o644)
	if fi, err := cl.Stat(p); err == nil {
		if version != "" && versionOf(fi) != version {
			return "", errors.New(ErrChanged)
		}
		mode = fi.Mode().Perm()
	} else if version != "" && errors.Is(err, os.ErrNotExist) {
		return "", errors.New("the file was removed on the server since you opened it")
	}
	a.snapshotBefore(serverID, cl, p)
	tmp := p + ".sshdeck-part"
	var f io.WriteCloser
	if cl.InPlace() { // in a container, keep the file's owner and mode
		tmp = ""
	} else if f, err = cl.Create(tmp); err != nil { // no room to create a neighbour (a read-only folder): write the file itself
		tmp = ""
	}
	if tmp == "" {
		if f, err = cl.Create(p); err != nil {
			return "", friendly(err, p)
		}
	}
	_, werr := io.WriteString(f, content)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		if tmp != "" {
			cl.Remove(tmp)
		}
		return "", werr
	}
	if tmp != "" {
		cl.Chmod(tmp, mode)
		if err := replace(cl, tmp, p); err != nil {
			cl.Remove(tmp)
			return "", err
		}
	}
	fi, err := cl.Stat(p)
	if err != nil {
		return "", err
	}
	a.audit("file.save", a.auditName(serverID), p)
	a.track(serverID, p, kind, who(), []byte(content))
	return versionOf(fi), nil
}

// snapshotBefore keeps what is on the server right now, so a save can always be undone, even the first one.
func (a *App) snapshotBefore(serverID string, cl fsys, p string) {
	if a.hist() == nil || a.denied(p) {
		return
	}
	f, err := cl.Open(p)
	if err != nil {
		return // a new file has nothing to keep
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxEdit+1))
	if err != nil || len(b) > maxEdit || !utf8.Valid(b) {
		return
	}
	a.noteCurrent(serverID, p, b)
}

// replace renames from over to, atomically when the server supports it.
func replace(cl fsys, from, to string) error {
	if err := cl.PosixRename(from, to); err == nil {
		return nil
	}
	cl.Remove(to)
	return cl.Rename(from, to)
}

func (a *App) Mkdir(serverID, p string) error {
	cl, err := a.fs(serverID)
	if err != nil {
		return err
	}
	if err := checkRemote(p); err != nil {
		return err
	}
	if err := cl.Mkdir(p); err != nil {
		return friendly(err, p)
	}
	a.audit("file.mkdir", a.auditName(serverID), p)
	return nil
}

// NewFile creates an empty file, failing if the name is taken.
func (a *App) NewFile(serverID, p string) error {
	cl, err := a.fs(serverID)
	if err != nil {
		return err
	}
	if err := checkRemote(p); err != nil {
		return err
	}
	f, err := cl.CreateNew(p)
	if err == nil {
		err = f.Close() // in a container the command's error only arrives on close
	}
	if err != nil {
		if _, serr := cl.Stat(p); serr == nil {
			return fmt.Errorf("%s already exists", path.Base(p))
		}
		return friendly(err, p)
	}
	a.audit("file.create", a.auditName(serverID), p)
	return nil
}

// Rename moves a file or folder. It never replaces something that is already at the new path.
func (a *App) Rename(serverID, from, to string) error {
	cl, err := a.fs(serverID)
	if err != nil {
		return err
	}
	if checkRemote(from) != nil || checkRemote(to) != nil {
		return errors.New("that is not a valid path")
	}
	if _, err := cl.Lstat(to); err == nil {
		return fmt.Errorf("%s already exists", path.Base(to))
	}
	if err := cl.Rename(from, to); err != nil {
		return friendly(err, from)
	}
	a.audit("file.rename", a.auditName(serverID), from+" -> "+to)
	return nil
}

// Delete removes files and folders, folders with everything inside. The UI asks first.
func (a *App) Delete(serverID string, paths []string) error {
	cl, err := a.fs(serverID)
	if err != nil {
		return err
	}
	for _, p := range paths {
		if err := checkRemote(p); err != nil {
			return err
		}
		if c := path.Clean(p); c == "/" || c == "." {
			return errors.New("refusing to delete the root folder")
		}
	}
	for _, p := range paths {
		if err := cl.RemoveAll(p); err != nil {
			return friendly(err, p)
		}
		a.audit("file.delete", a.auditName(serverID), p)
	}
	return nil
}
