package vpn

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// DefaultStarter runs VPN clients in the elevated helper.
func DefaultStarter() Starter { return &HelperStarter{Launch: launchElevated} }

// launchElevated starts this program again as "sshdeck --vpn-helper ADDR TOKEN" with the UAC prompt (the "runas" verb).
func launchElevated(addr, token string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	args, _ := windows.UTF16PtrFromString(fmt.Sprintf("--vpn-helper %s %s", addr, token))
	return windows.ShellExecute(0, verb, file, args, nil, windows.SW_HIDE)
}
