package sshx

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type fakePeer struct {
	hang bool
	fail bool
	n    atomic.Int32
}

func (f *fakePeer) SendRequest(string, bool, []byte) (bool, []byte, error) {
	f.n.Add(1)
	if f.hang {
		select {} // a half-dead TCP link never answers
	}
	if f.fail {
		return false, nil, errors.New("broken pipe")
	}
	return true, nil, nil
}

func watch(p requester, done chan struct{}) <-chan struct{} {
	dead := make(chan struct{})
	go keepAlive(p, 5*time.Millisecond, 20*time.Millisecond, done, func() { close(dead) })
	return dead
}

func TestKeepAliveReportsAHungPeer(t *testing.T) {
	select {
	case <-watch(&fakePeer{hang: true}, make(chan struct{})):
	case <-time.After(2 * time.Second):
		t.Fatal("hung peer not detected")
	}
}

func TestKeepAliveReportsAFailedRequest(t *testing.T) {
	select {
	case <-watch(&fakePeer{fail: true}, make(chan struct{})):
	case <-time.After(2 * time.Second):
		t.Fatal("failed request not detected")
	}
}

func TestKeepAliveLeavesAHealthyPeerAloneAndStops(t *testing.T) {
	p := &fakePeer{}
	done := make(chan struct{})
	dead := watch(p, done)
	time.Sleep(100 * time.Millisecond)
	close(done)
	select {
	case <-dead:
		t.Fatal("healthy peer reported dead")
	default:
	}
	if p.n.Load() < 3 {
		t.Fatalf("expected several probes, got %d", p.n.Load())
	}
}
