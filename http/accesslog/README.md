# goutils/http/accesslog

The `AccessLogger` interface that [`reverseproxy`](../reverseproxy/README.md) calls to
record proxied requests and proxy errors. The package contains only the interface;
you supply the implementation (a file, stdout, `slog`, a metrics sink, and so on).
Package name: `accesslog`.

## Install

```sh
go get github.com/yusing/goutils/http@v0.8.0
```

```go
import "github.com/yusing/goutils/http/accesslog"
```

`accesslog` lives in the `github.com/yusing/goutils/http` module (Go 1.27), has no
dependencies of its own, and does not need the `-checklinkname=0` linker flag. To
attach a logger to a proxy you also need
`go get github.com/yusing/goutils/http/reverseproxy@v0.8.0`, and binaries that link
the proxy need the flag described in the [`httputils` README](../README.md#required-linker-flag).

## The interface

```go
type AccessLogger interface {
	LogRequest(req *http.Request, res *http.Response)
	LogError(req *http.Request, err error)
	Close() error
}
```

- `LogRequest` is called once for every proxied request that reached the upstream
  round trip, after the response has been handled. `res` is the upstream response,
  or the synthetic `502 Bad Gateway` response the proxy generates when the origin is
  unreachable. By then its body has been consumed or closed, so read only the status
  and headers.
- `LogError` is called with the request and the error whenever the proxy handles an
  error: a failed round trip (before `LogRequest`, including one aborted by the
  client's cancellation), a failing `ModifyResponse`, an invalid `Upgrade` header,
  or a write error while copying the response to the client.
- `Close` is never called by the proxy. The code that creates the logger owns its
  lifetime and calls `Close` itself.

The proxy calls these methods from many request goroutines at once, so an
implementation must be safe for concurrent use.

## Example

```go
package main

import (
	"log/slog"
	"net/http"
	"net/url"

	"github.com/yusing/goutils/http/accesslog"
	"github.com/yusing/goutils/http/reverseproxy"
)

// slogAccessLog writes one structured line per request and per error.
type slogAccessLog struct{ logger *slog.Logger }

var _ accesslog.AccessLogger = slogAccessLog{}

func (l slogAccessLog) LogRequest(req *http.Request, res *http.Response) {
	l.logger.Info("request", "method", req.Method, "path", req.URL.Path, "status", res.StatusCode)
}

func (l slogAccessLog) LogError(req *http.Request, err error) {
	l.logger.Error("proxy error", "path", req.URL.Path, "error", err)
}

func (slogAccessLog) Close() error { return nil }

func main() {
	target, err := url.Parse("http://localhost:8080")
	if err != nil {
		panic(err)
	}

	logger := slogAccessLog{slog.Default()}
	defer logger.Close()

	proxy := reverseproxy.NewReverseProxy("backend", target, http.DefaultTransport)
	proxy.AccessLogger = logger
	_ = http.ListenAndServe(":8000", proxy)
}
```

## Related packages

- [reverseproxy](../reverseproxy/README.md) holds the `AccessLogger` field and calls
  the methods above.
- [httpheaders](../httpheaders/README.md) provides header helpers for custom
  logging.
