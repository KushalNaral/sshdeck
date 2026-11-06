package importer

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/KushalNaral/sshdeck/internal/store"
)

type sshBlock struct {
	aliases []string
	opts    map[string]string // lower-case keyword -> first value
	line    int
}

// ParseSSHConfig reads the Host blocks of an OpenSSH config into server rows. Wildcard hosts are skipped, but a
// "Host *" block supplies defaults. user and home fill in what the file leaves out and expand "~".
// shortcut: Include and Match are skipped, and only the first IdentityFile and the first ProxyJump hop are used.
func ParseSSHConfig(r io.Reader, user, home string) []Row {
	var blocks []*sshBlock
	var cur *sshBlock
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val := splitDirective(line)
		switch key {
		case "host":
			cur = &sshBlock{aliases: strings.Fields(val), opts: map[string]string{}, line: n}
			blocks = append(blocks, cur)
		case "match":
			cur = nil // its options must not leak into the previous host
		default:
			if cur != nil {
				if _, seen := cur.opts[key]; !seen { // ssh uses the first value it finds
					cur.opts[key] = val
				}
			}
		}
	}
	defaults := map[string]string{}
	for _, b := range blocks {
		for _, a := range b.aliases {
			if a == "*" {
				for k, v := range b.opts {
					if _, ok := defaults[k]; !ok {
						defaults[k] = v
					}
				}
			}
		}
	}
	known := map[string]bool{}
	for _, b := range blocks {
		for _, a := range b.aliases {
			known[a] = true
		}
	}
	var rows []Row
	seen := map[string]bool{}
	for _, b := range blocks {
		for _, alias := range b.aliases {
			if strings.ContainsAny(alias, "*?!") || seen[alias] {
				continue
			}
			seen[alias] = true
			rows = append(rows, sshRow(alias, b, defaults, known, user, home))
		}
	}
	return rows
}

// splitDirective splits "Keyword value", "Keyword=value" and "Keyword = value".
func splitDirective(line string) (string, string) {
	i := strings.IndexAny(line, " \t=")
	if i < 0 {
		return strings.ToLower(line), ""
	}
	val := strings.TrimLeft(line[i:], " \t=")
	return strings.ToLower(line[:i]), strings.Trim(strings.TrimSpace(val), `"`)
}

func sshRow(alias string, b *sshBlock, defaults map[string]string, known map[string]bool, user, home string) Row {
	get := func(k string) string {
		if v, ok := b.opts[k]; ok {
			return v
		}
		return defaults[k]
	}
	row := Row{Line: b.line, Type: "server", Name: alias}
	sv := store.Server{Name: alias, Host: get("hostname"), User: get("user"), Port: 22, Auth: "key"}
	if sv.Host == "" {
		sv.Host = alias
	}
	if sv.User == "" {
		sv.User = user
	}
	if p := get("port"); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil {
			row.Err = fmt.Sprintf("port %q is not a number", p)
		}
		sv.Port = n
	}
	if k := get("identityfile"); k != "" {
		sv.KeyPath = expandHome(k, home)
	}
	if pj := get("proxyjump"); pj != "" && !strings.EqualFold(pj, "none") {
		hop := strings.Split(pj, ",")[0]
		if i := strings.LastIndex(hop, "@"); i >= 0 {
			hop = hop[i+1:]
		}
		if h, _, err := net.SplitHostPort(hop); err == nil {
			hop = h
		}
		if known[hop] && !strings.ContainsAny(hop, "*?") {
			row.JumpName = hop
		} else if row.Err == "" {
			row.Err = fmt.Sprintf("jump host %q is not a Host in this file", hop)
		}
	}
	row.Server = sv
	return row
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		return filepath.Join(home, rest)
	}
	return p
}
