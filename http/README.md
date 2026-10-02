# goutils/http

HTTP utilities for request/response handling, reverse proxy, and WebSocket support.

The package is a separate Go module, `github.com/yusing/goutils/http`. Its
HTTP/2 dependency is not required by the root utility module. Existing package
import paths are unchanged; repository consumers use local module replacements
for development. Reverse-proxy and WebSocket modules remain separate.

## Overview

The `http` package provides comprehensive HTTP utilities.

## API Reference

### Request Diagnostics

Configure [`goutils/logging`](../logging/README.md) to receive diagnostics.
`LogError`, `LogWarn`, `LogInfo`, and `LogDebug` accept a request, message, and
optional `logging.Field` values. They emit immediately with `remote`, `host`,
and `uri` fields. Without an installed logger they produce no output.

```go
httputils.LogError(r, "request failed", logging.Field{Key: "error", Value: err})
```

These helpers no longer return a zerolog event. Migrate `LogError(r).Msg(message)`
to `LogError(r, message)` and format `Msgf` messages before calling the helper.
The separate server, reverse-proxy, and WebSocket modules retain their own logging
dependencies.

### Body Reading

```go
func ReadAllBody(resp *http.Response) (b []byte, release func([]byte), err error)
```

### Content Type

```go
func DetectContentType(data []byte) string
func GetExtension(contentType string) string
```

### Interceptors

```go
func InterceptRequest(handler http.Handler, interceptors ...func(*http.Request)) http.Handler
func InterceptResponse(handler http.Handler, interceptors ...func(http.ResponseWriter)) http.Handler
```

### Reverse Proxy

```go
func NewReverseProxy(director func(*http.Request)) *ReverseProxy
func (p *ReverseProxy) WithH2C() *ReverseProxy
```

### WebSocket

```go
type Manager struct {
    func NewManager(handler http.Handler) *Manager
    func (m *Manager) Upgrade(w http.ResponseWriter, r *http.Request) (*Conn, error)
}

type Conn struct {
    func Read() ([]byte, error)
    func Write([]byte) error
    func Close() error
}
```

## Usage

```go
// Read body with buffer pooling
body, release, _ := httputils.ReadAllBody(resp)
defer release(body)

// Request interceptor
intercepted := httputils.InterceptRequest(handler, func(r *http.Request) {
    r.Header.Set("Authorization", "Bearer token")
})

// WebSocket
manager := websocket.NewManager(nil)
conn, _ := manager.Upgrade(w, r)
conn.Write([]byte("hello"))
```
