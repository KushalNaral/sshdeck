//go:build !windows

package vpn

// DefaultStarter runs VPN clients as child processes (through pkexec, see command).
func DefaultStarter() Starter { return ExecStarter{} }
