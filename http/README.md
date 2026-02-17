# goutils/http

HTTP utilities for request/response handling, reverse proxy, and WebSocket support.

## Overview

The `http` package provides comprehensive HTTP utilities.

## API Reference

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
