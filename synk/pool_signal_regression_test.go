//go:build pprof && unix

package synk

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestPprofDoesNotInterceptInterruptRegression(t *testing.T) {
	const helperEnv = "GOUTILS_PPROF_INTERRUPT_HELPER"
	if os.Getenv(helperEnv) == "1" {
		// Allow package-init goroutines to install any signal handlers before signaling.
		time.Sleep(100 * time.Millisecond)
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			os.Exit(43)
		}
		time.Sleep(time.Second)
		os.Exit(42)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPprofDoesNotInterceptInterruptRegression$")
	cmd.Env = append(os.Environ(), helperEnv+"=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("SIGINT helper timed out: %s", output)
	}
	exit, ok := errors.AsType[*exec.ExitError](err)
	if !ok {
		t.Fatalf("expected SIGINT termination, got %v: %s", err, output)
	}
	status, ok := exit.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() || status.Signal() != syscall.SIGINT {
		t.Fatalf("SIGINT was intercepted: status=%v: %s", exit.ProcessState, output)
	}
}
