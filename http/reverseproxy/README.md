# goutils/http/reverseproxy

Enhanced HTTP reverse proxy with WebSocket and HTTP/2 support, integrated with godoxy's logging and access control systems.

## Overview

This package extends Go's `net/http/httputil/reverse_proxy` with:

- **Integrated logging**: Uses godoxy's zerolog logger for structured logging
- **Access logging**: Built-in support for `accesslog.AccessLogger`
- **Scheme mismatch handling**: Custom callback for HTTP/HTTPS scheme mismatches
- **Enhanced header manipulation**: X-Forwarded headers, WebSocket header normalization
- **HTTP/2 h2c support**: Cleartext HTTP/2 support
- **Error suppression**: Suppresses certain unimportant/expected errors to prevent logging them
- **Performance optimizations**: Uses `CopyCloseContext` with sync.Pool for efficient body copying, reducing GC pressure

## Key Differences from stdlib

| Feature             | stdlib `httputil/reverse_proxy` | This Package                        |
| ------------------- | ------------------------------- | ----------------------------------- |
| Logging             | `log.Printf`                    | `zerolog.Logger`                    |
| Access logging      | Not supported                   | `AccessLogger` interface            |
| Scheme mismatch     | Not supported                   | `OnSchemeMisMatch` callback         |
| X-Forwarded headers | Manual via `Director`           | Built-in `SetXForwarded()`          |
| WebSocket headers   | Basic handling                  | Normalized casing                   |
| Body copy           | `io.Copy`                       | `CopyCloseContext` with buffer pool |

## Integration with GoDoxy

```go
import (
    "github.com/yusing/godoxy/goutils/http/reverseproxy"
    "github.com/yusing/godoxy/internal/logging"
)

// Create proxy with godoxy's logger
rp := reverseproxy.NewReverseProxy("backend", target, nil)
rp.Logger = logging.Logger  // Uses godoxy's zerolog logger
rp.AccessLogger = logging.AccessLogger  // Uses godoxy's access logging
```

## API Reference

### ReverseProxy

```go
type ReverseProxy struct {
    zerolog.Logger  // Embedded for structured logging
    Transport http.RoundTripper
    ModifyResponse func(*http.Response) error
    AccessLogger accesslog.AccessLogger
    HandlerFunc http.HandlerFunc
    OnSchemeMisMatch func() bool  // Called on HTTP/HTTPS scheme mismatch
    TargetName string
    TargetURL *url.URL
}
```

### ProxyRequest

```go
type ProxyRequest struct {
    In *http.Request   // Incoming request (read-only)
    Out *http.Request  // Outgoing request (modifiable)
}
```

### Functions

#### NewReverseProxy

```go
func NewReverseProxy(name string, target *url.URL, transport http.RoundTripper) *ReverseProxy
```

Creates a proxy. Transport can be `nil` to use default.

#### SetXForwarded

```go
func (p *ProxyRequest) SetXForwarded()
```

Sets all X-Forwarded headers (For, Host, Proto, Method, URI, Port) on the outbound request.

### Handler

```go
func (p *ReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request)
```

Main request handler with automatic:

- Hop-by-hop header removal
- X-Forwarded header setting
- WebSocket upgrade handling
- Response modification

## Usage Examples

### Basic Setup

```go
target, _ := url.Parse("http://localhost:8080")
rp := reverseproxy.NewReverseProxy("backend", target, nil)
http.Handle("/", rp)
```

### With GoDoxy Logging

```go
import "github.com/yusing/godoxy/internal/logging"

rp := reverseproxy.NewReverseProxy("backend", target, nil)
rp.Logger = logging.With().Str("component", "reverse_proxy").Logger()
```

### With Scheme Mismatch Detection

```go
rp := reverseproxy.NewReverseProxy("backend", target, nil)
rp.OnSchemeMisMatch = func() bool {
    rp.Logger.Warn().Msg("Scheme mismatch detected")
    return true  // Retry with different scheme
}
```

### Custom Request Rewrite

```go
rewriteFunc := func(pr *reverseproxy.ProxyRequest) {
    pr.SetXForwarded()
    pr.Out.URL.Path = strings.TrimPrefix(pr.Out.URL.Path, "/api")
}
```

## Performance

- **Buffer pooling**: Uses `CopyCloseContext` with sync.Pool for efficient body copying, reducing GC pressure
- **Context propagation**: Properly handles request cancellation via context

- **HTTP/1.1**: Full support
- **HTTP/2**: h2c cleartext support
- **WebSocket**: Automatic upgrade handling with header normalization

## Header Handling

1. Removes hop-by-hop headers: `Connection`, `Transfer-Encoding`, `Upgrade`, etc.
2. Sets X-Forwarded headers automatically
3. Removes service headers: `X-Powered-By`, `Server`
4. Normalizes WebSocket header casing

## Related Packages

- [accesslog](../accesslog/README.md) - Access logging interface
- [httpheaders](../httpheaders/README.md) - Header manipulation utilities
- [websocket](../websocket/README.md) - WebSocket utilities
