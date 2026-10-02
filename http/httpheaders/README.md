# goutils/http/httpheaders

Header helpers for proxies and middleware: hop-by-hop removal, upgrade and
WebSocket detection, gRPC/SSE detection, header copying and filtering, and
Content-Security-Policy rewriting. Package name: `httpheaders`.

## Install

```sh
go get github.com/yusing/goutils/http@v0.8.0
```

```go
import "github.com/yusing/goutils/http/httpheaders"
```

`httpheaders` lives in the `github.com/yusing/goutils/http` module (Go 1.27). It
depends only on `golang.org/x/net` and the root `goutils` module, does not log, and
does not need the `-checklinkname=0` linker flag that the sibling
[`httputils`](../README.md) package requires.

## Quick start

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/yusing/goutils/http/httpheaders"
)

func main() {
	h := http.Header{
		"Connection": {"Upgrade, X-Debug"},
		"Upgrade":    {"websocket"},
		"X-Debug":    {"1"},
		"Keep-Alive": {"timeout=5"},
	}

	fmt.Println(httpheaders.IsWebsocket(h)) // true

	httpheaders.RemoveHop(h)
	fmt.Println(h) // map[Connection:[Upgrade] Upgrade:[websocket]]
}
```

## Hop-by-hop headers

- `RemoveHopByHopHeaders(h)` deletes every header named in `Connection` (RFC 7230
  section 6.1), then `Connection`, `Proxy-Connection`, `Keep-Alive`,
  `Proxy-Authenticate`, `Proxy-Authorization`, `Te`, `Trailer`, `Transfer-Encoding`,
  and `Upgrade`. It always removes `Upgrade`.
- `RemoveHop(h)` does the same but keeps an upgrade handshake intact: when the
  request asked for an upgrade (see `UpgradeType`), it sets `Connection: Upgrade` and
  `Upgrade: <original value>` again; otherwise no `Connection` header remains. `Te`
  is removed, so a proxy that must forward `TE: trailers` re-adds it afterwards.
- `RemoveServiceHeaders(h)` deletes `X-Powered-By` and `Server`.

## Upgrade, gRPC, and SSE detection

- `UpgradeType(h)` returns the `Upgrade` header value only when `Connection`
  contains the `Upgrade` token (as a comma-separated list member); otherwise `""`.
  An `Upgrade` header without a matching `Connection` header is ignored.
- `IsWebsocket(h)` is `UpgradeType(h) == "websocket"`. The comparison is
  case-sensitive, so `Upgrade: WebSocket` is not detected; use
  `strings.EqualFold(httpheaders.UpgradeType(h), "websocket")` when peers may
  capitalize it.
- `IsGrpcOrSSE(h)` is true when the `Content-Type` media type is `text/event-stream`
  or starts with `application/grpc` (both case-insensitive, parameters ignored).
  Streaming code uses it to decide whether to flush after every write.

## Copying and filtering

- `CopyHeader(dst, src)` appends every value of `src` to `dst` with `Add`. Values
  already in `dst` are kept, not replaced; clear or `Del` them first to overwrite.
- `FilterHeaders(h, allowed)` returns a new `http.Header` holding only the allowed
  names that are present in `h` (names are canonicalized, values copied). An empty
  `allowed` returns `h` itself, not a copy.
- `HeaderToMap(h)` returns a `map[string]string` holding the first value of each
  header.

## Content-Security-Policy

```go
func AppendCSP(w http.ResponseWriter, r *http.Request, cspDirectives, sources []string)
```

`AppendCSP` builds a new policy and stores it on `w`. Despite its signature it reads
the starting policy from the **request's** `Content-Security-Policy` header
(`r.Header`), not from `w.Header()`; whatever policy `w` already carried is
discarded, along with every case variant of the header name.

For each directive in `cspDirectives`:

- if the request policy lacks it, it starts as `'self'`;
- `'self'` becomes `'self' <sources>`;
- `'none'` is replaced by `<sources>` (no `'self'` is added);
- any other value gets each source appended unless that text already occurs in it,
  and `'self'` is prepended when absent.

Directives not listed pass through unchanged (directives without a value are
dropped). The result is written as one `Content-Security-Policy` header value per
directive, in unspecified order.

```go
package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/yusing/goutils/http/httpheaders"
)

func main() {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Content-Security-Policy", "default-src 'none'; img-src 'self'")

	w := httptest.NewRecorder()
	httpheaders.AppendCSP(w, r, []string{"img-src"}, []string{"https://cdn.example.com"})

	// default-src is untouched; img-src gains the CDN. Order may vary.
	fmt.Println(w.Header()["Content-Security-Policy"])
	// [default-src 'none' img-src 'self' https://cdn.example.com]
}
```

## Constants

```go
const (
	HeaderXForwardedMethod = "X-Forwarded-Method"
	HeaderXForwardedFor    = "X-Forwarded-For"
	HeaderXForwardedProto  = "X-Forwarded-Proto"
	HeaderXForwardedHost   = "X-Forwarded-Host"
	HeaderXForwardedPort   = "X-Forwarded-Port"
	HeaderXForwardedURI    = "X-Forwarded-Uri"
	HeaderXRealIP          = "X-Real-IP"

	HeaderContentType   = "Content-Type"
	HeaderContentLength = "Content-Length"
)
```

Run `go doc -all github.com/yusing/goutils/http/httpheaders` for the full API.

## Related packages

- [reverseproxy](../reverseproxy/README.md) uses these helpers to clean and forward
  headers.
- [websocket](../websocket/README.md) serves WebSocket connections; `IsWebsocket`
  detects the handshake.
