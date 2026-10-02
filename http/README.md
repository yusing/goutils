# goutils/http

Helpers for Go HTTP servers, proxies, and clients: pooled body reading, buffered
or lazy response rewriting, a `RoundTripper` that can short-circuit responses,
per-request parse caches, content-negotiation helpers, and request-scoped
diagnostics. Package name: `httputils`.

The Go module `github.com/yusing/goutils/http` contains three packages:

| Import path | Package name | Documentation |
| --- | --- | --- |
| `github.com/yusing/goutils/http` | `httputils` | This file |
| `github.com/yusing/goutils/http/httpheaders` | `httpheaders` | [httpheaders](httpheaders/README.md) |
| `github.com/yusing/goutils/http/accesslog` | `accesslog` | [accesslog](accesslog/README.md) |

The reverse proxy and WebSocket helpers are separate modules with their own
`go.mod`: [reverseproxy](reverseproxy/README.md) and
[websocket](websocket/README.md).

## Install

```sh
go get github.com/yusing/goutils/http@v0.8.0
```

The module requires Go 1.27. It pulls in the root module
`github.com/yusing/goutils` and `golang.org/x/net`; it needs no web framework.
The import path ends in `http` but the package is named `httputils`, so import it
with an explicit name:

```go
import httputils "github.com/yusing/goutils/http"
```

## Required linker flag

Every binary that links this package, including the test binaries of your own
packages, must be built with `-ldflags=-checklinkname=0`. `IsUnexpectedError`
reads unexported errors of `net/http/internal/http2` through `//go:linkname`
(`error.go`), and the Go linker rejects that reference by default:

```text
link: github.com/yusing/goutils/http: invalid reference to net/http/internal/http2.errStreamClosed
```

```sh
go build -ldflags=-checklinkname=0 ./cmd/app
go test  -ldflags=-checklinkname=0 ./...
GOFLAGS=-ldflags=-checklinkname=0 go run ./cmd/app   # same flag through the environment
```

`go vet` and compiling non-`main` packages do not link, so they work without the
flag. `httpheaders` and `accesslog` do not need it. `reverseproxy`, `server`, and
`websocket` need it as well (the first two through this package). This repository's
`scripts/release.py` passes the flag to every module test run.

## Quick start

A middleware that buffers a handler's response, rewrites the body, and a client
that reads a response body from the buffer pool:

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	httputils "github.com/yusing/goutils/http"
)

// shout buffers next's response so the body can be rewritten before it is sent.
func shout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rm := httputils.NewResponseModifier(w)
		defer rm.FlushRelease() // sends status, headers, and body; releases pooled buffers

		next.ServeHTTP(rm, r)

		body := strings.ToUpper(string(rm.Content()))
		_ = rm.SetBody(io.NopCloser(strings.NewReader(body)))
		rm.Header().Set("X-Rewritten", "true")
	})
}

func main() {
	srv := httptest.NewServer(shout(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "hello")
	})))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, release, err := httputils.ReadAllBody(resp)
	if err != nil {
		panic(err)
	}
	defer release(body) // body must not be used after release

	fmt.Println(string(body), resp.Header.Get("X-Rewritten")) // HELLO true
}
```

## Reading bodies

`ReadAllBody(resp)` and `ReadAllRequestBody(req)` read the whole body into a pooled
buffer and return `(b, release, err)`.

- Call `release(b)` once when finished and do not touch `b` afterwards. Copy the
  bytes first (`string(b)`, `bytes.Clone(b)`) if they must outlive the call.
- On error both `b` and `release` are `nil`; the buffer is already returned.
- Neither function closes the body. Close it yourself.
- When `ContentLength > 0`, exactly that many bytes are read into a sized buffer and
  a shorter body fails with `io.ErrUnexpectedEOF`. Otherwise the body is read until
  EOF into a growable buffer.
- No size limit is applied. For untrusted peers, bound the body first (for example
  `http.MaxBytesReader`) and reject oversized `Content-Length` values yourself.

## Rewriting responses

All of these implement `http.ResponseWriter` and `http.Hijacker`.

| Constructor | Behavior |
| --- | --- |
| `NewResponseModifier(w)` | Buffers status and body in memory until `FlushRelease`. Create it once, at the start of the request. |
| `NewPassthroughResponseModifier(w)` | Writes straight through and flushes after each write. Use it for request-only processing where buffering would break streaming responses. |
| `GetInitResponseModifier(w)` | Returns the `ResponseModifier` already wrapping `w` (found through `Unwrap() http.ResponseWriter` chains), or creates a new buffered one. |
| `NewLazyResponseModifier(w, shouldBuffer)` | Decides on the first `WriteHeader` or `Write` whether to buffer, using the response headers. Large or streaming responses pass through untouched. |
| `ResponseAsRW(resp)` | Wraps a received `*http.Response` as a read-oriented `ResponseModifier`: `Header()` is the response's header map, `Write` returns `io.ErrClosedPipe`, and `WriteHeader` only logs an error. |

Lifecycle rules for the buffered `ResponseModifier`:

- Headers and the status stay editable until `FlushRelease()`, which writes the
  status, sets `Content-Length` when the body changed, writes the body, and returns
  the buffers to the pool. It returns the byte count and the aggregated error
  (write errors plus anything recorded with `AppendError`). Call it exactly once,
  normally with `defer`. After it returns, do not use the modifier, its `Content()`,
  or its `SharedData()`.
- Buffered output is sent only by `FlushRelease`, so a forgotten call sends an empty
  `200` response.
- `Content()`, `BodyBuffer()`, `BodyReader()`, `SetBody(rc)`, and `ResetBody()` read
  or replace the buffered body. `ContentLength()` and `StatusCode()` (200 when unset)
  report the pending response.
- `Flush()` does nothing in buffered mode (`FlushError` returns
  `http.ErrNotSupported`) because flushing would bypass the modification. In
  passthrough mode it flushes. `Unwrap` is deliberately not provided.
- `SetMaxBufferedBytes(n)` caps the buffer. When a write would exceed `n`, the status,
  headers (minus `Content-Length`, `Transfer-Encoding`, `Trailer`), and buffered bytes
  are sent and the modifier permanently switches to passthrough
  (`IsPassthrough()`). A value `<= 0` removes the cap.
- After `Hijack()` the modifier writes nothing on `FlushRelease`.
- `SharedData()` (or `GetSharedData(w)`) returns the per-request `Cache` described
  below and releases it in `FlushRelease`.

`LazyResponseModifier` adds `IsBuffered()`, `ResponseModifier()` (nil when not
buffered), `SetMaxBufferedBytes`, and `SetModifyResponse(r, f)`, where `f` receives a
synthetic `*http.Response` (status and headers) before the buffering decision and a
returned error turns the status into 500. Call `FlushRelease()` when done; it is a
no-op returning `(0, nil)` when the response was not buffered.

`ModifyResponseWriter` and `NewModifyResponseWriter` are deprecated. Use the
`ResponseModifier` family instead.

## Intercepting client responses

`NewInterceptedTransport(rt, fn)` wraps a `http.RoundTripper` (`nil` means
`http.DefaultTransport`). After every successful round trip, `fn` receives the
response and returns `(intercepted bool, err error)`:

- `(false, nil)` passes the response through.
- `(true, httputils.NewRequestInterceptedError(resp, data))` stops the request and
  hands `resp` and `data` to the caller as an error.
- `(false, err)` or `(true, err)` with an ordinary `err` fails the request with `err`.

```go
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	httputils "github.com/yusing/goutils/http"
)

func main() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		io.WriteString(w, "short and stout")
	}))
	defer srv.Close()

	client := &http.Client{Transport: httputils.NewInterceptedTransport(nil,
		func(resp *http.Response) (bool, error) {
			if resp.StatusCode != http.StatusTeapot {
				return false, nil
			}
			defer resp.Body.Close() // the intercepted body is not closed for you
			body, release, err := httputils.ReadAllBody(resp)
			if err != nil {
				return false, err
			}
			defer release(body)
			return true, httputils.NewRequestInterceptedError(resp, string(body))
		})}

	_, err := client.Get(srv.URL)
	var intercepted *httputils.RequestInterceptedError
	if httputils.AsRequestInterceptedError(err, &intercepted) {
		fmt.Println(intercepted.Response.StatusCode, intercepted.Data) // 418 short and stout
	}
}
```

`http.Client` wraps transport errors in `*url.Error`, so extract the value with
`AsRequestInterceptedError` rather than a type assertion. `RequestInterceptedError.Is`
reports `true` for `context.Canceled` and `context.DeadlineExceeded`, so check for it
before treating those as client cancellation.

## Request diagnostics

`LogDebug`, `LogInfo`, `LogWarn`, and `LogError` take `(r *http.Request, message
string, fields ...logging.Field)`. They deliver the message to the logger installed
with [`logging.SetLogger`](../logging/README.md) and add `remote`
(`r.RemoteAddr`), `host`, and `uri` (`METHOD RequestURI`) fields. `message` is not a
format string. Until an application installs a logger, nothing is emitted.

```go
httputils.LogWarn(r, "upstream slow", logging.Field{Key: "elapsed", Value: elapsed})
```

This package logs only through `goutils/logging`; it does not use zerolog. The
`reverseproxy`, `websocket`, and `server` modules log through zerolog's global
logger instead, as their READMEs describe.

## Per-request cache

`Cache` (`map[string]any`) memoizes parsed request data so middleware stages do not
parse the same value twice: `GetQueries`, `UpdateQueries`, `GetCookies`,
`GetCookiesMap`, `UpdateCookies`, `GetRemoteIP`, and `GetBasicAuth`. Obtain one with
`NewCache()` and `Release()` it when the request ends, or use
`GetSharedData(w)`, which returns the cache owned by the `ResponseModifier` wrapping
`w` (released by `FlushRelease`). If `w` is not wrapped by a `ResponseModifier`,
`GetSharedData` returns a fresh empty `Cache` on every call, so nothing is shared.

## Content negotiation and predicates

- `GetContentType(h)` returns the media type of `Content-Type` without parameters,
  or `""` when absent or invalid. `ContentType` has `IsHTML`, `IsJSON`, and
  `IsPlainText`; constants such as `ContentTypeJSON` are provided.
- `GetAccept(h)` returns the `Accept` media types in header order without q-values,
  or `*/*` when the header is missing. `AcceptHTML`, `AcceptJSON`, `AcceptMarkdown`,
  and `AcceptPlainText` also return true for `*/*`, and `AcceptHTML` and
  `AcceptPlainText` also for `text/*`.
- `IsSuccess(status)` is true for 2xx. `IsStatusCodeValid(status)` is true when
  `http.StatusText` knows the code. `IsMethodValid(method)` accepts the nine
  standard methods.
- `IsUnexpectedError(err)` is for deciding whether an error from a proxied or copied
  body deserves a log line. It returns `false` for `nil`, expected HTTP/2 disconnect
  errors (closed stream, client disconnected, closed response body, stream error
  codes `STREAM_CLOSED` and `CANCEL`) and errors whose `ErrorCode` field is the
  HTTP/3 `H3_NO_ERROR` or `H3_REQUEST_CANCELLED`; everything else is unexpected.

Run `go doc -all github.com/yusing/goutils/http` for the complete exported API.

## Development

```sh
cd http && go test -ldflags=-checklinkname=0 ./...
```

`python3 scripts/release.py test` from the repository root tests every module and
supplies the flag; see the [repository README](../README.md).
