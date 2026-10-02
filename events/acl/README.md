# goutils/events/acl

Records "this IP was blocked" in the [event history](../README.md) attached to a
context. Call it from the code that rejects a connection or request by IP, and
a status page or event stream can show recent blocks without that code knowing
about either.

The import path is `github.com/yusing/goutils/events/acl`, but the package name
is `aclevents`.

## Install

This is a separate Go module so the root module stays free of
`golang.org/x/sync`, which provides the duplicate suppression. It requires the
root module for `events`.

```sh
go get github.com/yusing/goutils/events/acl@v0.8.0
go get github.com/yusing/goutils@v0.8.0
```

Keep all goutils modules at the same version. Mixing versions is unsupported.

## Quick start

```go
package main

import (
	"fmt"

	"github.com/yusing/goutils/events"
	aclevents "github.com/yusing/goutils/events/acl"
	"github.com/yusing/goutils/task"
)

func main() {
	history := events.NewHistory()

	app := task.RootTask("app", false)
	events.SetCtx(app, history) // once, at startup

	// In the code that rejects the connection, with a context derived from app:
	aclevents.Blocked(app.Context(), "203.0.113.7", "denied by rule 12")

	for _, e := range history.Get() {
		fmt.Println(e.Category, e.Action, e.Data)
	}
	// acl_event blocked map[ip:203.0.113.7 reason:denied by rule 12]
}
```

## `Blocked`

```go
func Blocked(ctx context.Context, ip string, reason string)
```

`Blocked` looks up the history with `events.FromCtx(ctx)` and, if there is one,
adds this event:

| Field | Value |
| --- | --- |
| `Level` | `info` |
| `Category` | `acl_event` |
| `Action` | `blocked` |
| `Data` | `map[string]any{"ip": ip, "reason": reason}` |

It returns nothing and cannot fail. It is safe for concurrent use. It does not
log, write a response, or block the caller.

### How it relates to the history in the context

The history comes from the context, not from an argument. It exists only if:

1. the application created one with `events.NewHistory()` and attached it with
   `events.SetCtx(task, history)` on a [`task.Task`](../../task/README.md), and
2. `ctx` descends from that task's `Context()`, directly or through
   `context.WithTimeout`, `WithCancel`, and similar, or through its subtasks.

If `ctx` carries no history, for example `context.Background()`, `Blocked` does
nothing and reports nothing. A missing history is therefore the first thing to
check when events do not appear. When the call comes from an HTTP handler, make
the server's `BaseContext` return the task's context, as shown in the
[events README](../README.md#attaching-a-history-to-a-context).

The history stores only the newest 100 events across all producers, and it is
what [`History.ListenJSON`](../README.md#streaming-as-json) streams. A flood of
blocked clients can therefore push other event types out of the buffer.

### Duplicate suppression

Concurrent calls for the same `ip` collapse into one recorded event, using a
`singleflight` group keyed by `ip`. This only merges calls that overlap in time.
Calls made one after another are each recorded. Consequences:

- Simultaneous rejections of one address are recorded once, with the `reason` of
  whichever call ran first. How many calls of a burst overlap depends on timing,
  so do not use the event count as a counter.
- The group is process-wide and keyed by IP only. If two histories are in use
  (for example, one per tenant), simultaneous blocks of the same IP can end up in
  only one of them.
- Repeated sequential blocks are not rate-limited. If one client can be rejected
  thousands of times a second, rate-limit before calling `Blocked`.

### What goes in the event

`ip` and `reason` are stored exactly as given. `ip` is not parsed or normalized;
pass the address the same way each time (for example without a port). Anything
that can read the history, such as a status page, can read the event, so keep
secrets and request contents out of `reason`.

## Dependencies

`github.com/yusing/goutils/events` and `golang.org/x/sync/singleflight`. The
package does not use [`logging`](../../logging/README.md).
