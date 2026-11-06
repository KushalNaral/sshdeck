package history

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// The shared copy: versions also written under ~/.sshdeck/history on the server, so everyone who edits through
// sshdeck sees the same trail. One folder per file (named from its path), and per version a small .json
// (who, when, kind) and a .gz (the text). The folder is private to the login (0700) like the file it describes.

const remoteDir = ".sshdeck/history"

func pathKey(p string) string { x := sha256.Sum256([]byte(p)); return hex.EncodeToString(x[:8]) }

func remoteRoot(cl *sftp.Client) (string, error) {
	home, err := cl.RealPath(".")
	if err != nil {
		return "", err
	}
	return path.Join(home, remoteDir), nil
}

func mkdirPrivate(cl *sftp.Client, dir string) error {
	if err := cl.MkdirAll(dir); err != nil {
		return err
	}
	cl.Chmod(path.Dir(dir), 0o700) // ~/.sshdeck, when this created it; a failure there is harmless
	return cl.Chmod(dir, 0o700)
}

func putFile(cl *sftp.Client, p string, b []byte) error {
	f, err := cl.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	_, werr := f.Write(b)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = cl.Chmod(p, 0o600)
	}
	return werr
}

func getFile(cl *sftp.Client, p string, limit int64) ([]byte, error) {
	f, err := cl.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// WriteRemote stores one version on the server. It is called after the local copy is saved; a failure is the
// caller's to report and never undoes anything.
func WriteRemote(cl *sftp.Client, v Version, content []byte) error {
	root, err := remoteRoot(cl)
	if err != nil {
		return err
	}
	dir := path.Join(root, pathKey(v.Path))
	if err := mkdirPrivate(cl, path.Join(root)); err != nil {
		return err
	}
	if err := mkdirPrivate(cl, dir); err != nil {
		return err
	}
	if _, err := cl.Stat(path.Join(dir, "PATH")); err != nil {
		if err := putFile(cl, path.Join(dir, "PATH"), []byte(v.Path+"\n")); err != nil {
			return err
		}
	}
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	zw.Write(content)
	zw.Close()
	meta, _ := json.Marshal(v)
	if err := putFile(cl, path.Join(dir, v.ID+".gz"), z.Bytes()); err != nil {
		return err
	}
	return putFile(cl, path.Join(dir, v.ID+".json"), meta) // the .json goes last: a version is listed only when complete
}

// ListRemote returns the versions of one file kept on the server, newest first.
func ListRemote(cl *sftp.Client, server, p string) ([]Version, error) {
	root, err := remoteRoot(cl)
	if err != nil {
		return nil, err
	}
	return readDir(cl, server, path.Join(root, pathKey(p)), 0)
}

// RecentRemote returns the newest versions of every file on the server's shared trail.
func RecentRemote(cl *sftp.Client, server string, limit int) ([]Version, error) {
	root, err := remoteRoot(cl)
	if err != nil {
		return nil, err
	}
	dirs, err := cl.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var all []Version
	for _, d := range dirs {
		if d.IsDir() {
			vs, _ := readDir(cl, server, path.Join(root, d.Name()), limit)
			all = append(all, vs...)
		}
	}
	all = byTime(all)
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// readDir reads the newest limit (all when 0) version records in one file's folder.
func readDir(cl *sftp.Client, server, dir string, limit int) ([]Version, error) {
	infos, err := cl.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, i := range infos {
		if strings.HasSuffix(i.Name(), ".json") {
			names = append(names, i.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names))) // names start with the time, so this is newest first
	if limit > 0 && len(names) > limit {
		names = names[:limit]
	}
	var out []Version
	for _, n := range names {
		b, err := getFile(cl, path.Join(dir, n), 64<<10)
		var v Version
		if err != nil || json.Unmarshal(b, &v) != nil || v.ID == "" || v.Path == "" {
			continue // damaged or not ours: ignore
		}
		v.Server, v.Remote = server, true
		out = append(out, v)
	}
	return out, nil
}

// ContentRemote fetches the text of a version kept on the server.
func ContentRemote(cl *sftp.Client, v Version) ([]byte, error) {
	root, err := remoteRoot(cl)
	if err != nil {
		return nil, err
	}
	b, err := getFile(cl, path.Join(root, pathKey(v.Path), v.ID+".gz"), 8<<20)
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(zr, 4<<20))
}

// PruneRemote applies the same count and age rules to one file's folder on the server.
func PruneRemote(cl *sftp.Client, p string, keep, days int, now time.Time) error {
	root, err := remoteRoot(cl)
	if err != nil {
		return err
	}
	dir := path.Join(root, pathKey(p))
	infos, err := cl.ReadDir(dir)
	if err != nil {
		return nil
	}
	var ids []string
	for _, i := range infos {
		if strings.HasSuffix(i.Name(), ".json") {
			ids = append(ids, strings.TrimSuffix(i.Name(), ".json"))
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ids)))
	cut := now.AddDate(0, 0, -days).UnixNano()
	for i, id := range ids {
		var ns int64
		fmt.Sscanf(id, "%d-", &ns)
		if i == 0 || (i < keep && ns >= cut) {
			continue
		}
		cl.Remove(path.Join(dir, id+".json"))
		cl.Remove(path.Join(dir, id+".gz"))
	}
	return nil
}

// DeleteRemote removes the shared trail of one file, or all of it when p is empty.
func DeleteRemote(cl *sftp.Client, p string) error {
	root, err := remoteRoot(cl)
	if err != nil {
		return err
	}
	target := root
	if p != "" {
		target = path.Join(root, pathKey(p))
	}
	if _, err := cl.Stat(target); err != nil {
		return nil
	}
	return cl.RemoveAll(target)
}
