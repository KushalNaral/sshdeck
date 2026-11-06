package main

// The file operations behind the Files tab and the editor, for three places a file can be: the server itself
// (SFTP), a container reached through the host's /proc (SFTP again, rooted at the container's filesystem), and a
// container reached with docker exec (for logins that cannot read /proc). A target names one of them: a server id,
// or "serverID@container".

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

type rfile interface {
	io.ReadCloser
	Stat() (os.FileInfo, error)
}

type fsys interface {
	Getwd() (string, error)
	RealPath(p string) (string, error)
	ReadDir(p string) ([]os.FileInfo, error)
	Stat(p string) (os.FileInfo, error)
	Lstat(p string) (os.FileInfo, error)
	Open(p string) (rfile, error)
	Create(p string) (io.WriteCloser, error)    // create or truncate
	CreateNew(p string) (io.WriteCloser, error) // fails when the file exists
	Remove(p string) error
	RemoveAll(p string) error
	Rename(from, to string) error
	PosixRename(from, to string) error
	Mkdir(p string) error
	MkdirAll(p string) error
	Chmod(p string, m os.FileMode) error
	// InPlace says writes must go to the file itself. Writing a neighbour and renaming it over the original (what
	// is done on the server) would give the file the owner and mode of whoever runs the copy, which inside a
	// container is usually the wrong user.
	InPlace() bool
}

// splitTarget turns "srv@web" into the server id and container; a plain id has no container.
func splitTarget(t string) (server, container string) {
	server, container, _ = strings.Cut(t, "@")
	return
}

// histPath is the name a file has in history: container files are told apart from the server's own.
func histPath(container, p string) string {
	if container == "" {
		return p
	}
	return "docker:" + container + ":" + p
}

// ---- the server: SFTP ----

type sftpFS struct{ c *sftp.Client }

func (s sftpFS) Getwd() (string, error)                  { return s.c.Getwd() }
func (s sftpFS) RealPath(p string) (string, error)       { return s.c.RealPath(p) }
func (s sftpFS) ReadDir(p string) ([]os.FileInfo, error) { return s.c.ReadDir(p) }
func (s sftpFS) Stat(p string) (os.FileInfo, error)      { return s.c.Stat(p) }
func (s sftpFS) Lstat(p string) (os.FileInfo, error)     { return s.c.Lstat(p) }
func (s sftpFS) Open(p string) (rfile, error)            { return s.c.Open(p) }
func (s sftpFS) Remove(p string) error                   { return s.c.Remove(p) }
func (s sftpFS) RemoveAll(p string) error                { return s.c.RemoveAll(p) }
func (s sftpFS) Rename(a, b string) error                { return s.c.Rename(a, b) }
func (s sftpFS) PosixRename(a, b string) error           { return s.c.PosixRename(a, b) }
func (s sftpFS) Mkdir(p string) error                    { return s.c.Mkdir(p) }
func (s sftpFS) MkdirAll(p string) error                 { return s.c.MkdirAll(p) }
func (s sftpFS) Chmod(p string, m os.FileMode) error     { return s.c.Chmod(p, m) }
func (s sftpFS) InPlace() bool                           { return false }
func (s sftpFS) Create(p string) (io.WriteCloser, error) {
	return s.c.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
}
func (s sftpFS) CreateNew(p string) (io.WriteCloser, error) {
	return s.c.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
}

// ---- a container through the host's /proc ----

// procFS runs another fsys with every path moved under /proc/<pid>/root, which is the container's filesystem as
// the host sees it. It needs a login allowed to read that (in practice root). Paths are resolved lexically, so
// ".." cannot climb out of the container's root.
type procFS struct {
	in   fsys
	root string
}

func (f procFS) abs(p string) string { return path.Join(f.root, path.Clean("/"+p)) }

func (f procFS) Getwd() (string, error) { return "/", nil }
func (f procFS) RealPath(p string) (string, error) {
	c := path.Clean("/" + p)
	if _, err := f.in.Stat(f.abs(c)); err != nil {
		return "", err
	}
	return c, nil
}
func (f procFS) ReadDir(p string) ([]os.FileInfo, error) { return f.in.ReadDir(f.abs(p)) }
func (f procFS) Stat(p string) (os.FileInfo, error)      { return f.in.Stat(f.abs(p)) }
func (f procFS) Lstat(p string) (os.FileInfo, error)     { return f.in.Lstat(f.abs(p)) }
func (f procFS) Open(p string) (rfile, error)            { return f.in.Open(f.abs(p)) }
func (f procFS) Create(p string) (io.WriteCloser, error) { return f.in.Create(f.abs(p)) }
func (f procFS) CreateNew(p string) (io.WriteCloser, error) {
	return f.in.CreateNew(f.abs(p))
}
func (f procFS) Remove(p string) error    { return f.in.Remove(f.abs(p)) }
func (f procFS) RemoveAll(p string) error { return f.in.RemoveAll(f.abs(p)) }
func (f procFS) Rename(a, b string) error { return f.in.Rename(f.abs(a), f.abs(b)) }
func (f procFS) PosixRename(a, b string) error {
	return f.in.PosixRename(f.abs(a), f.abs(b))
}
func (f procFS) Mkdir(p string) error                { return f.in.Mkdir(f.abs(p)) }
func (f procFS) MkdirAll(p string) error             { return f.in.MkdirAll(f.abs(p)) }
func (f procFS) Chmod(p string, m os.FileMode) error { return f.in.Chmod(f.abs(p), m) }
func (f procFS) InPlace() bool                       { return true }

// ---- a container through docker exec ----

type execer interface {
	Exec(ctx context.Context, cmd string, stdin io.Reader, stdout io.Writer) error
}

type dockerFS struct {
	ex   execer
	name string
	sudo bool
}

// shq quotes s for a POSIX shell.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// cmd builds "docker exec NAME sh -c SCRIPT sh ARGS": the script only ever sees file names as $1, $2, so a name
// cannot become shell syntax in the container; shq protects the host shell that receives the whole line.
func (d dockerFS) cmd(stdin bool, script string, args ...string) string {
	c := "docker exec "
	if d.sudo {
		c = "sudo -n docker exec "
	}
	if stdin {
		c += "-i "
	}
	c += d.name + " sh -c " + shq(script) + " sh"
	for _, a := range args {
		c += " " + shq(a)
	}
	return c
}

func (d dockerFS) run(script string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var out strings.Builder
	err := d.ex.Exec(ctx, d.cmd(false, script, args...), nil, &out)
	return out.String(), mapErr(err)
}

func mapErr(err error) error {
	if err == nil {
		return nil
	}
	m := err.Error()
	switch {
	case strings.Contains(m, "No such file"), strings.Contains(m, "nonexistent"), strings.Contains(m, "not found"):
		return fmt.Errorf("%w: %s", os.ErrNotExist, m)
	case strings.Contains(m, "Permission denied"):
		return fmt.Errorf("%w: %s", os.ErrPermission, m)
	case strings.Contains(m, "File exists"), strings.Contains(m, "cannot overwrite"), strings.Contains(m, "exists"):
		return fmt.Errorf("%w: %s", os.ErrExist, m)
	}
	return err
}

// finfo is a file as stat reports it.
type finfo struct {
	name  string
	size  int64
	mode  os.FileMode
	mtime time.Time
}

func (f finfo) Name() string       { return f.name }
func (f finfo) Size() int64        { return f.size }
func (f finfo) Mode() os.FileMode  { return f.mode }
func (f finfo) ModTime() time.Time { return f.mtime }
func (f finfo) IsDir() bool        { return f.mode.IsDir() }
func (f finfo) Sys() any           { return nil }

// statFmt prints the raw mode in hex, size, modification time and name, one line per file.
const statFmt = `%f|%s|%Y|%n`

func parseStat(line string) (os.FileInfo, error) {
	f := strings.SplitN(line, "|", 4)
	if len(f) != 4 {
		return nil, fmt.Errorf("unexpected stat output %q", line)
	}
	raw, err := strconv.ParseUint(f[0], 16, 32)
	if err != nil {
		return nil, err
	}
	size, _ := strconv.ParseInt(f[1], 10, 64)
	sec, _ := strconv.ParseInt(f[2], 10, 64)
	m := os.FileMode(raw & 0o777)
	switch raw & 0o170000 {
	case 0o040000:
		m |= os.ModeDir
	case 0o120000:
		m |= os.ModeSymlink
	}
	return finfo{name: path.Base(f[3]), size: size, mode: m, mtime: time.Unix(sec, 0)}, nil
}

func (d dockerFS) stat(p string, follow bool) (os.FileInfo, error) {
	flag := ""
	if follow {
		flag = "-L "
	}
	out, err := d.run(`stat `+flag+`-c '`+statFmt+`' -- "$1"`, p)
	if err != nil {
		return nil, err
	}
	return parseStat(strings.TrimRight(out, "\n"))
}

func (d dockerFS) Stat(p string) (os.FileInfo, error)  { return d.stat(p, true) }
func (d dockerFS) Lstat(p string) (os.FileInfo, error) { return d.stat(p, false) }
func (d dockerFS) Getwd() (string, error) {
	out, err := d.run(`pwd`)
	return strings.TrimSpace(out), err
}
func (d dockerFS) RealPath(p string) (string, error) {
	out, err := d.run(`cd -- "$1" && pwd -P`, p)
	return strings.TrimSpace(out), err
}

func (d dockerFS) ReadDir(p string) ([]os.FileInfo, error) {
	out, err := d.run(`cd -- "$1" || exit 2; find . -mindepth 1 -maxdepth 1 -exec stat -c '`+statFmt+`' {} +`, p)
	if err != nil {
		return nil, err
	}
	var infos []os.FileInfo
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if l == "" {
			continue
		}
		fi, err := parseStat(l)
		if err != nil {
			continue
		}
		infos = append(infos, fi)
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
	return infos, nil
}

func (d dockerFS) Remove(p string) error {
	_, err := d.run(`rm -- "$1" 2>/dev/null || rmdir -- "$1"`, p)
	return err
}
func (d dockerFS) RemoveAll(p string) error { _, err := d.run(`rm -rf -- "$1"`, p); return err }
func (d dockerFS) Rename(a, b string) error { _, err := d.run(`mv -- "$1" "$2"`, a, b); return err }
func (d dockerFS) PosixRename(a, b string) error {
	return d.Rename(a, b)
}
func (d dockerFS) Mkdir(p string) error    { _, err := d.run(`mkdir -- "$1"`, p); return err }
func (d dockerFS) MkdirAll(p string) error { _, err := d.run(`mkdir -p -- "$1"`, p); return err }
func (d dockerFS) Chmod(p string, m os.FileMode) error {
	_, err := d.run(`chmod `+strconv.FormatUint(uint64(m.Perm()), 8)+` -- "$1"`, p)
	return err
}
func (d dockerFS) InPlace() bool { return true }

// dreader streams a file out of the container.
type dreader struct {
	r      *io.PipeReader
	cancel context.CancelFunc
	fi     os.FileInfo
}

func (r *dreader) Read(p []byte) (int, error) { return r.r.Read(p) }
func (r *dreader) Stat() (os.FileInfo, error) { return r.fi, nil }
func (r *dreader) Close() error               { r.cancel(); return r.r.Close() }

func (d dockerFS) Open(p string) (rfile, error) {
	fi, err := d.Stat(p)
	if err != nil {
		return nil, err
	}
	if fi.IsDir() {
		return nil, fmt.Errorf("%s is a folder", p)
	}
	pr, pw := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { pw.CloseWithError(mapErr(d.ex.Exec(ctx, d.cmd(false, `cat -- "$1"`, p), nil, pw))) }()
	return &dreader{r: pr, cancel: cancel, fi: fi}, nil
}

// dwriter feeds a file in the container; Close waits for the command and returns its error.
type dwriter struct {
	w    *io.PipeWriter
	done chan error
}

func (w *dwriter) Write(p []byte) (int, error) { return w.w.Write(p) }
func (w *dwriter) Close() error {
	w.w.Close()
	return <-w.done
}

func (d dockerFS) write(script, p string) (io.WriteCloser, error) {
	pr, pw := io.Pipe()
	w := &dwriter{w: pw, done: make(chan error, 1)}
	go func() {
		err := mapErr(d.ex.Exec(context.Background(), d.cmd(true, script, p), pr, nil))
		pr.CloseWithError(err) // a failed start unblocks the writer
		w.done <- err
	}()
	return w, nil
}

// Create writes in place with cat, so the file keeps its owner, mode and inode.
func (d dockerFS) Create(p string) (io.WriteCloser, error) { return d.write(`cat > "$1"`, p) }
func (d dockerFS) CreateNew(p string) (io.WriteCloser, error) {
	return d.write(`set -C; cat > "$1"`, p)
}

// ---- walking ----

// walk calls fn for root and everything below it, parents first. A folder's entries come in name order.
func walk(f fsys, root string, fn func(p string, fi os.FileInfo) error) error {
	fi, err := f.Stat(root)
	if err != nil {
		return err
	}
	if err := fn(root, fi); err != nil {
		return err
	}
	if !fi.IsDir() {
		return nil
	}
	entries, err := f.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Mode()&fs.ModeSymlink != 0 {
			continue // links are not followed, so a loop cannot trap a copy
		}
		if !safeName(e.Name()) {
			continue
		}
		if err := walk(f, path.Join(root, e.Name()), fn); err != nil {
			return err
		}
	}
	return nil
}

// ---- finding the right one ----

var errNoContainer = errors.New("that container is not running")

// containerFS picks how to reach a container's files: through /proc when the login can read it, else docker exec.
func (a *App) containerFS(host *sftp.Client, cl interface{ execer }, name string) (fsys, error) {
	if !containerRe.MatchString(name) {
		return nil, fmt.Errorf("%q is not a valid container name", name)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sudo := false
	var out strings.Builder
	inspect := func(sudo bool) error {
		out.Reset()
		c := "docker inspect -f '{{.State.Running}} {{.State.Pid}}' " + name
		if sudo {
			c = "sudo -n " + c
		}
		return cl.Exec(ctx, c, nil, &out)
	}
	if err := inspect(false); err != nil {
		if err2 := inspect(true); err2 != nil {
			if strings.Contains(err.Error(), "No such") {
				return nil, errNoContainer
			}
			return nil, fmt.Errorf("docker is not available to this login: %w", err)
		}
		sudo = true
	}
	f := strings.Fields(out.String())
	if len(f) != 2 || f[0] != "true" {
		return nil, errNoContainer
	}
	if pid, _ := strconv.Atoi(f[1]); pid > 0 {
		pf := procFS{in: sftpFS{host}, root: "/proc/" + f[1] + "/root"}
		if _, err := pf.ReadDir("/"); err == nil {
			return pf, nil
		}
	}
	return dockerFS{ex: cl, name: name, sudo: sudo}, nil
}
