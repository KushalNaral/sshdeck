package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"slices"
	"time"

	"github.com/KushalNaral/sshdeck/internal/vpn"
)

// hostOS is the running system; a variable so tests can behave as Windows.
var hostOS = runtime.GOOS

// installOnWindows installs openconnect through the elevated helper, which downloads the one pinned installer,
// checks it and runs it. Nothing else can be installed this way.
func (a *App) installOnWindows(names []string) error {
	if len(names) != 1 || names[0] != "openconnect" {
		return fmt.Errorf("%v is not something sshdeck can install here", names)
	}
	a.audit("deps.install", "openconnect", "")
	ctx, cancel := context.WithTimeout(a.ctx, 15*time.Minute)
	defer cancel()
	if err := vpn.RunInstall(ctx, a.starter, func(l string) { a.emit("deps:log", l) }); err != nil {
		return fmt.Errorf("%w (if you cancelled the administrator prompt, try again)", err)
	}
	return nil
}

// Dep is a program sshdeck needs on this machine.
type Dep struct {
	Name        string `json:"name"`
	Purpose     string `json:"purpose"`
	Installed   bool   `json:"installed"`
	Installable bool   `json:"installable"` // false for things the app cannot install itself
}

var deps = []Dep{
	{Name: "openfortivpn", Purpose: "FortiClient VPN (HQ old, Quartz Reports)", Installable: true},
	{Name: "gpclient", Purpose: "GlobalProtect VPN, preferred when installed (HQ, Portal, Branch)"},
	{Name: "openconnect", Purpose: "GlobalProtect VPN, used when gpclient is missing", Installable: true},
	{Name: "pkexec", Purpose: "Asks for your admin password to start a VPN (part of polkit)"},
}

// CheckDependencies reports which required programs are on this machine.
func (a *App) CheckDependencies() []Dep {
	if hostOS == "windows" { // the Linux clients below do not exist here; openconnect covers both VPN types
		_, err := a.lookPath("openconnect")
		return []Dep{{Name: "openconnect", Purpose: "VPN client for GlobalProtect and FortiClient, with the Wintun driver. Installing it asks for administrator approval.", Installed: err == nil, Installable: true}}
	}
	out := slices.Clone(deps)
	for i := range out {
		_, err := a.lookPath(out[i].Name)
		out[i].Installed = err == nil
	}
	return out
}

// installArgv returns the package-manager command for pkgs, or an error when the distro is not recognised.
func installArgv(lookPath func(string) (string, error), pkgs []string) ([]string, error) {
	for _, pm := range []struct {
		bin  string
		args []string
	}{
		{"apt-get", []string{"install", "-y"}},
		{"dnf", []string{"install", "-y"}},
		{"pacman", []string{"-S", "--noconfirm"}},
	} {
		if _, err := lookPath(pm.bin); err == nil {
			argv := append([]string{"pkexec", pm.bin}, pm.args...)
			return append(argv, pkgs...), nil
		}
	}
	return nil, errors.New("no supported package manager found (apt, dnf or pacman); install the programs by hand")
}

// InstallDependencies installs the named programs through the system package manager, asking for the admin
// password with pkexec. Output streams as "deps:log" events. Only names from the fixed list are accepted.
func (a *App) InstallDependencies(names []string) error {
	if hostOS == "windows" {
		return a.installOnWindows(names)
	}
	var pkgs []string
	for _, n := range names {
		i := slices.IndexFunc(deps, func(d Dep) bool { return d.Name == n && d.Installable })
		if i < 0 {
			return fmt.Errorf("%q is not something sshdeck can install", n)
		}
		pkgs = append(pkgs, n)
	}
	if len(pkgs) == 0 {
		return errors.New("nothing to install")
	}
	if _, err := a.lookPath("pkexec"); err != nil {
		return errors.New("pkexec is missing, so the app cannot ask for your admin password; install polkit first")
	}
	argv, err := installArgv(a.lookPath, pkgs)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	a.emit("deps:log", "$ "+shellJoin(argv[1:]))
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			a.emit("deps:log", sc.Text())
		}
		close(done)
	}()
	err = cmd.Wait()
	pw.Close()
	<-done
	if err != nil {
		return fmt.Errorf("the install did not finish (%w); if you cancelled the password prompt, try again", err)
	}
	return nil
}

func shellJoin(a []string) string {
	s := ""
	for i, x := range a {
		if i > 0 {
			s += " "
		}
		s += x
	}
	return s
}
