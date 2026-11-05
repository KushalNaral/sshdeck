//go:build !windows

package sshx

import (
	"fmt"
	"io"
	"net"
	"os"
)

// dialAgent connects to the running ssh-agent through SSH_AUTH_SOCK.
func dialAgent(server string) (io.ReadWriteCloser, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, fmt.Errorf("%s uses key login but sets no key file, and no ssh-agent is running (SSH_AUTH_SOCK is empty): choose a key file in Admin or start ssh-agent", server)
	}
	c, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("cannot reach the ssh-agent at %s: %w", sock, err)
	}
	return c, nil
}
