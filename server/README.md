# goutils/server

Server utilities for HTTP/HTTPS/HTTP3 server management.

## Overview

The `server` package provides utilities for managing HTTP servers with TLS, ACL, and proxy protocol support.

## API Reference

### Server

```go
type Server struct {
    Name         string
    CertProvider CertProvider
}

type Options struct {
    Name         string
    HTTPAddr     string
    HTTPSAddr    string
    CertProvider CertProvider
    Handler      http.Handler
    ACL          ACL
    SupportProxyProtocol bool // deprecated legacy mode
    ProxyProtocolPolicy ProxyProtocolPolicy
}

type CertProvider interface {
    GetCert(_ *tls.ClientHelloInfo) (*tls.Certificate, error)
}

type ACL interface {
    WrapTCP(l net.Listener) net.Listener
    WrapUDP(l net.PacketConn) net.PacketConn
}
```

### Functions

```go
func NewServer(opt Options) *Server
func StartServer(parent task.Parent, opt Options) *Server
func (s *Server) Start(parent task.Parent, http3Enabled bool)
func (s *Server) Uptime() time.Duration
```

### Server Start

```go
func Start[Server httpServer](task *task.Task, srv Server, optFns ...ServerStartOption) int
```

### Options

```go
func WithTCPWrappers(wrappers ...TCPWrapper) ServerStartOption
func WithUDPWrappers(wrappers ...UDPWrapper) ServerStartOption
func WithLogger(logger *zerolog.Logger) ServerStartOption
func WithACL(acl ACL) ServerStartOption
func WithProxyProtocolSupport(value bool) ServerStartOption
func WithProxyProtocolPolicy(policy ProxyProtocolPolicy) ServerStartOption

func NewProxyProtocolPolicy(config ProxyProtocolConfig) (ProxyProtocolPolicy, error)
func NewLegacyProxyProtocolPolicy() ProxyProtocolPolicy
func (p ProxyProtocolPolicy) Enabled() bool
func (p ProxyProtocolPolicy) Wrap(listener net.Listener) net.Listener
```

## Usage

```go
policy, err := server.NewProxyProtocolPolicy(server.ProxyProtocolConfig{
    Mode:           server.ProxyProtocolModeRequired,
    TrustedProxies: []string{"172.18.0.10", "10.0.0.0/8"},
})
if err != nil {
    return err
}
httpServer := server.NewServer(server.Options{
    Name:      "main",
    HTTPAddr:  ":8080",
    HTTPSAddr: ":8443",
    Handler:   myHandler,
    CertProvider: myCertProvider,
    ProxyProtocolPolicy: policy,
})
httpServer.Start(parent, true)
```

## Features

- HTTP/HTTPS/HTTP3 support
- TLS with custom certificate providers
- Proxy protocol support
- Required and mixed trusted-source PROXY protocol policies
- ACL integration
- Graceful shutdown
- Task-based lifetime management
