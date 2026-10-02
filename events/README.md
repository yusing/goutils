# goutils/events

A small in-memory event history with live subscription. It keeps the newest 100
structured events, lets you read them, follow new ones, or stream them as
newline-delimited JSON, and can be attached to a [`task`](../task/README.md) so
any code with the task's context can record events without a global variable.

It is a recent-activity buffer for status pages and diagnostics. It is not a
durable log, a message bus, or an audit trail.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/events"
```

The package is in the root module and needs Go 1.27 or later. The producers
[`events/acl`](acl/README.md) and [`events/http`](http/README.md) are separate
modules.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/task"
)

func main() {
	history := events.NewHistory()

	app := task.RootTask("app", false)
	events.SetCtx(app, history) // attach the history to the task tree

	// Anywhere that has a context derived from app.Context():
	ctx := app.Context()
	if h := events.FromCtx(ctx); h != nil {
		h.Add(events.NewEvent(events.LevelInfo, "deploy", "started",
			map[string]any{"version": "1.2.3"}))
	}

	fmt.Println("stored:", len(history.Get()))

	// Stream the stored events, then new ones, until the context ends.
	streamCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err := history.ListenJSON(streamCtx, os.Stdout)
	fmt.Println(err) // context deadline exceeded
}
```

`ListenJSON` writes one JSON object per line:

```json
{"uuid":"01a0fc01-21b4-7001-8000-000000000000","timestamp":"2026-10-02T17:45:27.732894918+08:00","level":"info","category":"deploy","action":"started","data":{"version":"1.2.3"}}
```

## Events

```go
type Event struct {
	ID        string    `json:"uuid"`      // UUIDv7, set by NewEvent
	Timestamp time.Time `json:"timestamp"` // time.Now() in NewEvent
	Level     Level     `json:"level"`
	Category  string    `json:"category"` // producer, e.g. "pool.servers", "acl_event"
	Action    string    `json:"action"`   // what happened, e.g. "added", "blocked"
	Data      any       `json:"data"`     // payload
}
```

`NewEvent(level, category, action, data)` fills `ID` and `Timestamp`. An empty
level becomes `LevelInfo`. `Level` is a string with four values:
`LevelDebug` (`"debug"`), `LevelInfo` (`"info"`), `LevelWarn` (`"warn"`), and
`LevelError` (`"error"`). Nothing filters by level. It is a label for consumers.

`Data` can be anything, but `ListenJSON` marshals it with `encoding/json/v2`
semantics. A struct with no exported fields cannot be marshaled and fails the
stream, so use maps, structs with exported fields, or types with a
`MarshalJSON` method.

Within goutils, these producers write to a history:

| Category | Actions | Source |
| --- | --- | --- |
| `pool.<eventKey>` | `added`, `reloaded`, `removed` | [`pool`](../pool/README.md) with `SetEventHistory` |
| `acl_event` | `blocked` | [`events/acl`](acl/README.md) |
| `http_event` | `blocked` | [`events/http`](http/README.md) |

## History

Create histories with `NewHistory()`. The zero value accepts `Add` and `Get` but
panics in `SnapshotAndListen` and `ListenJSON` (nil subscriber map). Do not copy a
`History` after use.

| Method | Behavior |
| --- | --- |
| `Add(e)` | Appends one event. When 100 events are stored, the oldest is dropped. |
| `AddAll(es)` | Appends the batch under one lock, so readers see all or none of it. A batch larger than 100 leaves only its newest 100. |
| `Get() []Event` | A copy of the stored events, oldest first. |
| `Clear()` | Forgets the stored events. Current subscribers stay subscribed. |
| `SnapshotAndListen()` | Returns the stored events, a channel of new events, and a cancel function. |
| `ListenJSON(ctx, w)` | Writes the stored events and then new ones to `w` as JSON lines until `ctx` ends or a write fails. |

The limit of 100 is shared by all categories. A burst from one producer can push
out events from another.

All methods are safe for concurrent use.

### Following new events

```go
current, ch, cancel := history.SnapshotAndListen()
defer cancel()

for _, e := range current { // everything stored at the moment of subscribing
	handle(e)
}
for e := range ch { // everything added after that, with no gap or duplicate
	handle(e)
}
```

- `ch` is buffered (64 events) and delivery never blocks `Add`. If a subscriber
  falls behind and its buffer is full, new events are dropped for it without
  notice. Keep the receive loop quick or hand events to another goroutine.
- `cancel` unsubscribes and closes `ch`. It is safe to call more than once. A
  loop `for e := range ch` ends after `cancel`, so run `cancel` from the
  goroutine that should stop, or from `defer`. Always cancel, or the subscriber is
  kept for the life of the history.
- Use `e, ok := <-ch` if you receive in a `select`: after `cancel` a closed
  channel yields zero `Event` values.

### Streaming as JSON

`ListenJSON(ctx, w)` is built for server-sent streams and log tails. It writes
the current events, then each new event, with exactly one `Write` call per event
and a trailing newline, so record-oriented writers keep event boundaries. It
blocks until:

- `ctx` is done, and it returns `ctx.Err()`;
- `w.Write` fails (the error is returned), or writes fewer bytes than given
  (`io.ErrShortWrite`);
- an event cannot be marshaled (the error is returned and the stream ends).

Run it on its own goroutine for each client. It never flushes `w`. For an
`http.ResponseWriter`, pass a small wrapper whose `Write` calls
`http.Flusher.Flush` after writing.

## Attaching a history to a context

```go
func SetCtx(target interface{ SetValue(any, any) }, history *History)
func FromCtx(ctx context.Context) *History
```

`SetCtx` stores the history as a value on a `*task.Task` (or any type with a
`SetValue(key, value any)` method). `FromCtx` finds it again from the task's
context, from contexts derived from it with `context.WithTimeout` and the like,
and from the context of the task's subtasks. It returns nil when none is set.

Because the key is private to this package, a plain `context.Background()`
cannot carry a history. The context has to descend from a task's `Context()`. For
an HTTP server, make request contexts descend from the task:

```go
srv := &http.Server{
	Handler: handler,
	BaseContext: func(net.Listener) context.Context {
		return app.Context() // the task that SetCtx was called on
	},
}
```

Without that, `events.FromCtx(r.Context())` returns nil, and helpers such as
[`events/http`](http/README.md)'s `Blocked` quietly record nothing.

## Dependencies

[`strings`](../strings) from this module, for UUID generation and JSON encoding.
No logging: this package never writes through [`logging`](../logging/README.md).
