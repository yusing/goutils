package eventqueue

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yusing/goutils/task"
	expect "github.com/yusing/goutils/testing"
)

func TestStartReceivesEventsWhileFlushIsBlocked(t *testing.T) {
	eventCh := make(chan int)
	errCh := make(chan error)
	firstFlushStarted := make(chan struct{})
	releaseFirstFlush := make(chan struct{})
	release := sync.OnceFunc(func() {
		close(releaseFirstFlush)
	})
	flushed := make(chan []int, 2)
	var flushing atomic.Bool
	var concurrentFlush atomic.Bool

	queueTask := task.GetTestTask(t).Subtask("event_queue", true)
	t.Cleanup(func() {
		release()
		queueTask.FinishAndWait(nil)
	})

	queue := New(queueTask, Options[int]{
		FlushInterval: time.Millisecond,
		OnFlush: func(events []int) {
			if !flushing.CompareAndSwap(false, true) {
				concurrentFlush.Store(true)
			}
			defer flushing.Store(false)

			flushed <- append([]int(nil), events...)
			if len(events) == 1 && events[0] == 1 {
				close(firstFlushStarted)
				<-releaseFirstFlush
			}
		},
	})
	queue.Start(eventCh, errCh)

	select {
	case eventCh <- 1:
	case <-time.After(time.Second):
		t.Fatal("timed out sending first event")
	}
	select {
	case <-firstFlushStarted:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first flush")
	}
	expect.Equal(t, receiveFlushedEvents(t, flushed), []int{1})

	select {
	case eventCh <- 2:
	case <-time.After(time.Second):
		t.Fatal("timed out sending event during blocked flush")
	}
	select {
	case events := <-flushed:
		t.Fatalf("flush ran concurrently: %v", events)
	case <-time.After(20 * time.Millisecond):
	}
	expect.False(t, concurrentFlush.Load())

	release()
	expect.Equal(t, receiveFlushedEvents(t, flushed), []int{2})
}

func receiveFlushedEvents(t *testing.T, flushed <-chan []int) []int {
	t.Helper()

	select {
	case events := <-flushed:
		return events
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for flushed events")
		return nil
	}
}
