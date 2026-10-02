# goutils/http/reverseproxy

A single-target HTTP reverse proxy derived from `net/http/httputil.ReverseProxy`. It
adds cleartext HTTP/2 upstreams (`h2c://`, for example gRPC backends), streaming-aware
flushing, a scheme-mismatch retry hook, an access-log hook, a friendly 502 page when
the origin is unreachable, and WebSocket upgrade handling. Package name:
`reverseproxy`.

## Install

```sh
go get github.com/yusing/goutils/http/reverseproxy@v0.8.0
```

```go
import "github.com/yusing/goutils/http/reverseproxy"
```

Requirements:

- Go 1.27.
- The module depends on `github.com/rs/zerolog` (its public `ReverseProxy` type embeds
  a `zerolog.Logger`) and `golang.org/x/net`.
- Binaries that link the proxy, including your own test binaries, need
  `-ldflags=-checklinkname=0` because the proxy uses
  [`httputils`](../README.md#required-linker-flag):

  ```sh
  go build -ldflags=-checklinkname=0 ./cmd/app
  go test  -ldflags=-checklinkname=0 ./...
  ```

## Quick start

```go
package main

import (
	"log"
	"net/http"
	"net/url"

	"github.com/yusing/goutils/http/reverseproxy"
)

func main() {
	target, err := url.Parse("http://localhost:8080/base")
	if err != nil {
		log.Fatal(err)
	}

	proxy := reverseproxy.NewReverseProxy("backend", target, http.DefaultTransport)
	log.Fatal(http.ListenAndServe(":8000", proxy))
}
```

A request for `/dir?x=1` is sent to `http://localhost:8080/base/dir?x=1`. The target
path is prefixed to the request path, and the target query is merged with the
request query.

`NewReverseProxy(name, target, transport)`:

- `transport` must not be `nil` (the constructor panics). Share one transport across
  proxies for connection reuse.
- `target` must not be `nil` either; the constructor accepts it but `ServeHTTP`
  dereferences it.
- Always build a proxy with the constructor. `ServeHTTP` calls the `HandlerFunc` field,
  which only the constructor fills in.

## What the proxy does

On each request:

- Removes hop-by-hop headers (including those named by `Connection`) and the
  `Forwarded` header, and keeps `TE: trailers`. For `Upgrade` requests it sends
  `Connection: Upgrade` and the original `Upgrade` value, and restores the canonical
  `Sec-WebSocket-*` header casing. An `Upgrade` value that is not printable ASCII is
  rejected with a 500 (see `IsPrint`).
- Sets `X-Forwarded-For` (appending the client IP from `RemoteAddr` to any existing
  chain), `X-Forwarded-Method`, `X-Forwarded-Proto`, `X-Forwarded-Host`, and
  `X-Forwarded-Uri`. It does not set `X-Forwarded-Port` or `X-Real-IP`.
  - `X-Forwarded-Proto` is `https` when the inbound connection used TLS **or** the
    inbound `X-Forwarded-Proto` header is `https`, otherwise `http`. The proxy
    trusts that inbound header.
  - An existing `X-Forwarded-For` from the client is kept and extended, so strip or
    overwrite it in front of the proxy if clients are untrusted. A request with an
    empty `RemoteAddr` adds no empty hop, and a header key present with a `nil` slice
    suppresses the header.
- Sets an empty `User-Agent` when the client sent none, instead of Go's default.
- Calls `http.ResponseController.EnableFullDuplex` for requests with a body, so
  gRPC and other full-duplex streams work over HTTP/1.1.

On each response:

- Removes `Server`, `X-Powered-By`, and hop-by-hop headers, then calls
  `ModifyResponse` if set. The `Request` field of the response it receives is the
  original inbound request. A returned error closes the body and produces a 500.
- Forwards trailers, and flushes headers immediately for gRPC (`application/grpc*`)
  and Server-Sent Events responses. The body is copied in pooled buffers of at most
  32 KiB (from `goutils/synk`, sized from `Content-Length` when known). The proxy
  flushes after every write for gRPC, SSE, and any response without a
  `Content-Length`, so unsized streams are not held back. The copy ends when the
  request context is done.
- Handles `101 Switching Protocols` (WebSocket and similar) by hijacking the client
  connection and piping bytes both ways. This needs an HTTP/1.1 client connection;
  over HTTP/2 the proxy answers 500 because the writer cannot be hijacked.

## Configuration

Fields on `*ReverseProxy` (set them after `NewReverseProxy`, before serving):

| Field | Meaning |
| --- | --- |
| `Transport http.RoundTripper` | Upstream transport. For `h2c://` targets it is replaced by an h2c wrapper at construction, so pass the transport to `NewReverseProxy` instead of reassigning the field. |
| `ModifyResponse func(*http.Response) error` | Optional response hook. It also runs for the synthetic 502 response. |
| `AccessLogger accesslog.AccessLogger` | Optional; see [accesslog](../accesslog/README.md). |
| `OnSchemeMisMatch func(currentScheme string) (retryScheme string, retry bool)` | Optional scheme-mismatch hook (below). |
| `HandlerFunc http.HandlerFunc` | The handler `ServeHTTP` calls. Wrap it to add behavior around the proxy. |
| `TargetName`, `TargetURL` | The name and target given to the constructor. |
| `zerolog.Logger` (embedded) | Initialized with a `name` field; see Logging. |

There is no `Director`, `Rewrite`, `ErrorHandler`, `BufferPool`, or `FlushInterval`.
To change the outbound request, modify the inbound `*http.Request` in a wrapping
handler before the proxy runs, or replace `HandlerFunc`:

```go
orig := proxy.HandlerFunc
proxy.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
	r.Header.Set("X-Tenant", "acme") // cloned into the outbound request
	orig(w, r)
}
```

`ProxyRequest` is exported but nothing in the package consumes it.

### Scheme mismatch

When the round trip fails because the target scheme does not match what the origin
speaks (`http.ErrSchemeMismatch`, or a TLS record-header error such as dialing a
plain-HTTP origin with `https`), `OnSchemeMisMatch` is called once with the scheme
that was just tried (`"http"`, `"https"`, or `"h2c"`). Return the scheme to retry
with and `true` to retry:

```go
proxy.OnSchemeMisMatch = func(currentScheme string) (retryScheme string, retry bool) {
	if currentScheme == "https" {
		return "http", true
	}
	return "", false
}
```

The hook runs at most once per request, and the retry applies to that request only;
`TargetURL` is not modified. A request with a body is retried only when it has
`GetBody` set; otherwise the original error is handled as a failure. Requests that
arrive at a server have no `GetBody`, so in practice only body-less requests (such
as `GET`) are retried unless a wrapping handler sets `GetBody`.

### h2c and gRPC

A target such as `h2c://localhost:50051` makes the proxy speak cleartext HTTP/2 with
prior knowledge to the origin; the outbound URL scheme becomes `http`. The h2c
wrapper dials with the `DialContext` and `DisableCompression` of the transport you
pass when it is an `*http.Transport`. On an h2c proxy, outbound requests with the
`http` scheme also use h2c, and only the `https` scheme uses the original transport.
Upgrade requests (such as WebSocket) are not sent over h2c; they use the original
transport over HTTP/1.1. `TE: trailers` and backend trailers such as `Grpc-Status`
are forwarded.

```go
target, _ := url.Parse("h2c://localhost:50051")
proxy := reverseproxy.NewReverseProxy("grpc-backend", target, http.DefaultTransport)
```

The `integrationtest/` directory is a separate module (not an application
dependency) that runs real gRPC clients through an h2c proxy; its gRPC dependencies
stay out of this module.

## Errors and the unreachable page

If the upstream round trip fails (and `OnSchemeMisMatch` does not recover),
the proxy logs the error, calls `AccessLogger.LogError`, and answers with a
synthetic **502 Bad Gateway**. `ModifyResponse` and `AccessLogger.LogRequest` see this
response like any other. The body is `Origin server is not reachable.` as
`text/plain`, or, for `GET` requests that accept HTML (an `Accept` header allowing
`text/html`, `text/*`, or `*/*`, or no `Accept` header on the path `/`), a minified HTML
retry page with no-store cache headers.

`WriteDebugOriginUnreachablePage(w)` writes that page with status 200 so you can
preview it from a debug route. `IsPrint(s)` reports whether `s` contains only
printable ASCII.

## Logging

This module logs through zerolog's global logger (`github.com/rs/zerolog/log`), not
through `goutils/logging`. Errors the proxy reports (`http proxy error`) are written
there at a level that depends on the error: context cancellation and EOF at trace,
deadline expiry at debug, a TLS-on-plain-HTTP hint at error, and other errors at
error unless `httputils.IsUnexpectedError` marks them as routine disconnects.

The embedded `zerolog.Logger` field is initialized with `{"name": name}` and its
methods are promoted onto `*ReverseProxy`, but the proxy's own messages do not go
through it; replacing it does not change the proxy's output. Configure the global
logger instead (`zerolog/log.Logger`, `zerolog.SetGlobalLevel`) before serving.
Zerolog's default global logger writes JSON to stderr, so unconfigured
applications see these messages.

## Related packages

- [accesslog](../accesslog/README.md): the `AccessLogger` interface.
- [httpheaders](../httpheaders/README.md): the header helpers the proxy uses.
- [httputils](../README.md): `IsUnexpectedError`, `GetAccept`, and the required
  linker flag.
- [websocket](../websocket/README.md): server-side WebSocket helpers.
