package main

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const searchCap = 500

// SearchHit is a file that matched, and for a content search the line that did.
type SearchHit struct {
	Path string `json:"path"`
	Line int    `json:"line,omitempty"`
	Text string `json:"text,omitempty"`
}

// The folder and the term reach the script as $1 and $2, never as part of its text, so neither can be shell syntax.
const (
	searchByName    = `find "$1" -iname "*$2*" 2>/dev/null | head -n 500`
	searchByContent = `grep -rIn -F -s -m 20 -- "$2" "$1" 2>/dev/null | head -n 500`
)

var grepLine = regexp.MustCompile(`^(.*?):(\d+):(.*)$`)

// SearchFiles looks under dir on a server, or inside a container, by file name (case-insensitive, `*` and `?` work)
// or by text in files. Binary files are skipped. At most searchCap hits come back.
// shortcut: needs find and grep in the container or on the server, and does not follow a symlinked dir.
func (a *App) SearchFiles(target, dir, query string, byContent bool) ([]SearchHit, error) {
	query = strings.TrimSpace(query)
	switch {
	case query == "":
		return nil, errors.New("type something to search for")
	case dir == "":
		return nil, errors.New("open a folder first")
	case len(query) > 200 || strings.ContainsRune(query, 0):
		return nil, errors.New("that search is not valid")
	}
	sid, ctr := splitTarget(target)
	cl, err := a.live(sid)
	if err != nil {
		return nil, err
	}
	script := searchByName
	if byContent {
		script = searchByContent
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	var out strings.Builder
	if ctr == "" {
		err = cl.Exec(ctx, "sh -c "+shq(script)+" sh "+shq(dir)+" "+shq(query), nil, &out)
	} else {
		d := dockerFS{ex: cl, name: ctr}
		err = cl.Exec(ctx, d.cmd(false, script, dir, query), nil, &out)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "permission denied") {
			out.Reset()
			d.sudo = true
			err = cl.Exec(ctx, d.cmd(false, script, dir, query), nil, &out)
		}
	}
	if ctx.Err() != nil {
		return nil, errors.New("the search took too long; try a smaller folder")
	}
	// find and grep exit non-zero when nothing matched or a folder was unreadable: that is not a failure.
	if err != nil && out.Len() == 0 && !strings.Contains(err.Error(), "exit status") && !strings.Contains(err.Error(), "exited with status") {
		return nil, err
	}
	return parseHits(out.String(), byContent), nil
}

func parseHits(out string, content bool) []SearchHit {
	hits := []SearchHit{}
	for _, l := range strings.Split(out, "\n") {
		if l == "" || len(hits) >= searchCap {
			continue
		}
		if !content {
			hits = append(hits, SearchHit{Path: l})
		} else if m := grepLine.FindStringSubmatch(l); m != nil {
			n, _ := strconv.Atoi(m[2])
			text := strings.TrimSpace(m[3])
			if len(text) > 300 {
				text = text[:300]
			}
			hits = append(hits, SearchHit{Path: m[1], Line: n, Text: text})
		}
	}
	return hits
}
