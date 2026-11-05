package sshx

import (
	"fmt"
	"io"
	"os"
)

// agentPipe is where the Windows OpenSSH agent service listens.
const agentPipe = `\\.\pipe\openssh-ssh-agent`

// dialAgent connects to the OpenSSH agent's named pipe (or the pipe named by SSH_AUTH_SOCK). A named pipe can be
// opened like a file, which is all the agent protocol needs.
func dialAgent(server string) (io.ReadWriteCloser, error) {
	path := os.Getenv("SSH_AUTH_SOCK")
	if path == "" {
		path = agentPipe
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("%s uses key login but sets no key file, and the ssh-agent could not be reached (%s): choose a key file in Admin or start the OpenSSH Authentication Agent service: %w", server, path, err)
	}
	return f, nil
}
