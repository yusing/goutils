package workerpool

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConcurrentWaitRegression(t *testing.T) {
	for range 10 {
		ctx, cancel := context.WithCancel(t.Context())
		p := New(ctx, WithN(32))
		release := make(chan struct{})
		for range 32 {
			p.Go(func(context.Context, int) { <-release })
		}
		var waiting sync.WaitGroup
		start := make(chan struct{})
		for range 64 {
			waiting.Go(func() { <-start; p.Wait() })
		}
		done := make(chan struct{})
		go func() { waiting.Wait(); close(done) }()
		close(start)
		close(release)
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			t.Fatal("concurrent Wait calls did not finish after all workers completed")
		}
		cancel()
	}
}

func TestWaitCancellationRegression(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	p := New(ctx, WithN(2))
	release := make(chan struct{})
	defer close(release)
	p.Go(func(context.Context, int) { <-release })
	var waiting sync.WaitGroup
	for range 16 {
		waiting.Go(func() { p.Wait() })
	}
	done := make(chan struct{})
	go func() { waiting.Wait(); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Wait did not respond to cancellation while workers remained active")
	}
}

func TestInvalidWorkerCountRegression(t *testing.T) {
	for _, n := range []int{0, -1} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			defer func() {
				recovered := recover()
				if recovered == nil {
					t.Fatal("New accepted a nonpositive worker count")
				}
				message := fmt.Sprint(recovered)
				if !strings.Contains(message, "workerpool") || !(strings.Contains(message, "positive") || strings.Contains(message, "> 0")) {
					t.Fatalf("panic does not clearly explain invalid worker count: %q", message)
				}
			}()
			New(t.Context(), WithN(n))
		})
	}
}
