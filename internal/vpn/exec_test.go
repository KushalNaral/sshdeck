package vpn

import (
	"context"
	"testing"
	"time"
)

func TestExecStarterStreamsLinesAndStops(t *testing.T) {
	p, err := ExecStarter{}.Start(context.Background(), []string{"sh", "-c", "read x; echo got:$x; sleep 30"}, "hello\n")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case l := <-p.Lines():
		if l != "got:hello" {
			t.Fatalf("got %q", l)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no output")
	}
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not return")
	}
}
