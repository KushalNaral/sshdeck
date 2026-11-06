package importer

import (
	"strings"
	"testing"
)

const sshSample = `
# my servers
Host *
    User deploy
    Port 2200

Host web1 web2
    HostName 10.0.0.5
    IdentityFile ~/.ssh/id_work

Host db
    HostName=db.internal
    User = postgres
    Port 22
    ProxyJump ops@bastion:2222

Host bastion
    HostName "203.0.113.9"

Host *.example.com
    User nobody

Match host legacy
    User old

Host lost
    ProxyJump ghost
`

func byName(rows []Row) map[string]Row {
	m := map[string]Row{}
	for _, r := range rows {
		m[r.Name] = r
	}
	return m
}

func TestSSHConfigHosts(t *testing.T) {
	rows := ParseSSHConfig(strings.NewReader(sshSample), "me", "/home/me")
	m := byName(rows)
	for _, n := range []string{"web1", "web2", "db", "bastion", "lost"} {
		if _, ok := m[n]; !ok {
			t.Errorf("%s missing from %v", n, rows)
		}
	}
	if len(rows) != 5 {
		t.Errorf("wildcard hosts must be skipped, got %d rows", len(rows))
	}
	w := m["web1"]
	if w.Server.Host != "10.0.0.5" || w.Server.User != "deploy" || w.Server.Port != 2200 {
		t.Errorf("web1 should take defaults from Host *: %+v", w.Server)
	}
	if w.Server.Auth != "key" || w.Server.KeyPath != "/home/me/.ssh/id_work" {
		t.Errorf("identity file with ~ expanded: %+v", w.Server)
	}
	if m["web2"].Server.Host != "10.0.0.5" {
		t.Error("one block can name several hosts")
	}
}

func TestSSHConfigQuotesEqualsAndDefaults(t *testing.T) {
	m := byName(ParseSSHConfig(strings.NewReader(sshSample), "me", "/home/me"))
	if d := m["db"]; d.Server.Host != "db.internal" || d.Server.User != "postgres" || d.Server.Port != 22 {
		t.Errorf("= syntax and own values beat Host *: %+v", d.Server)
	}
	if b := m["bastion"]; b.Server.Host != "203.0.113.9" || b.Server.KeyPath != "" || b.Server.Auth != "key" {
		t.Errorf("quotes dropped, no identity file means the agent: %+v", b.Server)
	}
}

func TestSSHConfigProxyJump(t *testing.T) {
	m := byName(ParseSSHConfig(strings.NewReader(sshSample), "me", "/home/me"))
	if m["db"].JumpName != "bastion" || m["db"].Err != "" {
		t.Errorf("user and port in a ProxyJump are dropped, the alias is kept: %+v", m["db"])
	}
	if l := m["lost"]; l.Err == "" || !strings.Contains(l.Err, "ghost") {
		t.Errorf("a jump host that is not in the file is an error: %+v", l)
	}
}

func TestSSHConfigBadPort(t *testing.T) {
	rows := ParseSSHConfig(strings.NewReader("Host a\n  Port abc\n"), "me", "/h")
	if len(rows) != 1 || rows[0].Err == "" {
		t.Fatalf("%+v", rows)
	}
}
