# goutils/eventqueue

Batches events from a channel and hands them to one callback on a timer. Bursts
become a few calls instead of one call per event, flushes never overlap, and the
queue lives and dies with a [`task`](../task/README.md).

Typical uses are coalescing file-system or container events before a reload,
and batching writes to a slow sink. It does not persist, filter, deduplicate, or
acknowledge events.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/eventqueue"
```

The package is in the root module and needs Go 1.27 or later. It depends on
`task` and `errs` from the same module.

## Quick start

```go
package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/yusing/goutils/eventqueue"
	"github.com/yusing/goutils/task"
)

type change struct{ Path string }

func main() {
	eventCh := make(chan change)
	errCh := make(chan error)

	queueTask := task.RootTask("change-queue", true)
	queue := eventqueue.New(queueTask, eventqueue.Options[change]{
		FlushInterval: 200 * time.Millisecond,
		OnFlush: func(batch []change) {
			fmt.Println("flush:", batch)
		},
		OnError: func(err error) {
			fmt.Println("error:", err)
		},
	})
	queue.Start(eventCh, errCh)

	eventCh <- change{"a.txt"}
	eventCh <- change{"b.txt"}
	errCh <- errors.New("watcher hiccup")
	time.Sleep(500 * time.Millisecond)

	// Stop the queue and wait for any flush that is still running.
	queueTask.FinishAndWait(nil)
}
```

Expected output, with the two events normally in one batch:

```text
error: watcher hiccup
flush: [{a.txt} {b.txt}]
```

## How it behaves

`New(queueTask, Options)` allocates the queue and a ticker. `Start(eventCh, errCh)`
launches the one goroutine that does the work. Call `Start` once.

| Aspect | Behavior |
| --- | --- |
| Buffering | Every value received from `eventCh` is appended to an in-memory buffer. |
| Flush trigger | Every `FlushInterval` tick, if the buffer is not empty and no flush is running. |
| Flush call | The buffer is copied and `OnFlush(batch)` runs on its own goroutine. The slice belongs to the callback. Order is preserved within and across batches. |
| One at a time | Never two `OnFlush` calls at once. While one runs, new events keep being buffered, and they are flushed right after it returns, without waiting for the next tick. |
| Errors | A non-nil value received from `errCh` goes to `OnError`. Nil values are ignored. |
| Panics | A panic in `OnFlush` is recovered, converted to an error that names the task, and passed to `OnError`. The queue keeps running. |
| Stop | See below. |

### Options

| Field | Default | Meaning |
| --- | --- | --- |
| `FlushInterval` | 1 s when zero or negative | Tick period. |
| `OnFlush` | none | Required. A nil `OnFlush` makes every flush report a nil-dereference error to `OnError`. |
| `OnError` | none | Optional. Called on the queue goroutine, so a slow handler delays event intake. |
| `Capacity` | 10 when zero or negative | Initial capacity of the buffer only. It is not a limit: the buffer grows, nothing is dropped, and senders are not blocked by it. |
| `Debug` | `false` | Appends a stack trace to the error produced from a recovered `OnFlush` panic. |

There is no backpressure. If `OnFlush` is slower than the event rate, the buffer
grows without bound.

### Stopping and ownership

The queue goroutine stops when any of these happens:

- the queue's task is canceled (call `Finish` on it, or cancel a parent task);
- `eventCh` is closed;
- `errCh` is closed.

On stop it waits for a flush in progress and passes that flush's error to
`OnError`, then calls `queueTask.Finish(nil)` and stops the ticker. Events that
are still buffered and were never handed to `OnFlush` are discarded.

Pass a subtask dedicated to this queue, created with `needFinish=true`, such as
`componentTask.Subtask("change-queue", true)`, so a parent's `FinishAndWait` or
`task.WaitExit` waits for the last flush. The queue finishes that task itself.
The quick start uses `RootTask` only to stay self-contained.

Two practical consequences:

- Closing either channel stops the queue, even if the other channel is still in
  use. Do not close `errCh` as a way to say "no more errors". Pass `nil` for a
  channel you never use. A nil channel is simply never selected.
- After the queue stops, nobody reads `eventCh`. A producer that sends on an
  unbuffered channel would block forever, so select on the queue's context:

```go
select {
case eventCh <- ev:
case <-queueTask.Context().Done():
}
```

## Errors

`OnError` receives:

- every non-nil error sent on `errCh`, unchanged;
- one error per `OnFlush` panic. It is a goutils [`errs`](../errs) error whose
  subject is the queue task's name, prefixed to the original error when the panic
  value was an error. Its `Error()` text contains ANSI styling, so do not compare
  it as a plain string.

The package writes no logs. Where the errors go is up to `OnError`.

## Concurrency

Channels are the only way to feed the queue. Call `Start` once. `OnFlush` runs on
a goroutine of its own, one call at a time. `OnError` runs on the queue goroutine.
They can overlap, because an error arriving on `errCh` is handled while a flush is
running, so guard state they share.

## Reference

```go
func New[Event any](queueTask *task.Task, opt Options[Event]) *EventQueue[Event]
func (e *EventQueue[Event]) Start(eventCh <-chan Event, errCh <-chan error)

type Options[Event any] struct {
	Capacity      int
	FlushInterval time.Duration
	OnFlush       OnFlushFunc[Event] // func(events []Event)
	OnError       OnErrorFunc        // func(err error)
	Debug         bool
}
```

`go doc -all github.com/yusing/goutils/eventqueue` lists the exported API.

## Testing

Use a subtask of `task.GetTestTask(t)` as `queueTask`, a short `FlushInterval`,
and channels you control. In your cleanup, call `FinishAndWait` on that subtask
so `OnFlush` has returned before the test ends.
