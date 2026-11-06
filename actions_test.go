package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

// scripted answers commands in the shape runCode expects: the output, then the exit mark with the code.
type scripted struct {
	mu   sync.Mutex
	cmds []string
	ans  func(cmd string) (string, int)
}

func (s *scripted) exec(_ context.Context, cmd string, _ io.Reader, out io.Writer) error {
	s.mu.Lock()
	s.cmds = append(s.cmds, cmd)
	s.mu.Unlock()
	o, code := s.ans(cmd)
	fmt.Fprintf(out, "%s%s%d", o, exitMark, code)
	return nil
}

func (s *scripted) seen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.cmds, "\n")
}

func rigActions(t *testing.T, ans func(string) (string, int)) (*App, *scripted, *trail, string) {
	t.Helper()
	a, ids := rigRun(t, 1, nil)
	s := &scripted{ans: ans}
	a.liveClientFor(ids[0]).exec = s.exec
	tr := &trail{}
	a.audit = tr.add
	return a, s, tr, ids[0]
}

func okAnswer(string) (string, int) { return "", 0 }

func TestServiceActionBuildsTheExactCommand(t *testing.T) {
	a, s, tr, id := rigActions(t, okAnswer)
	for _, c := range []struct{ action, want string }{
		{"restart", "systemctl restart -- 'nginx.service'"},
		{"start", "systemctl start -- 'nginx.service'"},
		{"stop", "systemctl stop -- 'nginx.service'"},
		{"status", "systemctl status --no-pager -n 30 -- 'nginx.service'"},
	} {
		if _, err := a.ServiceAction(id, "nginx.service", c.action); err != nil {
			t.Fatalf("%s: %v", c.action, err)
		}
		if !strings.Contains(s.seen(), c.want) {
			t.Errorf("%s: want %q in\n%s", c.action, c.want, s.seen())
		}
	}
	if tr.find("service.restart") == nil || tr.find("service.stop") == nil {
		t.Errorf("changes must be audited:\n%s", tr.all())
	}
	if tr.find("service.status") != nil {
		t.Error("status only reads and is not audited")
	}
}

func TestContainerActionBuildsTheExactCommand(t *testing.T) {
	a, s, tr, id := rigActions(t, okAnswer)
	if _, err := a.ContainerAction(id, "web-1", "restart"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.seen(), "docker restart -- 'web-1'") {
		t.Errorf("got\n%s", s.seen())
	}
	if tr.find("container.restart") == nil {
		t.Error("expected an audit entry")
	}
	if _, err := a.ContainerAction(id, "web-1", "status"); err == nil {
		t.Error("status is not a container action")
	}
}

func TestActionsRefuseHostileNamesAndUnknownActions(t *testing.T) {
	a, s, _, id := rigActions(t, okAnswer)
	for _, bad := range []string{"", "-rf", "a b", "a;reboot", "$(id)", "a\nb", "`x`", "../x", "a'b"} {
		if _, err := a.ServiceAction(id, bad, "restart"); err == nil {
			t.Errorf("unit %q should be refused", bad)
		}
		if _, err := a.ContainerAction(id, bad, "restart"); err == nil {
			t.Errorf("container %q should be refused", bad)
		}
	}
	if _, err := a.ServiceAction(id, "nginx", "mask"); err == nil {
		t.Error("only status, start, stop and restart are allowed")
	}
	if s.seen() != "" {
		t.Errorf("nothing may reach the server for a refused request:\n%s", s.seen())
	}
}

func TestActionRetriesWithSudoOnPermissionDenied(t *testing.T) {
	a, s, tr, id := rigActions(t, func(cmd string) (string, int) {
		if strings.Contains(cmd, "sudo -n") {
			return "", 0
		}
		return "Failed to restart nginx.service: Interactive authentication required.\n", 1
	})
	r, err := a.ServiceAction(id, "nginx", "restart")
	if err != nil || r.Exit != 0 || !r.Sudo {
		t.Fatalf("%+v %v", r, err)
	}
	if !strings.Contains(s.seen(), "sudo -n systemctl restart -- 'nginx'") {
		t.Errorf("got\n%s", s.seen())
	}
	if e := tr.find("service.restart"); e == nil || !strings.Contains(e.Detail, "sudo") {
		t.Errorf("the audit should say sudo was used: %+v", e)
	}
}

func TestActionNeverTypesASudoPassword(t *testing.T) {
	a, _, _, id := rigActions(t, func(cmd string) (string, int) {
		if strings.Contains(cmd, "sudo -n") {
			return "sudo: a password is required\n", 1
		}
		return "permission denied while trying to connect to the Docker daemon socket\n", 1
	})
	_, err := a.ContainerAction(id, "web-1", "restart")
	if err == nil || !strings.Contains(err.Error(), "without a password") {
		t.Fatalf("want the sudo hint, got %v", err)
	}
}

func TestActionFailureKeepsTheOutput(t *testing.T) {
	a, _, _, id := rigActions(t, func(string) (string, int) { return "Unit nope.service not found.\n", 5 })
	r, err := a.ServiceAction(id, "nope", "restart")
	if err != nil {
		t.Fatal(err)
	}
	if r.Exit != 5 || !strings.Contains(r.Output, "not found") {
		t.Errorf("%+v", r)
	}
}

func TestActionNeedsAConnection(t *testing.T) {
	a, _, _, _ := rigActions(t, okAnswer)
	if _, err := a.ServiceAction("ghost", "nginx", "restart"); err == nil {
		t.Error("an unconnected server must not be dialled")
	}
}
