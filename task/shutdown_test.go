package task

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// tree shaped like production: config -> provider -> routes, each route holding
// OnCancel callbacks that undo its registrations.
func newRouteTree(tb testing.TB, routes int, onCancel func()) {
	tb.Helper()
	cfg := RootTask("config", false)
	provider := cfg.Subtask("provider.docker", false)
	for range routes {
		r := provider.Subtask("route", false)
		r.OnCancel("remove_route", onCancel)
		r.OnCancel("remove_route_from_provider", onCancel)
	}
}

func TestGracefulShutdownRunsEveryCallbackWithinBudget(t *testing.T) {
	t.Cleanup(testCleanup)

	var invoked atomic.Int64
	newRouteTree(t, 50, func() { invoked.Add(1) })

	require.NoError(t, gracefulShutdown(3*time.Second))
	require.EqualValues(t, 100, invoked.Load())
	require.Zero(t, root.children.Len())
}

// A shutdown with no budget must not claim that a healthy tree is stuck, but it
// also cannot pretend the teardown finished.
func TestGracefulShutdownWithoutBudgetFails(t *testing.T) {
	t.Cleanup(testCleanup)

	newRouteTree(t, 5, func() {})

	start := time.Now()
	require.Error(t, gracefulShutdown(0))
	require.Less(t, time.Since(start), time.Second)
}

func TestStuckCallbackIsReportedWithItsOwner(t *testing.T) {
	t.Cleanup(testCleanup)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	cfg := RootTask("config", false)
	route := cfg.Subtask("route.myapp", false)
	route.OnCancel("remove_route", func() { <-release })

	require.Error(t, gracefulShutdown(100*time.Millisecond))

	// The route task stays in the tree while its callback hangs, so the report can
	// say which route is holding shutdown up.
	var stuck stuckSubtree
	stuck.collect(root)
	require.Contains(t, stuck.callbacks, "config.route.myapp: remove_route")
	require.Contains(t, stuck.children, "config.route.myapp")
}

// Outside shutdown a stuck callback must not pin its task to the parent forever.
func TestStuckCallbackDetachesOutsideShutdown(t *testing.T) {
	t.Cleanup(testCleanup)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	parent := RootTask("parent", true)
	child := parent.Subtask("child", true)
	child.OnCancel("blocked", func() { <-release })

	child.FinishAndWait(nil) // gives up after taskTimeout
	require.Zero(t, parent.children.Len())
}

// A nested FinishAndWait must not outlast the program-wide shutdown budget,
// otherwise the root wait can never be the last one to give up.
func TestNestedWaitStaysWithinShutdownBudget(t *testing.T) {
	t.Cleanup(testCleanup)

	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	cfg := RootTask("config", true)
	cfg.OnCancel("blocked", func() { <-release })
	go func() {
		<-cfg.Context().Done()
		cfg.FinishAndWait(nil)
	}()

	start := time.Now()
	_ = gracefulShutdown(200 * time.Millisecond)
	elapsed := time.Since(start)

	require.Less(t, elapsed, taskTimeout, "nested wait used its own timeout instead of the shutdown budget")
}

func TestWaitTimeoutTracksShutdownBudget(t *testing.T) {
	require.Equal(t, taskTimeout, waitTimeout())

	shutdownDeadline.Store(time.Now().Add(50 * time.Millisecond).UnixNano())
	t.Cleanup(func() { shutdownDeadline.Store(0) })
	require.Less(t, waitTimeout(), taskTimeout)
	require.Positive(t, waitTimeout())
}
