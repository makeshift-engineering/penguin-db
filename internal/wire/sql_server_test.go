package wire

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type mockListener struct {
	acceptCount int32
	closed      chan struct{}
}

func newMockListener() *mockListener {
	return &mockListener{
		closed: make(chan struct{}),
	}
}

func (m *mockListener) Accept() (net.Conn, error) {
	count := atomic.AddInt32(&m.acceptCount, 1)
	if count <= 3 {
		// Return temporary error
		return nil, errors.New("temporary accept error")
	}
	<-m.closed
	return nil, net.ErrClosed
}

func (m *mockListener) Close() error {
	select {
	case <-m.closed:
	default:
		close(m.closed)
	}
	return nil
}

func (m *mockListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 5432}
}

func TestServer_Serve_BackoffAndShutdown(t *testing.T) {
	s := NewServer("127.0.0.1:0", nil, nil)
	l := newMockListener()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Serve(ctx, l)
	}()

	// Wait for a few accept attempts with backoff
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("expected nil error on shutdown, got %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Serve did not exit within timeout")
	}

	count := atomic.LoadInt32(&l.acceptCount)
	if count < 3 {
		t.Errorf("expected at least 3 accept attempts, got %d", count)
	}
}
