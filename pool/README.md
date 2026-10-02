# goutils/pool

A concurrent registry of keyed objects. Use it to track live things such as
servers, routes, or watched containers: add by key, look up, remove, and list in
a stable order, with optional logging and a bounded event history of changes.

This is not an object-reuse pool. For reusable byte buffers see
[`synk`](../synk/README.md).

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/pool"
```

The package is in the root module and needs Go 1.27 or later.

## Quick start

```go
package main

import (
	"fmt"

	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/pool"
)

// Server satisfies pool.Object.
type Server struct {
	ID    string
	Label string
	Addr  string
}

func (s Server) Key() string  { return s.ID }
func (s Server) Name() string { return s.Label }

func main() {
	history := events.NewHistory()

	servers := pool.New[Server]("servers", "servers")
	servers.SetEventHistory(history) // optional; call before sharing the pool

	servers.Add(Server{"2", "web-02", "10.0.0.2"})
	servers.Add(Server{"1", "web-01", "10.0.0.1"})

	if s, ok := servers.Get("1"); ok {
		fmt.Println("found", s.Addr)
	}
	for _, s := range servers.Slice() { // sorted by Name()
		fmt.Println(s.Label)
	}

	servers.DelKey("1")
	_, ok := servers.Get("1")
	fmt.Println("after delete:", ok, "events:", len(history.Get()))
}
```

Output:

```text
found 10.0.0.1
web-01
web-02
after delete: false events: 2
```

## Concepts

`New[T Object](name, eventKey string) *Pool[T]`

| Argument | Used for |
| --- | --- |
| `name` | A human-readable pool name. It prefixes every log message (`servers: added web-01`) and is returned by `Name()`. |
| `eventKey` | The suffix of the event category. Events are recorded with category `"pool." + eventKey`. It is ignored unless an event history is set. |

Objects implement `Object`:

```go
type Object interface {
	Key() string  // unique, stable while the object is in the pool
	Name() string // used for sorting and logs
}
```

Optional interfaces:

- `ObjectWithDisplayName` adds `DisplayName() string`. Log messages then read
  `servers: added <display> (<name>)`. Sorting still uses `Name()`.
- `Preferable` adds `PreferOver(other any) bool`, checked on the new object
  when its key is already live. The new object replaces the old one only if
  `PreferOver` returns true. `other` is the object currently stored, as a `T`.
  Without it, the last `Add` wins.

## Operations

| Method | Behavior |
| --- | --- |
| `Add(obj)` / `AddKey(key, obj)` | Stores `obj` under `obj.Key()` or the explicit key. A live key is replaced unless `Preferable` says to keep the existing one. A key removed less than 1 s ago is reported as `reloaded`, otherwise as `added`. |
| `AddIfNotExists(obj) (actual, added)` | Stores `obj` only if its key is free. If a live object holds the key, returns it and `false`. It does not consult `Preferable`. Removed keys are treated as absent; recent removals are reported as `reloaded`. |
| `Get(key) (T, bool)` | The live object, or the zero value and `false`. |
| `Del(obj)` / `DelKey(key)` | Removes logically. The key stops being visible to `Get`, `Iter`, and `Slice` at once, but an internal tombstone remains. Removing a missing key does nothing. |
| `Iter(fn func(key string, v T) bool)` | Calls `fn` for every live entry, in no particular order, until `fn` returns false. Concurrent changes may or may not be seen. |
| `Slice() []T` | A new slice of live objects sorted by `Name()`. |
| `Size() int` | Number of stored entries, tombstones included. It can exceed the number of live objects. |
| `Clear()` | Drops every entry silently: no log message and no event. |
| `PurgeExpiredTombs() int` | Deletes tombstones older than 1 s, emits their `removed` log and event, and returns how many it purged. |
| `DisableLog(bool)` | Stops this pool's log messages. Events are still recorded. |
| `SetEventHistory(*events.History)` | Records changes in the history. |
| `Name() string` | The `name` given to `New`. |

### Removal and tombstones

`Del` does not log or record anything. The `removed` log message and event are
produced when the tombstone is purged by `PurgeExpiredTombs`. The pool calls it
itself only when more than 256 tombstones have accumulated. If you rely on
removal logs, removal events, or an accurate `Size()`, call `PurgeExpiredTombs`
periodically, for example from a ticker. Re-adding a key replaces its tombstone
and produces `reloaded` or `added` instead of `removed`.

## Events and logging

With `SetEventHistory(h)` the pool adds an event to `h` for every change:

| Action | `Data` |
| --- | --- |
| `added`, `reloaded` | The object itself. |
| `removed` | A JSON-encodable record with `name`, `display`, and `removed_at`. |

Events have level `info`, category `"pool." + eventKey`, and the history keeps
only the newest 100 events across all producers; see
[`events`](../events/README.md). For `added` and `reloaded`, `Data` holds the pooled value itself. If `T` is a pointer type, later mutations
show up in events already recorded, so keep pooled objects effectively immutable. To stream the history as JSON, the objects must
marshal with `encoding/json/v2`: a struct with no exported fields cannot be
marshaled and ends the stream with an error.

Log messages go through [`logging`](../logging/README.md) at `Info`, and the
package is silent until the application calls `logging.SetLogger`. `DisableLog`
turns the pool's messages off.

## Concurrency

All methods are safe for concurrent use, with these exceptions and limits:

- Call `SetEventHistory` during setup, before the pool is shared. It is a plain
  field write.
- There is no pool-wide mutation lock. Updates and preference checks are atomic per
  key using the concurrent map. Iteration can still observe concurrent changes.
- `Clear` atomically replaces the registry. Operations already in flight may finish
  against the previous registry and emit their logs or events afterward.

## Build tags

| Tag | Effect |
| --- | --- |
| `debug` | `Add` and `AddKey` log a `Warn` with a stack trace, `<name>: key <key> already exists`, when they are about to replace a live key. |

## Dependencies

`github.com/puzpuzpuz/xsync/v4` for the concurrent map, plus `events` and
[`logging`](../logging/README.md) from this module.
