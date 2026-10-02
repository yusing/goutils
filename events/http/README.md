# goutils/events/http

Records "this HTTP request was blocked" in the [event history](../README.md)
attached to the request's context. Call it from the middleware or handler that
rejects a request, and a status page or event stream can show recent blocks with
the remote address, host, and reason, without the middleware knowing about
either.

The import path is `github.com/yusing/goutils/events/http`, but the package name
is `httpevents`.

## Install

This is a separate Go module so the root module stays free of
`golang.org/x/sync`, which provides the duplicate suppression. It requires the
root module for `events`.

```sh
go get github.com/yusing/goutils/events/http@v0.8.0
go get github.com/yusing/goutils@v0.8.0
```

Keep all goutils modules at the same version. Mixing versions is unsupported.

## Quick start

```go
package main

import (
	"context"
	"net"
	"net/http"

	"github.com/yusing/goutils/events"
	httpevents "github.com/yusing/goutils/events/http"
	"github.com/yusing/goutils/task"
)

func main() {
	history := events.NewHistory()

	app := task.RootTask("app", false)
	events.SetCtx(app, history) // once, at startup

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/admin" {
			httpevents.Blocked(r, "admin-guard", "admin path is not public")
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:    ":8080",
		Handler: handler,
		// Request contexts descend from the task, so they carry the history.
		BaseContext: func(net.Listener) context.Context { return app.Context() },
	}
	_ = srv.ListenAndServe()
}
```

A request to `/admin` from `203.0.113.7` over plain HTTP adds this event:

```json
{"uuid":"01a0fc09-a03c-7001-8000-000000000000","timestamp":"2026-10-02T17:54:44+08:00","level":"info","category":"http_event","action":"blocked",
 "data":{"remote_ip":"203.0.113.7","request_url":"http://example.com:8080","source":"admin-guard","reason":"admin path is not public"}}
```

## `Blocked`

```go
func Blocked(r *http.Request, source, reason string)
```

`Blocked` writes nothing to the response and does not log. It adds one event to
the history found with `events.FromCtx(r.Context())`, if there is one:

| Field | Value |
| --- | --- |
| `Level` | `info` |
| `Category` | `http_event` |
| `Action` | `blocked` |
| `Data` | `map[string]any` with the keys below |

| `Data` key | Value |
| --- | --- |
| `remote_ip` | The host part of `r.RemoteAddr`, or `r.RemoteAddr` unchanged if it has no port. `X-Forwarded-For` is not read. If a trusted proxy sits in front, set `r.RemoteAddr` from it first. |
| `request_url` | `scheme://` plus `r.Host`. It never includes the path or query string, so tokens in URLs are not recorded. |
| `source` | Your label for what rejected the request, such as `"acl"` or `"rate-limit"`. |
| `reason` | Your free-form explanation. |

The scheme is `https` when `r.TLS` is set or when `X-Forwarded-Proto` equals
`https` (case-insensitive), otherwise `http`. The header is sent by the client
unless a trusted proxy overwrites it. It only affects the reported text.

It returns nothing and cannot fail. It is safe for concurrent use.

### How it relates to the history in the context

The history comes from the request context. It exists only if:

1. the application created one with `events.NewHistory()` and attached it with
   `events.SetCtx(task, history)` on a [`task.Task`](../../task/README.md), and
2. the request's context descends from that task's `Context()`.

A standard `http.Server` gives each request a context that descends from
`BaseContext`, so set `BaseContext` to return the task's context, as in the
quick start. With the default base context, or in a handler test built with
`httptest.NewRequest`, no history is found. `Blocked` then does nothing and
reports nothing. In tests, give the request the task's context with
`req = req.WithContext(app.Context())`. A missing history is the first thing to
check when events do not appear.

The history stores only the newest 100 events across all producers, and it is
what [`History.ListenJSON`](../README.md#streaming-as-json) streams. A flood of
blocked requests can push other event types out of the buffer.

### Duplicate suppression

Concurrent calls with the same remote IP and `r.Host` collapse into one recorded
event, using a `singleflight` group keyed by `remoteIP|host`. This only merges
calls that overlap in time. Calls made one after another are each recorded.

- Simultaneous blocked requests from one client for one host are recorded once,
  with the `source` and `reason` of whichever call ran first. How many calls of a
  burst overlap depends on timing, so do not use the event count as a counter.
- The group is process-wide. With more than one history in use, simultaneous
  blocks of the same client and host can end up in only one of them.
- Sequential repeats are not rate-limited. If one client can send thousands of
  blocked requests a second, rate-limit before calling `Blocked`.

Anything that can read the history, such as a status page, can read the event.
Keep secrets and request contents out of `source` and `reason`.

## Dependencies

`github.com/yusing/goutils/events` and `golang.org/x/sync/singleflight`. The
package does not use [`logging`](../../logging/README.md).
