package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	h2proxy "github.com/pires/go-proxyproto/helper/http2"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/goutils/env"
	"github.com/yusing/goutils/task"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

type CertProvider interface {
	GetCert(_ *tls.ClientHelloInfo) (*tls.Certificate, error)
}

type ACL interface {
	WrapTCP(l net.Listener) net.Listener
	WrapUDP(l net.PacketConn) net.PacketConn
}

type Server struct {
	Name                string
	CertProvider        CertProvider
	http                *http.Server
	https               *http.Server
	httpListener        net.Listener
	httpsListener       net.Listener
	startTime           time.Time
	acl                 ACL
	proxyProtocolPolicy ProxyProtocolPolicy
	drainTimeout        time.Duration
	initErr             error

	l zerolog.Logger
}

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

	// SupportProxyProtocol enables legacy optional PROXY-header detection.
	// Deprecated: configure ProxyProtocolPolicy instead.
	SupportProxyProtocol bool
	ProxyProtocolPolicy  ProxyProtocolPolicy

	// DrainTimeout is how long in-flight HTTP/1.1 and HTTP/2 requests may run
	// after the server task is cancelled. Default: 1 second. See WithDrainTimeout.
	DrainTimeout time.Duration
}

type (
	TCPWrapper = func(l net.Listener) net.Listener
	UDPWrapper = func(l net.PacketConn) net.PacketConn
)

type httpServer interface {
	*http.Server | *http3.Server
	Shutdown(ctx context.Context) error
}

var (
	envServerDebug  = env.GetEnvBool("SERVER_DEBUG", false) || env.GetEnvBool("DEBUG", false)
	envHTTP3Enabled = env.GetEnvBool("HTTP3_ENABLED", false)
)

func StartServer(parent task.Parent, opt Options) (s *Server, err error) {
	s = NewServer(opt)
	err = s.Start(parent, envHTTP3Enabled)
	return s, err
}

func NewServer(opt Options) (s *Server) {
	var httpSer, httpsSer *http.Server

	logger := log.With().Str("server", opt.Name).Logger()

	var initErr error
	if opt.HTTPSAddr != "" {
		if opt.CertProvider == nil {
			initErr = errors.New("HTTPS requested without a certificate provider")
		} else if cert, err := opt.CertProvider.GetCert(nil); err != nil {
			initErr = fmt.Errorf("failed to load HTTPS certificate: %w", err)
		} else if cert == nil {
			initErr = errors.New("HTTPS certificate provider returned no certificate")
		}
	}

	if opt.HTTPAddr != "" {
		httpSer = &http.Server{
			Addr:    opt.HTTPAddr,
			Handler: h2c.NewHandler(opt.Handler, &http2.Server{}),
		}
	}
	if initErr == nil && opt.HTTPSAddr != "" {
		tlsConfig := &tls.Config{
			GetCertificate: opt.CertProvider.GetCert,
			MinVersion:     tls.VersionTLS12,
			NextProtos:     []string{http2.NextProtoTLS, "http/1.1"},
		}
		if opt.TLSConfigMutator != nil {
			tlsConfig = opt.TLSConfigMutator(tlsConfig)
		}
		httpsSer = &http.Server{
			Addr:      opt.HTTPSAddr,
			Handler:   opt.Handler,
			TLSConfig: tlsConfig,
		}
		if err := http2.ConfigureServer(httpsSer, &http2.Server{}); err != nil {
			initErr = fmt.Errorf("failed to configure HTTP/2 for HTTPS server: %w", err)
		}
	}
	return &Server{
		Name:                opt.Name,
		CertProvider:        opt.CertProvider,
		http:                httpSer,
		https:               httpsSer,
		httpListener:        opt.HTTPListener,
		httpsListener:       opt.HTTPSListener,
		l:                   logger,
		acl:                 opt.ACL,
		proxyProtocolPolicy: resolveProxyProtocolPolicy(opt.ProxyProtocolPolicy, opt.SupportProxyProtocol),
		drainTimeout:        opt.DrainTimeout,
		initErr:             initErr,
	}
}

// Start will start the http and https servers.
//
// If both are not set, this does nothing.
//
// Start() is non-blocking if no error is returned.
//
// If an error occurs, it will wait for started servers to finish and return the error.
func (s *Server) Start(parent task.Parent, http3Enabled bool) (startErr error) {
	if s.initErr != nil {
		return s.initErr
	}
	var started []*task.Task
	defer func() {
		if startErr != nil {
			for _, subtask := range started {
				subtask.Finish(startErr)
			}
			for _, subtask := range started {
				subtask.FinishAndWait(startErr)
			}
		}
	}()
	taskName := func(proto string) string {
		return s.Name + "." + proto
	}
	s.startTime = time.Now()

	if s.https != nil && http3Enabled {
		if s.proxyProtocolPolicy.Enabled() {
			// TODO: support proxy protocol for HTTP/3
			s.l.Warn().Msg("HTTP/3 is enabled, but proxy protocol is yet not supported for HTTP/3")
		} else {
			s.https.TLSConfig.NextProtos = []string{http3.NextProtoH3, "h2", "http/1.1"}
			h3 := &http3.Server{
				Addr:      s.https.Addr,
				Handler:   s.https.Handler,
				TLSConfig: http3.ConfigureTLSConfig(s.https.TLSConfig),
			}
			subtask := parent.Subtask(taskName("http3"), true)
			started = append(started, subtask)
			_, err := Start(subtask, h3, WithProxyProtocolPolicy(s.proxyProtocolPolicy), WithACL(s.acl), WithLogger(&s.l))
			if err != nil {
				subtask.Finish(err)
				return fmt.Errorf("failed to start HTTP/3 server: %w", err)
			}
			if s.http != nil {
				s.http.Handler = advertiseHTTP3(s.http.Handler, h3)
			}
			// s.https is not nil (checked above)
			s.https.Handler = advertiseHTTP3(s.https.Handler, h3)
		}
	}

	// Only create proto tasks for servers that exist. A needFinish task with no
	// Serve goroutine never Finishes and pins parent shutdown.
	if s.http != nil {
		subtask := parent.Subtask(taskName("http"), true)
		started = append(started, subtask)
		_, err := Start(subtask, s.http, WithListener(s.httpListener), WithProxyProtocolPolicy(s.proxyProtocolPolicy), WithACL(s.acl), WithLogger(&s.l), WithDrainTimeout(s.drainTimeout))
		if err != nil {
			subtask.Finish(err)
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
	}

	if s.https != nil {
		subtask := parent.Subtask(taskName("https"), true)
		started = append(started, subtask)
		_, err := Start(subtask, s.https, WithListener(s.httpsListener), WithProxyProtocolPolicy(s.proxyProtocolPolicy), WithACL(s.acl), WithLogger(&s.l), WithDrainTimeout(s.drainTimeout))
		if err != nil {
			subtask.Finish(err)
			return fmt.Errorf("failed to start HTTPS server: %w", err)
		}
	}
	return nil
}

type ServerStartOptions struct {
	tcpWrappers         []func(l net.Listener) net.Listener
	udpWrappers         []func(l net.PacketConn) net.PacketConn
	logger              *zerolog.Logger
	proxyProto          bool
	proxyProtocolPolicy ProxyProtocolPolicy
	listener            net.Listener
	drainTimeout        time.Duration
}

type ServerStartOption func(opts *ServerStartOptions)

// WithDrainTimeout sets how long in-flight requests may run after the server
// task is cancelled before they are cancelled too. Default: 1 second.
// Applies to HTTP/1.1 and HTTP/2; HTTP/3 servers stop as before.
func WithDrainTimeout(timeout time.Duration) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.drainTimeout = timeout
	}
}

func WithTCPWrappers(wrappers ...TCPWrapper) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.tcpWrappers = append(opts.tcpWrappers, wrappers...)
	}
}

func WithUDPWrappers(wrappers ...UDPWrapper) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.udpWrappers = append(opts.udpWrappers, wrappers...)
	}
}

func WithLogger(logger *zerolog.Logger) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.logger = logger
	}
}

func WithACL(acl ACL) ServerStartOption {
	return func(opts *ServerStartOptions) {
		if acl == nil {
			return
		}
		opts.tcpWrappers = append(opts.tcpWrappers, acl.WrapTCP)
		opts.udpWrappers = append(opts.udpWrappers, acl.WrapUDP)
	}
}

func WithProxyProtocolSupport(value bool) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.proxyProto = value
	}
}

func WithProxyProtocolPolicy(policy ProxyProtocolPolicy) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.proxyProtocolPolicy = policy
	}
}

func WithListener(listener net.Listener) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.listener = listener
	}
}

func Start[Server httpServer](task *task.Task, srv Server, optFns ...ServerStartOption) (port int, err error) {
	if srv == nil {
		task.Finish("server not configured")
		return port, nil
	}

	var opts ServerStartOptions
	for _, optFn := range optFns {
		optFn(&opts)
	}
	if opts.logger == nil {
		opts.logger = &log.Logger
	}

	if envServerDebug {
		setLogger(srv, opts.logger)
	}

	proto := proto(srv)
	proxyProtocolPolicy := resolveProxyProtocolPolicy(opts.proxyProtocolPolicy, opts.proxyProto)
	proxyProtocolEnabled := proxyProtocolPolicy.Enabled()

	var lc net.ListenConfig
	var serveFunc func() error

	switch srv := any(srv).(type) {
	case *http.Server:
		// Request contexts must not end the moment the task is cancelled (e.g.
		// on a config reload): stopHTTP drains them first and cancels this
		// context, with the task's cause, when the drain window ends.
		// Set before any Serve goroutine (h2proxy starts one) reads srv.
		baseCtx, cancelBase := context.WithCancelCause(context.WithoutCancel(task.Context()))
		srv.BaseContext = func(net.Listener) context.Context {
			return baseCtx
		}
		stopRegistered := false
		defer func() {
			if !stopRegistered {
				cancelBase(errors.New("server not started"))
			}
		}()
		l := opts.listener
		if l == nil {
			l, err = lc.Listen(task.Context(), "tcp", srv.Addr)
			if err != nil {
				return port, err
			}
		}
		tcpAddr, ok := l.Addr().(*net.TCPAddr)
		if !ok {
			if opts.listener == nil {
				_ = l.Close()
			}
			return 0, fmt.Errorf("expected TCP listener address, got %T", l.Addr())
		}
		port = tcpAddr.Port
		if proxyProtocolEnabled {
			l = proxyProtocolPolicy.Wrap(l)
		}
		if srv.TLSConfig != nil {
			l = tls.NewListener(l, srv.TLSConfig)
		}
		for _, wrapper := range opts.tcpWrappers {
			l = wrapper(l)
		}
		// TLS connections negotiate HTTP/2 directly; net/http already owns their
		// configured HTTP/2 server. The helper is only needed for cleartext PROXY
		// ALPN TLVs and would configure TLS HTTP/2 a second time.
		if proxyProtocolEnabled && srv.TLSConfig == nil {
			serveFunc = getServeFunc(l, h2proxy.NewServer(srv, nil).Serve)
		} else {
			serveFunc = getServeFunc(l, srv.Serve)
		}
		task.OnCancel("stop", func() {
			stopHTTP(srv, l, proto, opts.logger, opts.drainTimeout, cancelBase, context.Cause(task.Context()))
		})
		stopRegistered = true
	case *http3.Server:
		srv.ConnContext = func(ctx context.Context, c *quic.Conn) context.Context {
			subtask := task.Subtask("conn", true)
			context.AfterFunc(ctx, func() {
				subtask.Finish(ctx.Err())
			})
			subtask.SetValue(http3.ServerContextKey, srv)
			subtask.SetValue(http.LocalAddrContextKey, c.LocalAddr())
			subtask.SetValue(http3.RemoteAddrContextKey, c.RemoteAddr())
			return subtask.Context()
		}
		l, err := lc.ListenPacket(task.Context(), "udp", srv.Addr)
		if err != nil {
			return port, err
		}
		udpAddr, ok := l.LocalAddr().(*net.UDPAddr)
		if !ok {
			_ = l.Close()
			return 0, fmt.Errorf("expected UDP listener address, got %T", l.LocalAddr())
		}
		port = udpAddr.Port
		for _, wrapper := range opts.udpWrappers {
			l = wrapper(l)
		}
		serveFunc = getServeFunc(l, srv.Serve)
		task.OnCancel("stop", func() {
			stop(srv, l, proto, opts.logger)
		})
	}
	logStarted(srv, opts.logger)
	go func() {
		err := convertError(serveFunc())
		if err != nil {
			opts.logger.Err(err).Msg("failed to serve " + proto + " server")
		}
		task.Finish(err)
	}()
	return port, nil
}

func resolveProxyProtocolPolicy(policy ProxyProtocolPolicy, legacy bool) ProxyProtocolPolicy {
	if policy.configured {
		return policy
	}
	if legacy {
		return NewLegacyProxyProtocolPolicy()
	}
	return policy
}

// defaultDrainTimeout is how long in-flight requests may run after a stop.
const defaultDrainTimeout = time.Second

// stopHTTP stops an HTTP/1.1 / HTTP/2 server without cutting in-flight
// requests at once:
//
//  1. The listener is closed first, so the port is free for a replacement
//     server (config reload) right away.
//  2. Shutdown waits up to drain for active requests to finish.
//  3. Request contexts, including hijacked connections such as WebSockets
//     that Shutdown does not track, are cancelled with cause when the window
//     ends; connections still open are closed.
//
// On a reload this runs in the background, so stopping the task does not
// wait for the drain. On program exit it runs synchronously so responses are
// flushed before the process ends, and it does not wait out the window once
// Shutdown is done.
func stopHTTP(srv *http.Server, l io.Closer, proto string, logger *zerolog.Logger, drain time.Duration, cancelBase context.CancelCauseFunc, cause error) {
	if drain <= 0 {
		drain = defaultDrainTimeout
	}
	if err := convertError(l.Close()); err != nil {
		logger.Err(err).Msg("failed to close " + proto + " listener")
	}

	exiting := false
	select {
	case <-task.RootContextCanceled():
		exiting = true
	default:
	}

	drainAndStop := func() {
		// This runs after the task is cancelled; strip that cancellation so
		// Shutdown actually waits.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(task.RootContext()), drain)
		defer cancel()

		err := convertError(srv.Shutdown(ctx))
		if !exiting {
			<-ctx.Done() // give hijacked connections the rest of the window
		}
		cancelBase(cause)
		if err != nil {
			_ = srv.Close()
			logger.Info().Str("proto", proto).Str("addr", addr(srv)).Dur("drain", drain).Msg("server stopped, requests still running after the drain window were cancelled")
			return
		}
		logger.Info().Str("proto", proto).Str("addr", addr(srv)).Msg("server stopped")
	}
	if exiting {
		drainAndStop()
	} else {
		go drainAndStop()
	}
}

func stop[Server httpServer](srv Server, l io.Closer, proto string, logger *zerolog.Logger) {
	if srv == nil {
		return
	}

	// This runs as an OnCancel callback, so on program exit the root context is
	// already canceled. Strip the cancellation, otherwise Shutdown returns at once
	// and in-flight responses lose their connection instead of being flushed.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(task.RootContext()), 1*time.Second)
	defer cancel()

	// Close the listener first so socket/port is released as early as possible
	// during reloads (especially for UDP/HTTP3).
	if err := convertError(errors.Join(l.Close(), srv.Shutdown(ctx))); err != nil {
		logger.Err(err).Msg("failed to shutdown " + proto + " server")
	} else {
		logger.Info().Str("proto", proto).Str("addr", addr(srv)).Msg("server stopped")
	}
}

func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}
