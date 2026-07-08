package ioutils

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("bidirectional pipe did not stop after context cancellation")
	}
}
