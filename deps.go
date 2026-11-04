package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
)

// Dep is a program sshdeck needs on this machine.
type Dep struct {
	Name        string `json:"name"`
	Purpose     string `json:"purpose"`
	Installed   bool   `json:"installed"`
	Installable bool   `json:"installable"` // false for things the app cannot install itself
}

var deps = []Dep{
	{Name: "openfortivpn", Purpose: "FortiClient VPN (HQ old, Quartz Reports)", Installable: true},
	{Name: "openconnect", Purpose: "GlobalProtect VPN (HQ, Portal)", Installable: true},
	{Name: "pkexec", Purpose: "Asks for your admin password to start a VPN (part of polkit)"},
}

// CheckDependencies reports which required programs are on this machine.
func (a *App) CheckDependencies() []Dep {
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
