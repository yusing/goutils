# Lifecycle, concurrency, and events

Covers `task`, `eventqueue`, `pool`, `events`, `events/acl`, `events/http`, `synk`, and
`synk/workerpool`. Package READMEs carry the full API and diagrams.

## task: object lifetimes

A `*task.Task` is a node in a process-wide tree. It owns a cancelable context, cleanup callbacks,
and child tasks. Use it for long-lived components (servers, watchers, pollers) whose shutdown must
be coordinated; use a plain `context.Context` for request-scoped work.

```go
func main() {
	logging.SetLogger(myLogger) // shutdown warnings are silent otherwise

	app := task.RootTask("app", true)
	if err := startPoller(app); err != nil {
		app.Finish(err)
		os.Exit(1)
	}
	task.WaitExit(10) // blocks for SIGINT/SIGTERM/SIGHUP, then shuts the tree down
}

func startPoller(parent task.Parent) error {
	t := parent.Subtask("poller", true)
	go func() {
		defer t.Finish(nil) // needFinish=true: the owner must call Finish
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-t.Context().Done():
				return
			case <-tick.C:
				poll(t.Context())
			}
		}
	}()
	return nil
}
```

Rules that are easy to get wrong:

- `RootTask(name, needFinish)` creates a child of a hidden process root. Only `WaitExit` finishes
  that root, and `WaitExit` waits for its own signal: finishing your tasks first does not make it
  return. It returns after shutdown (bounded by its timeout in seconds) and does not exit the
  process.
- `needFinish=true` means the task counts as running until someone calls `Finish`; parents and
  `WaitExit` wait for it (up to a 3 s per-task timeout, then a Warn "stucked" report through
  `logging`). `needFinish=false` finishes itself as soon as its context is canceled; use it for
  tasks with nothing to drain.
- `OnCancel(name, fn)` runs when the context is canceled. `OnFinished(name, fn)` runs after this
  task's own `Finish` (for `needFinish=false`, it behaves like `OnCancel`); it does not wait for
  children. For children-first cleanup, finish with `FinishAndWait` from the parent and run the
  parent cleanup after it returns.
- Register callbacks and create subtasks before `Finish`. A callback added after cancellation
  never runs and makes `FinishAndWait` wait the full timeout.
- `Finish` is asynchronous and idempotent; a repeated `Finish` blocks up to 3 s waiting.
  `FinishAndWait` blocks until children and callbacks finish or the timeout elapses.
- Callbacks run on their own goroutines with panic recovery. Under `-tags debug` a callback panic
  is logged and re-raised, crashing the process.
- `FinishCause()` returns `context.Canceled` for `Finish(nil)`, an error built from a string
  reason, or the error passed in; process shutdown uses `task.ErrProgramExiting`.
- `SetValue`/`GetValue` store values visible to descendants, and `Context().Value(key)` reads
  them, so values reach code that only receives the context.
- Components follow `TaskStarter` (`Start(parent task.Parent) error`, `Task() *task.Task`) and
  `TaskFinisher` (`Finish(reason any)`). On a failed `Start`, finish the subtask you created.

For tests, see `testing.md`.

## eventqueue: batched event processing

```go
q := eventqueue.New(parent.Subtask("file-events", true), eventqueue.Options[FileEvent]{
	FlushInterval: 500 * time.Millisecond, // default 1s
	OnFlush:       func(batch []FileEvent) { handle(batch) },
	OnError:       func(err error) { logging.Log(logging.Warn, "watcher", logging.Field{Key: "error", Value: err}) },
})
q.Start(eventCh, errCh)
```

- `Start` launches the processing goroutine; `New` does not. The queue finishes the task you pass
  in when that task is canceled or when either channel closes, after waiting for an in-flight
  flush. Pending unflushed events are discarded.
- `Capacity` is only the initial buffer capacity; the buffer grows without bound and there is no
  backpressure. Producers should select on the queue task's `Context().Done()`.
- Flushes run on a separate goroutine, serialized with each other; events keep buffering during a
  flush. A panic in `OnFlush` becomes an error passed to `OnError` (with a stack when
  `Debug: true`). `OnError` can run concurrently with `OnFlush`.

## pool: keyed object registry

`pool.Pool[T]` is a concurrent registry of objects keyed by string, not a reuse pool for buffers
(use `synk` for that). `T` implements `pool.Object` (`Key()`, `Name()`); optional
`ObjectWithDisplayName` changes log names and `Preferable` (`PreferOver(other any) bool`) decides
replacement on duplicate keys.

```go
routes := pool.New[*Route]("routes", "routes") // name prefixes logs; eventKey sets event category "pool.routes"
routes.SetEventHistory(history)                 // optional; call before concurrent use
routes.Add(r)
if r, ok := routes.Get("example.com"); ok { use(r) }
routes.DelKey("example.com")
for _, r := range routes.Slice() { ... }        // sorted by Name()
```

- Deletion leaves a 1 s tombstone; `Get`, `Iter`, and `Slice` skip it, but `Size` counts it. The
  "removed" log/event is emitted when tombstones are purged (automatically past 256, or via
  `PurgeExpiredTombs`).
- Add/remove diagnostics go through `logging` at Info unless `DisableLog(true)`; events are still
  recorded.
- Known defects: `Slice` can panic after `Del` followed by `Clear` (the tombstone counter is not
  reset); `AddIfNotExists` does not store over an expired but unpurged tombstone; and "removed"
  events carry unexported data that makes `History.ListenJSON` stop with an encoding error. Check
  `pool/README.md` for the current list before relying on these paths.

## events: in-process event history

`events.History` is a 100-entry ring with live listeners, used by `pool`, `aclevents`, and
`httpevents`. Make one per application and attach it to the task tree:

```go
history := events.NewHistory() // the zero value is not usable
app := task.RootTask("app", true)
events.SetCtx(app, history)

history.Add(events.NewEvent(events.LevelInfo, "config", "reloaded", map[string]any{"files": 3}))
current, ch, cancel := history.SnapshotAndListen()
defer cancel()
```

- `events.FromCtx(ctx)` finds the history only in contexts derived from the task passed to
  `SetCtx`. For an `http.Server`, set `BaseContext` to return that task's context, or handlers see
  no history.
- Listener channels buffer 64 events and drop on overflow. `ListenJSON(ctx, w)` writes the
  snapshot and then each new event as one newline-terminated `Write`, encoded with
  `encoding/json/v2`; event `Data` must be JSON-encodable.
- `aclevents.Blocked(ctx, ip, reason)` and `httpevents.Blocked(r, source, reason)` (separate
  modules) record "blocked" events from that context's history, collapsing concurrent duplicates
  per IP (ACL) or remote IP and host (HTTP). Without a history in the context they do nothing.
  `httpevents` reads the IP from `r.RemoteAddr` only.

## synk: buffer pools and small sync helpers

```go
pool := synk.GetSizedBytesPool()
buf := pool.GetSized(n) // len(buf) == n
defer pool.Put(buf)

ub := synk.GetUnsizedBytesPool()
b := ub.GetBuffer() // *bytes.Buffer
defer ub.PutBuffer(b)
```

- Use the sized pool when the length is known (sized tiers 2 KiB to 2 MiB; larger requests
  allocate and are dropped on `Put`), and the unsized pool for growing buffers.
- `Put`/`PutBuffer` transfers ownership: never touch, append to, or return the buffer afterwards.
  Reused memory is not zeroed, so clear secrets before `Put` and overwrite before reading.
- Reuse is best-effort (weak references), so never depend on getting the same buffer back.
- `synk.Value[T]` is a typed `atomic.Value` (`Store` panics on nil and on differing concrete
  types). `NewRefCounter()` starts at 1; `Zero()` closes when `Sub` brings it to 0, and `Add` after zero
  panics.
- The pool uses `go:linkname` to `runtime.procPin`; it builds on Go 1.27 without linker flags,
  but run the pool tests after Go upgrades.

## synk/workerpool: bounded concurrency

```go
wp := workerpool.New(ctx, workerpool.WithN(8)) // default n is GOMAXPROCS
for _, item := range items {
	wp.Go(func(ctx context.Context, _ int) { process(ctx, item) })
}
wp.Wait()
```

- `Go` blocks while all slots are busy. After `ctx` is canceled it usually drops the work, and
  `Wait` returns early without waiting for running workers.
- Panics in workers are not recovered, the `idx` argument is a running call counter (not a slot
  number), and `WithN(0)` makes `Go` block forever. Call `Wait` from one goroutine only.
- Prefer `errgroup` when you need errors or first-error cancellation.
