package vpn

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
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
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
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

// Stop ends the client. pkexec runs the client as root, so a plain signal from this user is refused (EPERM);
// then the client's processes are ended through pkexec itself (polkit may ask for the password again).
// shortcut: needs a real VPN to confirm end to end; unit-tested only with plain processes.
func (p *execProc) Stop() error {
	select {
	case <-p.done:
		return nil
	default:
	}
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGTERM); err != nil {
		p.killAsRoot()
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
	}
	if err := syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		p.killAsRoot()
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("the VPN client (pid %d) did not stop; end it with: sudo kill %d", p.cmd.Process.Pid, p.cmd.Process.Pid)
	}
}

// killAsRoot sends SIGTERM to pkexec's children, which are the root-owned client.
func (p *execProc) killAsRoot() {
	out, err := exec.Command("pgrep", "-P", strconv.Itoa(p.cmd.Process.Pid)).Output()
	if err != nil {
		return
	}
	pids := strings.Fields(string(out))
	if len(pids) == 0 {
		return
	}
	_ = exec.Command("pkexec", append([]string{"kill", "-TERM"}, pids...)...).Run()
}
