# goutils/server

Starts HTTP, HTTPS, and HTTP/3 servers whose lifetime is tied to a
[`goutils/task`](../task/README.md) task: listen, serve in the background, and shut
down when the task is cancelled. It adds cleartext HTTP/2 (h2c) on the plain HTTP
port, certificate-provider based TLS, optional HTTP/3 with `Alt-Svc` advertising,
connection-level ACL hooks, and PROXY protocol policies. Package name: `server`.

## Install

```sh
go get github.com/yusing/goutils/server@v0.8.0
```

```go
import "github.com/yusing/goutils/server"
```

- Go 1.27. The module requires `github.com/quic-go/quic-go` (HTTP/3),
  `github.com/pires/go-proxyproto`, `github.com/rs/zerolog`,
  `github.com/samber/slog-zerolog/v2`, `golang.org/x/net`, and the sibling modules
  `github.com/yusing/goutils` and `github.com/yusing/goutils/http`; `go get` pulls them
  all in.
- Binaries that link this package, including your own test binaries, need
  `-ldflags=-checklinkname=0` because it uses
  [`httputils`](../http/README.md#required-linker-flag):

  ```sh
  go build -ldflags=-checklinkname=0 ./cmd/app
  go test  -ldflags=-checklinkname=0 ./...
  ```

## Quick start

```go
package main

import (
	"fmt"
	"net/http"

	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/task"
)

func main() {
	root := task.RootTask("app", false)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "hello")
	})

	if _, err := server.StartServer(root, server.Options{
		Name:     "web",
		HTTPAddr: ":8080",
		Handler:  mux,
	}); err != nil {
		panic(err)
	}

	task.WaitExit(5) // block until SIGINT, SIGTERM, or SIGHUP, then shut down (5 s budget)
}
```

`StartServer` binds the listeners, starts serving in background goroutines, and
returns. It returns `(*Server, error)`; the error describes a failure to bind or to
start a protocol.

## Lifecycle and ownership

- Each protocol runs under its own subtask of the task you pass in, named
  `<Name>.http`, `<Name>.https`, and `<Name>.http3`. Cancelling or finishing the parent
  stops them. Typical shutdown is `root.FinishAndWait(reason)` or `task.WaitExit(n)`.
- On cancellation the listener is closed first, then `Shutdown` waits up to one second
  for in-flight requests. The wait uses a fresh deadline that ignores the already
  cancelled parent, so responses that finish within a second are flushed.
- Every request context derives from the server's task, so handlers see
  `r.Context().Done()` as soon as shutdown starts, even while `Shutdown` is still
  waiting for them. Do not overwrite `http.Server.BaseContext` on a server you pass to
  `Start`. Hijacked connections (WebSocket, for example) are not waited for.
- `Server` has no `Close` or `Shutdown` method; stopping is done through the task.
  `Uptime()` is the time since `Start` began.
- If `Start` fails partway (for example, HTTP is up but the HTTPS port is busy), it
  returns the error but the servers that did start keep running until the parent task is
  cancelled. Cancel the parent on error if that is not what you want.
- If both `HTTPAddr` and `HTTPSAddr` are empty, `Start` does nothing and returns `nil`.

## Options

```go
type Options struct {
	Name             string
	HTTPAddr         string
	HTTPSAddr        string
	HTTPListener     net.Listener
	HTTPSListener    net.Listener
	CertProvider     CertProvider
	Handler          http.Handler
	ACL              ACL
	TLSConfigMutator func(*tls.Config) *tls.Config

	SupportProxyProtocol bool // Deprecated: use ProxyProtocolPolicy.
	ProxyProtocolPolicy  ProxyProtocolPolicy
}
```

- `Handler` is required. The plain HTTP server wraps it in `h2c.NewHandler`, so with a
  nil `Handler` every plain-HTTP request panics inside the wrapper (`net/http` recovers
  and drops the connection). The plain HTTP port also accepts cleartext HTTP/2 (h2c).
- `HTTPAddr` and `HTTPSAddr` decide which servers exist. A server is not created when
  its address is empty, even if you supplied a listener for it, and that listener is
  then never served. When you provide `HTTPListener` or `HTTPSListener`, the address
  value is not used for binding, so any non-empty placeholder works. The listener's
  `Addr()` must return a `*net.TCPAddr`; the port is taken from it with a type assertion
  that panics for other types, such as Unix sockets.
- `HTTPS` exists only when `HTTPSAddr` is set **and** `CertProvider.GetCert(nil)` succeeds.
  `NewServer` calls `GetCert` once with a `nil` `*tls.ClientHelloInfo` to probe the
  provider, so the implementation must tolerate `nil` and return a default
  certificate. If the probe fails the HTTPS server is skipped silently, with no error and
  no log line, and HTTP/3 is skipped with it.
- `TLSConfigMutator` receives the TLS configuration (`MinVersion` TLS 1.2, `h2` and
  `http/1.1` ALPN, `GetCertificate` from the provider) and returns the one to use. It
  must not return `nil`.
- `ACL` is any value with `WrapTCP(net.Listener) net.Listener` and
  `WrapUDP(net.PacketConn) net.PacketConn`. It wraps the TCP listener after PROXY
  protocol and TLS handling, so on HTTPS the wrapper receives the TLS listener, not
  the raw TCP one. HTTP/3 UDP sockets go through `WrapUDP`.
- `NewServer(opt)` builds a `*Server` without starting it, and `(*Server).Start(parent,
  http3Enabled)` starts it with an explicit HTTP/3 switch. `StartServer` is both steps
  with the `HTTP3_ENABLED` environment setting.

### TLS example

```go
package main

import (
	"crypto/tls"
	"log"
	"net/http"

	"github.com/yusing/goutils/server"
	"github.com/yusing/goutils/task"
)

type staticCert struct{ cert tls.Certificate }

// GetCert is also called once with a nil hello when the server is created.
func (c staticCert) GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return &c.cert, nil
}

func main() {
	cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		log.Fatal(err)
	}

	root := task.RootTask("app", false)
	_, err = server.StartServer(root, server.Options{
		Name:         "web",
		HTTPAddr:     ":8080",
		HTTPSAddr:    ":8443",
		CertProvider: staticCert{cert},
		Handler:      http.NotFoundHandler(),
	})
	if err != nil {
		log.Fatal(err)
	}
	task.WaitExit(5)
}
```

## HTTP/3

HTTP/3 is on when the HTTPS server exists and HTTP/3 is requested: `HTTP3_ENABLED` for
`StartServer`, or the second argument of `(*Server).Start`.

- It listens on UDP at `HTTPSAddr` (a real `host:port`; the `HTTPSListener` placeholder
  trick does not apply) and uses the same handler and TLS configuration, with `h3`
  added to ALPN.
- HTTP/1.1 and HTTP/2 responses from both the HTTP and HTTPS servers gain an
  `Alt-Svc: h3=":<port>"; ma=2592000` header so clients can upgrade.
- If the UDP port cannot be bound, `Start` returns `failed to start HTTP/3 server` and
  neither TCP server is started.
- It is not started when a PROXY protocol policy is enabled; a warning is logged
  instead (and see the limitation below).

## PROXY protocol

```go
policy, err := server.NewProxyProtocolPolicy(server.ProxyProtocolConfig{
	Mode:           server.ProxyProtocolModeRequired,
	TrustedProxies: []string{"172.18.0.10", "10.0.0.0/8"},
})
if err != nil {
	return err
}
opts := server.Options{Name: "web", HTTPAddr: ":8080", Handler: h, ProxyProtocolPolicy: policy}
```

| Mode | Behavior |
| --- | --- |
| `disabled` | Connections are not wrapped. |
| `mixed` | Peers in `TrustedProxies` must send a PROXY header; every other peer is treated as a direct client and its bytes are not parsed. |
| `required` | Only peers in `TrustedProxies` are accepted, and each must send a valid header. |

`TrustedProxies` is a list of IPs or CIDR ranges and must be non-empty for `mixed` and
`required`. `ProxyProtocolConfig.Validate()` checks a configuration without building a
policy, and the type has JSON tags (`mode`, `trusted_proxies`). `SupportProxyProtocol:
true` selects the deprecated `NewLegacyProxyProtocolPolicy()`, which accepts an optional
header from any peer, so any client can claim an arbitrary source address. An explicit
`ProxyProtocolPolicy` (even `disabled`) takes precedence over the legacy flag.

Known limitation: starting the HTTPS server with an enabled policy currently panics with
`http: HTTP/2 Server already registered` (`HTTP3_ENABLED` does not matter). Use PROXY
protocol with the plain HTTP server only, or terminate TLS and PROXY protocol outside this
package.

## Lower-level API

`Start` serves a single `*http.Server` or `*http3.Server` on a task you supply and
returns the port it actually bound (useful with `":0"`):

```text
Start(task *task.Task, srv *http.Server or *http3.Server, opts ...ServerStartOption) (port int, err error)
```

```go
root := task.RootTask("app", false)
srv := &http.Server{Addr: "127.0.0.1:0", Handler: h}
port, err := server.Start(root.Subtask("http", true), srv)
```

- On error `Start` returns before serving and does **not** finish the subtask. Call
  `subtask.Finish(err)` yourself, otherwise a task created with `needFinish` true blocks
  shutdown of its parent.
- A nil server finishes the task with `server not configured` and returns port 0 and a
  `nil` error.
- Options: `WithListener`, `WithACL`, `WithTCPWrappers`, `WithUDPWrappers`,
  `WithLogger`, `WithProxyProtocolPolicy`, and the deprecated `WithProxyProtocolSupport`.
  `WithTCPWrappers` and `WithUDPWrappers` replace the wrappers set by earlier options,
  `WithACL` included, so pass them before `WithACL`.

## Logging and environment

- Logs go to zerolog's global logger (`github.com/rs/zerolog/log`) with a `server` field
  holding `Options.Name`, not to `goutils/logging`. Start and stop are logged at info
  level; serve and shutdown failures at error level. Zerolog's default global logger
  writes JSON to stderr, so unconfigured applications see these lines.
- `HTTP3_ENABLED` and `SERVER_DEBUG` are defined by `goutils/env/godoxy` and read once
  when the package is initialized. Each name is looked up with the prefixes `GODOXY_`,
  `GOPROXY_`, and then none (for example `GODOXY_HTTP3_ENABLED`, then `HTTP3_ENABLED`),
  and both default to `false`.
  - `HTTP3_ENABLED` is the HTTP/3 switch for `StartServer`; it does not affect
    `(*Server).Start`, which takes the switch as an argument.
  - `SERVER_DEBUG` routes the `net/http` error log and the HTTP/3 logger to zerolog. An
    explicit `DEBUG=true` enables it too; the `DEBUG` default derived for Go test
    binaries does not.
  - A value that is not a Go boolean (for example `DEBUG=app:*`) panics at startup.

## Related packages

- [task](../task/README.md) owns the lifetime model used here.
- [reverseproxy](../http/reverseproxy/README.md) is a typical `Handler`.
- [httputils](../http/README.md) provides the diagnostics logging used for HTTP/3
  advertising errors and the required linker flag.
