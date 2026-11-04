package sshx

import (
	"context"
	"strings"
	"testing"
	"time"
)

func readOutput(t *testing.T, s Session, want string) {
	t.Helper()
	var got strings.Builder
	deadline := time.After(2 * time.Second)
	for !strings.Contains(got.String(), want) {
		select {
		case b, ok := <-s.Output():
			if !ok {
				t.Fatalf("output closed before %q (got %q)", want, got.String())
			}
			got.Write(b)
		case <-deadline:
			t.Fatalf("timed out waiting for %q (got %q)", want, got.String())
		}
	}
}

func TestManyShellsAndCommandsShareOneConnection(t *testing.T) {
	ts, srv := startServer(t)
	c, err := Dial(context.Background(), srv, goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i := 0; i < 5; i++ {
		if out, err := c.Run(context.Background(), "echo hi"); err != nil || out != "hi\n" {
			t.Fatalf("run %d: %q %v", i, out, err)
		}
	}
	a, err := c.Shell(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Shell(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.Write([]byte("one"))
	b.Write([]byte("two"))
	readOutput(t, a, "ONE")
	readOutput(t, b, "TWO")
	if n := ts.accepted.Load(); n != 1 {
		t.Fatalf("server saw %d connections, want 1", n)
	}
}

func TestClosingOneShellLeavesTheConnectionAndTheOtherShellAlone(t *testing.T) {
	_, srv := startServer(t)
	c, err := Dial(context.Background(), srv, goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := c.Shell(context.Background())
	b, _ := c.Shell(context.Background())
	a.Close()
	select {
	case <-c.Done():
		t.Fatal("closing a shell ended the connection")
	default:
	}
	b.Write([]byte("still here"))
	readOutput(t, b, "STILL HERE")
	if out, err := c.Run(context.Background(), "echo hi"); err != nil || out != "hi\n" {
		t.Fatalf("run after closing a shell: %q %v", out, err)
	}
	c.Close()
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("Done did not fire after Close")
	}
	if _, ok := <-b.Output(); ok {
		// drain: the stream must end once the connection is gone
		for range b.Output() {
		}
	}
}

func TestDoneFiresWhenTheServerDropsTheConnection(t *testing.T) {
	ts, srv := startServer(t)
	c, err := Dial(context.Background(), srv, goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	sh, _ := c.Shell(context.Background())
	ts.dropAll()
	select {
	case <-c.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("a dropped connection was not noticed")
	}
	select {
	case _, ok := <-sh.Output():
		for ok {
			_, ok = <-sh.Output()
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the shell's output did not end")
	}
}

func TestACancelledCommandDoesNotKillTheSharedConnection(t *testing.T) {
	_, srv := startServer(t)
	c, err := Dial(context.Background(), srv, goodPassword)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Run(ctx, "echo hi"); err == nil {
		t.Fatal("a cancelled command succeeded")
	}
	if out, err := c.Run(context.Background(), "echo hi"); err != nil || out != "hi\n" {
		t.Fatalf("connection unusable after a cancelled command: %q %v", out, err)
	}
}
