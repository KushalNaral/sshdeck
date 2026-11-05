package vpn

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strings"
)

// ExecStarter launches real processes with os/exec.
type ExecStarter struct{}

type execProc struct {
	cmd   *exec.Cmd
	lines chan string
	done  chan struct{} // closed when the process has exited
	err   error
}

func (ExecStarter) Start(ctx context.Context, argv []string, stdin string) (Proc, error) {
	cmd := exec.Command(argv[0], argv[1:]...) // not CommandContext: Stop does a graceful stop, ctx ending is the manager's job
	cmd.Stdin = strings.NewReader(stdin)
	pr, pw := io.Pipe()
	cmd.Stdout, cmd.Stderr = pw, pw
	prepare(cmd)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &execProc{cmd: cmd, lines: make(chan string, 64), done: make(chan struct{})}
	go func() {
		p.err = cmd.Wait()
		pw.Close()
		close(p.done)
	}()
	go func() {
		defer close(p.lines)
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			p.lines <- sc.Text()
		}
	}()
	return p, nil
}

func (p *execProc) Lines() <-chan string { return p.lines }
func (p *execProc) Wait() error          { <-p.done; return p.err }
