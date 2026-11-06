package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KushalNaral/sshdeck/internal/sshx"
)

// ActionResult is what a service or container action printed. Sudo says the second, privileged try was the one
// that ran.
type ActionResult struct {
	Output string `json:"output"`
	Exit   int    `json:"exit"`
	Sudo   bool   `json:"sudo,omitempty"`
}

// serviceRe allows what a systemd unit name can hold; it cannot start with "-", so it is never read as an option.
var serviceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9@._:\\-]*$`)

const actionTimeout = 60 * time.Second

// ServiceAction runs systemctl status, start, stop or restart for one unit on a connected server.
func (a *App) ServiceAction(serverID, unit, action string) (ActionResult, error) {
	if !serviceRe.MatchString(unit) {
		return ActionResult{}, errors.New("that is not a valid service name")
	}
	var cmd string
	switch action {
	case "status":
		cmd = "systemctl status --no-pager -n 30 -- " + shq(unit)
	case "start", "stop", "restart":
		cmd = "systemctl " + action + " -- " + shq(unit)
	default:
		return ActionResult{}, errors.New("a service can be started, stopped, restarted or shown")
	}
	return a.act(serverID, "service."+action, unit, cmd, action != "status")
}

// ContainerAction starts, stops or restarts one Docker container on a connected server.
func (a *App) ContainerAction(serverID, name, action string) (ActionResult, error) {
	if !containerRe.MatchString(name) {
		return ActionResult{}, errors.New("that is not a valid container name")
	}
	switch action {
	case "start", "stop", "restart":
	default:
		return ActionResult{}, errors.New("a container can be started, stopped or restarted")
	}
	return a.act(serverID, "container."+action, name, "docker "+action+" -- "+shq(name), true)
}

// act runs cmd and, when it fails for lack of rights, once more with "sudo -n". It never answers a password
// prompt: with no passwordless sudo the user is told to use a terminal.
func (a *App) act(serverID, event, subject, cmd string, audit bool) (ActionResult, error) {
	cl, err := a.live(serverID)
	if err != nil {
		return ActionResult{}, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, actionTimeout)
	defer cancel()
	out, code, err := execCode(ctx, cl, cmd)
	if err != nil {
		return ActionResult{}, err
	}
	res := ActionResult{Output: out, Exit: code}
	if code != 0 && lacksRights(out) {
		out2, code2, err := execCode(ctx, cl, "sudo -n "+cmd)
		if err != nil {
			return ActionResult{}, err
		}
		low := strings.ToLower(out2)
		if code2 != 0 && (strings.Contains(low, "password is required") || strings.Contains(low, "a terminal is required")) {
			return ActionResult{}, errors.New("This needs sudo without a password. Open a terminal and run it there.")
		}
		res = ActionResult{Output: out2, Exit: code2, Sudo: true}
	}
	if audit {
		detail := fmt.Sprintf("%s exit=%d", subject, res.Exit)
		if res.Sudo {
			detail += " sudo"
		}
		a.audit(event, a.serverName(serverID), detail)
	}
	return res, nil
}

// lacksRights recognises the ways systemctl, docker and the shell say "you may not".
func lacksRights(out string) bool {
	o := strings.ToLower(out)
	for _, s := range []string{"permission denied", "interactive authentication required", "access denied", "must be root", "not permitted"} {
		if strings.Contains(o, s) {
			return true
		}
	}
	return false
}

// execCode runs cmd in a subshell with stderr joined to stdout and returns the output and the real exit code.
// Exec only reports a non-zero exit as an error, so the code is printed after the output and read back.
func execCode(ctx context.Context, cl sshx.Client, cmd string) (string, int, error) {
	var out strings.Builder
	err := cl.Exec(ctx, "(\n"+cmd+"\n) 2>&1; printf '"+strings.ReplaceAll(exitMark, "\x01", "\\001")+"%d' $?", nil, &out)
	text := out.String()
	k := strings.LastIndex(text, exitMark)
	if k < 0 {
		if err == nil {
			err = errors.New("the server gave no answer")
		}
		return "", -1, err
	}
	code, e := strconv.Atoi(strings.TrimSpace(text[k+len(exitMark):]))
	if e != nil {
		code = -1
	}
	return strings.TrimSpace(text[:k]), code, nil
}
