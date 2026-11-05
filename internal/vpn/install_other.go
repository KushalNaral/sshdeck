//go:build !windows

package vpn

// selectConsoleComponent does nothing off Windows: the installer is only ever run there.
func selectConsoleComponent() (func(), error) { return func() {}, nil }
