package task

import (
	"testing"
	"time"
)

func TestTestTaskFinishWithoutRootChildren(t *testing.T) {
	initRoot()
	t.Cleanup(testCleanup)
	scope := GetTestTask(t)
	scope.Finish(nil)
	if scope.Context().Err() == nil {
		t.Fatal("test task not canceled")
	}
	if GetTestTask(t) != scope {
		t.Fatal("test task not cached")
	}
	scope.FinishAndWait(nil)
}

func TestTestTaskFinishAndWaitChildren(t *testing.T) {
	initRoot()
	t.Cleanup(testCleanup)
	scope := GetTestTask(t)
	child := scope.Subtask("worker", true)
	done := make(chan struct{})
	go func() { <-child.Context().Done(); child.Finish(nil); close(done) }()
	scope.FinishAndWait(nil)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("test task did not stop child")
	}
}
