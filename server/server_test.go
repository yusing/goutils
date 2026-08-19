package server

import (
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/yusing/goutils/task"
)

const shutdownWaitLimit = 2 * time.Second

func TestStartNilServerFinishesTask(t *testing.T) {
	t.Parallel()

	parent := task.GetTestTask(t).Subtask("nil-server", true)
	child := parent.Subtask("https", true)

	port, err := Start(child, (*http.Server)(nil))
	if err != nil {
		t.Fatalf("Start(nil): %v", err)
	}
	if port != 0 {
		t.Fatalf("Start(nil) port = %d, want 0", port)
	}

	started := time.Now()
	parent.FinishAndWait("test done")
	if elapsed := time.Since(started); elapsed >= shutdownWaitLimit {
		t.Fatalf("unused proto task still blocked shutdown after %s", elapsed)
	}
}

func TestStartServerHTTPOnlyDoesNotLeaveHTTPSTask(t *testing.T) {
	t.Parallel()

	parent := task.GetTestTask(t).Subtask("http-only", true)
	_, err := StartServer(parent, Options{
		Name:     "api",
		HTTPAddr: "127.0.0.1:0",
		Handler:  http.NotFoundHandler(),
	})
	if err != nil {
		t.Fatalf("StartServer: %v", err)
	}

	started := time.Now()
	parent.FinishAndWait("test done")
	if elapsed := time.Since(started); elapsed >= shutdownWaitLimit {
		t.Fatalf("HTTP-only server still blocked shutdown after %s", elapsed)
	}
}

func TestStartServerListenErrorFinishesTask(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	parent := task.GetTestTask(t).Subtask("listen-error", true)
	_, err = StartServer(parent, Options{
		Name:     "busy",
		HTTPAddr: ln.Addr().String(),
		Handler:  http.NotFoundHandler(),
	})
	if err == nil {
		t.Fatal("StartServer: want listen error, got nil")
	}

	started := time.Now()
	parent.FinishAndWait("test done")
	if elapsed := time.Since(started); elapsed >= shutdownWaitLimit {
		t.Fatalf("failed HTTP task still blocked shutdown after %s", elapsed)
	}
}
