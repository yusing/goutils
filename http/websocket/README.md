# goutils/http/websocket

Server-side WebSocket helpers for [Gin](https://github.com/gin-gonic/gin) handlers,
built on [gorilla/websocket](https://github.com/gorilla/websocket). A `Manager`
upgrades the request, answers an application-level `ping`/`pong` heartbeat, delivers
incoming messages over a channel, serializes writes, and closes the connection when
the request context ends. Package name: `websocket`.

## Install

```sh
go get github.com/yusing/goutils/http/websocket@v0.8.0
```

```go
import "github.com/yusing/goutils/http/websocket"
```

Things to know before you add it:

- It is tied to Gin: `NewManagerWithUpgrade` takes a `*gin.Context`. The module
  requires `github.com/gin-gonic/gin` v1.12.0 and `github.com/gorilla/websocket`
  v1.5.3, plus the root `github.com/yusing/goutils` module (all pulled in by
  `go get`). It does not work with plain `net/http` handlers.
- Gorilla's package is also named `websocket`. Alias one of them when you import both,
  for example `gws "github.com/gorilla/websocket"`.
- Binaries that link this package, including your own test binaries, need
  `-ldflags=-checklinkname=0`. `DeepEqual` reaches into the runtime through the
  `github.com/yusing/gointernals` linkname helper, and the Go linker rejects that by
  default:

  ```sh
  go build -ldflags=-checklinkname=0 ./cmd/app
  go test  -ldflags=-checklinkname=0 ./...
  ```

  `go vet` and compiling non-`main` packages work without it. See the
  [`httputils` README](../README.md#required-linker-flag).
- Go 1.27. JSON helpers use `encoding/json/v2` (through `goutils/strings`), not
  `encoding/json`; `time.Duration` values are encoded as nanosecond numbers.

## Quick start

```go
package main

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/yusing/goutils/http/websocket"
)

func main() {
	gin.SetMode(gin.ReleaseMode) // see "Heartbeat" for why this matters
	r := gin.New()

	r.GET("/ws", func(c *gin.Context) {
		m, err := websocket.NewManagerWithUpgrade(c)
		if err != nil {
			return // the upgrader already wrote the HTTP error response
		}
		defer m.Close()

		// Echo every message until the connection ends. The handler must keep
		// running: returning from it closes the connection.
		for {
			select {
			case <-m.Done():
				return
			case data := <-m.ReadCh():
				if err := m.WriteData(websocket.TextMessage, data, 5*time.Second); err != nil {
					return
				}
			}
		}
	})

	log.Fatal(r.Run(":8080"))
}
```

To push JSON on a timer, hand the whole handler to `PeriodicWrite`:

```go
r.GET("/stats", func(c *gin.Context) {
	websocket.PeriodicWrite(c, time.Second, func() (any, error) {
		return map[string]int{"clients": 3}, nil
	})
})
```

It upgrades the request, sends the value of `get` immediately and then every
`interval`, skips a message when it equals the previous one, and returns when the
connection ends or `get` returns an error. It writes no response of its own: failures
are recorded with `c.Error` as `apitypes.InternalServerError` values (from
`goutils/apitypes`) for your Gin error middleware.

## Lifecycle and ownership

- `NewManagerWithUpgrade(c)` hijacks the connection. On failure (not a WebSocket
  request, bad origin, and so on) gorilla has already written a 4xx response, so just
  return from the handler.
- The `Manager` owns the connection. Call `Close()` (usually `defer m.Close()`) when
  your handler is done; it is idempotent, sends a normal-closure (1000) close frame,
  closes the socket, and cancels `m.Context()`.
- The handler must block until the connection is finished. The manager's context is
  derived from the request context, and returning from the handler cancels it, which
  closes the WebSocket (a client of a handler that returned right after the upgrade
  receives a normal-closure close frame). Wait on `m.Done()`, a read loop, or
  `PeriodicWrite`.
- The manager also closes itself when the peer closes, a read fails, the heartbeat
  lapses, a pong cannot be written, or the request context is cancelled.
  `m.Done()` and `m.Context()` report all of these.
- Writes (`WriteJSON`, `WriteData`, `NewWriter`, `PeriodicWrite`, the `Copy*` methods)
  are serialized with a mutex and safe to call from several goroutines. Reads are
  consumed by one internal goroutine and delivered through a channel; do not read from
  the underlying connection yourself (it is not exposed).

## Reading

Incoming text and binary messages arrive on a channel with a buffer of one. The
reader goroutine blocks while that channel is full, so a handler that never reads
applies backpressure to the peer. The same goroutine answers heartbeat pings, so a
handler that stops draining the channel also stops getting `pong` replies, and the
heartbeat check closes the connection about 6 seconds later. Keep reading (or do not
let the peer send data you do not consume).

- `ReadCh() <-chan []byte` is the raw feed. It is never closed; always select on
  `m.Done()` as well.
- `ReadBinary(timeout)` returns the next message or `ErrReadTimeout`.
- `ReadJSON(&out, timeout)` does the same and decodes it.
- The text message `ping` is consumed by the heartbeat and never reaches these
  readers.

After the connection ends, `ReadBinary` and `ReadJSON` return the recorded failure
or `net.ErrClosed` for a clean shutdown. A successful read therefore always has data
to decode or a received message (which may be empty).

No read size limit is configured, so an untrusted client can send arbitrarily large
messages. Put a limit in front of the handler if that matters.

## Writing

All writers take a per-call timeout that becomes the write deadline.

- `WriteJSON(v, timeout)` marshals `v` and sends it as a text message.
- `WriteData(type, data, timeout)` sends a raw message (`websocket.TextMessage` or
  `websocket.BinaryMessage`).
- `NewWriter(type)` returns an `io.Writer` that writes one message per `Write` with a
  10-second deadline.
- `CopyJSONStream(r)` sends each JSON value read from `r` as its own text message.
- `CopyTextLines(r)` sends each line as its own text message. The line terminator
  `\n` is part of the message, and a final line without one is sent as is. Lines
  longer than the 4 KiB read buffer are accumulated in a pooled buffer; all active
  streams share a 64 MiB budget, and the copy fails with
  `ErrTextBufferBudgetExceeded` when it is exhausted.
- `(*Manager).PeriodicWrite(interval, get, dedupe...)` runs the same loop on an
  existing manager and returns the error that ended it (`nil` after a clean close).

After the connection ends, `WriteData` and `WriteJSON` return the recorded failure
or `net.ErrClosed` for a clean shutdown.

A write timeout returns `ErrWriteTimeout`. Treat any write error as fatal: gorilla
connections cannot be reused after one.

### Deduplication

`PeriodicWrite` skips a tick when the new value equals the previous one. The first
value is always sent. By default equality is `websocket.DeepEqual`, a reflective
comparison of numbers, strings, maps, slices, arrays, and structs (exported fields
only). Pass a `DeduplicateFunc` to replace it:

```go
websocket.PeriodicWrite(c, time.Second, get, func(a, b any) bool {
	return a.(Snapshot).Version == b.(Snapshot).Version
})
```

`DeduplicateFunc` receives `(last, current)`: the previous value first and the new
value second. Return `true` to skip the write.

## Heartbeat

The manager disconnects clients that do not speak its application-level heartbeat.
This is a text message, not a WebSocket ping frame:

- The client sends a text message whose payload is exactly `ping`.
- The server replies with a text message `pong` (2-second write deadline) and records
  the time.
- Every 3 seconds the server checks the last ping. If it is more than 5 seconds old
  the connection is closed (close code 1000). A client that never pings is dropped
  about 6 seconds after the upgrade. Send a ping every 2 to 3 seconds.

```js
const ws = new WebSocket("wss://example.com/ws");
setInterval(() => ws.readyState === WebSocket.OPEN && ws.send("ping"), 2500);
ws.onmessage = (e) => { if (e.data !== "pong") console.log(e.data); };
```

Your clients must implement this, and a handler that reads `ReadCh` never sees `ping`
or needs to answer it. The heartbeat can be switched off only while Gin is in debug
mode (`gin.Mode() == gin.DebugMode`, Gin's default unless `GIN_MODE=release` or
`gin.SetMode` is used) by adding `?no-ping=true` (or `1`) to the URL. Set release mode
in production so clients cannot turn the idle-timeout off.

## Origin check, subprotocols, and compression

The default upgrader accepts a request when:

- it has no `Origin` header; or
- the origin parses with a host and its hostname equals the request `Host` hostname,
  compared case-insensitively and ignoring ports and scheme.

Anything else gets a 403, including cross-host origins on localhost and loopback
addresses. External reverse proxies must preserve the public Host, or the application
must supply an explicit custom origin policy.

If the client offers a subprotocol beginning with `csrf.` (`CSRFSecWebSocketProtocolPrefix`),
the server selects the first such offer and echoes it in `Sec-WebSocket-Protocol`.
This lets a browser smuggle a CSRF token through the subprotocol list. The package
does not validate the token; check it in your own middleware.

`permessage-deflate` compression is enabled at `BestSpeed`.

To change the upgrader, store a `*websocket.Upgrader` from gorilla under the context
key `"upgrader"` before upgrading. Any other value type panics:

```go
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	gws "github.com/gorilla/websocket"
	"github.com/yusing/goutils/http/websocket"
)

func main() {
	r := gin.New()
	r.GET("/ws", func(c *gin.Context) {
		c.Set("upgrader", &gws.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") == "https://app.example.com"
			},
		})
		m, err := websocket.NewManagerWithUpgrade(c)
		if err != nil {
			return
		}
		defer m.Close()
		<-m.Done()
	})
	_ = r.Run(":8080")
}
```

A custom upgrader replaces the default one completely, including its origin check and
its `EnableCompression` setting. The manager still asks for write compression at
`BestSpeed`, which has an effect only when the upgrader negotiated compression.

## Reader adapter

`NewReader()` returns an `io.Reader` with a 10-second wait for each new message.
Small reads preserve the remainder for subsequent calls; empty messages are skipped.
A clean close returns `io.EOF`, while recorded failures are returned unchanged.

## Errors and logging

- `ErrReadTimeout` is returned by `ReadBinary` and `ReadJSON`. Values returned for a
  failed connection wrap the first failure, for example
  `failed to read message: websocket: close 1006 (abnormal closure): unexpected EOF`.
- `ErrTextBufferBudgetExceeded` comes from `CopyTextLines`.
- Logging uses zerolog's global logger (`github.com/rs/zerolog/log`), not
  `goutils/logging`. A connection that ends with an error other than cancellation
  is logged once at debug level. Zerolog's default global logger prints JSON to
  stderr and does not filter debug, so an unconfigured application will show these
  lines; raise the level with `zerolog.SetGlobalLevel`.

## Environment variables

The settings are read through `goutils/env` once, when the package is
initialized. Each name is looked up with the prefixes `GODOXY_`, `GOPROXY_`, and then
none (for example `GODOXY_WEBSOCKET_DEBUG`, then `WEBSOCKET_DEBUG`):

- `WEBSOCKET_DEBUG` (default `false`). When true, ping lapses and close frames other
  than normal closure and going-away are recorded as errors, so reads and writes return
  them and `PeriodicWrite` reports them instead of ending silently. An explicit
  `DEBUG=true` enables it too (`WEBSOCKET_DEBUG || DEBUG`); test binaries have no
  special derived default.

A value that is not a Go boolean (for example `DEBUG=app:*`) makes the process panic at
startup with `env DEBUG: invalid bool value`. Applications that already use `DEBUG`
with other meanings must avoid collisions.

## Related packages

- [reverseproxy](../reverseproxy/README.md) proxies WebSocket upgrades to upstream
  servers.
- [httpheaders](../httpheaders/README.md) provides `IsWebsocket` for detecting the
  handshake.
