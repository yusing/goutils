package ioutils

import (
	"context"
	"net"
	"testing"
	"time"

	expect "github.com/yusing/goutils/testing"
)

func TestBidirectionalPipeStopsOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	clientConn, backendConn := net.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- NewBidirectionalPipe(ctx, clientConn, backendConn).Start()
	}()

	cancel()

	select {
	case err := <-done:
		expect.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("bidirectional pipe did not stop after context cancellation")
	}
}
