package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"testing"
)

func TestConvertError(t *testing.T) {
	t.Parallel()

	otherErr := errors.New("other")
	tests := []struct {
		name string
		err  error
		want error
	}{
		{name: "nil", err: nil},
		{name: "http server closed", err: http.ErrServerClosed},
		{name: "context canceled", err: context.Canceled},
		{name: "net closed", err: net.ErrClosed},
		{name: "wrapped http server closed", err: fmt.Errorf("wrapped: %w", http.ErrServerClosed)},
		{name: "wrapped context canceled", err: fmt.Errorf("wrapped: %w", context.Canceled)},
		{name: "wrapped net closed", err: fmt.Errorf("wrapped: %w", net.ErrClosed)},
		{name: "joined closed error", err: errors.Join(otherErr, net.ErrClosed)},
		{name: "other error", err: otherErr, want: otherErr},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := convertError(tt.err); got != tt.want {
				t.Fatalf("convertError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func BenchmarkConvertError(b *testing.B) {
	otherErr := errors.New("other")
	benchmarks := []struct {
		name string
		err  error
	}{
		{name: "nil", err: nil},
		{name: "http_server_closed", err: http.ErrServerClosed},
		{name: "context_canceled", err: context.Canceled},
		{name: "net_closed", err: net.ErrClosed},
		{name: "wrapped_http_server_closed", err: fmt.Errorf("wrapped: %w", http.ErrServerClosed)},
		{name: "joined_net_closed", err: errors.Join(otherErr, net.ErrClosed)},
		{name: "other_error", err: otherErr},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for b.Loop() {
				_ = convertError(bm.err)
			}
		})
	}
}
