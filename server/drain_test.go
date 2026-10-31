package server

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/yusing/goutils/task"
)

// startDrainTestServer starts an HTTP server on a fresh port under its own
// task, the way an entrypoint listener runs under a config runtime.
func startDrainTestServer(t *testing.T, handler http.HandlerFunc, opts ...ServerStartOption) (*task.Task, string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtime := task.GetTestTask(t).Subtask("runtime", true)
	srv := &http.Server{Handler: handler}
	if _, err := Start(runtime, srv, append([]ServerStartOption{WithListener(ln)}, opts...)...); err != nil {
		t.Fatal(err)
	}
	return runtime, ln.Addr().String()
}

type drainResult struct {
	body string
	err  error
}

func getAsync(addr string) <-chan drainResult {
	ch := make(chan drainResult, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/")
		if err != nil {
			ch <- drainResult{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		ch <- drainResult{body: string(b), err: err}
	}()
	return ch
}

// A reload stops the old runtime while requests are in flight. They must be
// allowed to finish within the drain timeout instead of being cancelled.
func TestInFlightRequestSurvivesStop(t *testing.T) {
	started := make(chan struct{})
	runtime, addr := startDrainTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-time.After(300 * time.Millisecond):
			_, _ = io.WriteString(w, "done")
		case <-r.Context().Done():
			http.Error(w, context.Cause(r.Context()).Error(), http.StatusServiceUnavailable)
		}
	}, WithDrainTimeout(2*time.Second))

	res := getAsync(addr)
	<-started
	stopStarted := time.Now()
	runtime.FinishAndWait(errors.New("config changed"))
	if elapsed := time.Since(stopStarted); elapsed > 200*time.Millisecond {
		t.Errorf("stop blocked for %s; draining must not delay a reload", elapsed)
	}

	// the port is released at once, so a new runtime can bind it
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("port not released after stop: %v", err)
	}
	_ = ln.Close()

	r := <-res
	if r.err != nil || r.body != "done" {
		t.Fatalf("in-flight request: body %q, err %v; want it to complete", r.body, r.err)
	}
}

// Requests still running when the drain timeout ends are cancelled with the
// stop cause, so connections on an old config do not live forever.
func TestDrainTimeoutCancelsRemainingRequests(t *testing.T) {
	started := make(chan struct{})
	cause := make(chan error, 1)
	runtime, addr := startDrainTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-time.After(10 * time.Second):
		case <-r.Context().Done():
			cause <- context.Cause(r.Context())
		}
	}, WithDrainTimeout(200*time.Millisecond))

	_ = getAsync(addr)
	<-started
	stopStarted := time.Now()
	runtime.FinishAndWait(errors.New("config changed"))

	select {
	case err := <-cause:
		if elapsed := time.Since(stopStarted); elapsed < 150*time.Millisecond {
			t.Errorf("request cancelled after %s, before the drain timeout", elapsed)
		}
		if err == nil || err.Error() != "config changed" {
			t.Errorf("cancel cause = %v, want the stop reason", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request still running long after the drain timeout")
	}
}

func TestStartServerPassesDrainTimeout(t *testing.T) {
	started := make(chan struct{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runtime := task.GetTestTask(t).Subtask("runtime", true)
	_, err = StartServer(runtime, Options{
		Name:         "test",
		HTTPAddr:     ln.Addr().String(),
		HTTPListener: ln,
		DrainTimeout: 2 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			close(started)
			select {
			case <-time.After(300 * time.Millisecond):
				_, _ = io.WriteString(w, "done")
			case <-r.Context().Done():
			}
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	res := getAsync(ln.Addr().String())
	<-started
	runtime.FinishAndWait(errors.New("config changed"))
	if r := <-res; r.err != nil || r.body != "done" {
		t.Fatalf("body %q, err %v; want the request to complete", r.body, r.err)
	}
}
