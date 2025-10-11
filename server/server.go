package server

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/pires/go-proxyproto"
	h2proxy "github.com/pires/go-proxyproto/helper/http2"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/yusing/goutils/env"
	"github.com/yusing/goutils/task"
)

type CertProvider interface {
	GetCert(_ *tls.ClientHelloInfo) (*tls.Certificate, error)
}

type ACL interface {
	WrapTCP(l net.Listener) net.Listener
	WrapUDP(l net.PacketConn) net.PacketConn
}

type Server struct {
	Name         string
	CertProvider CertProvider
	http         *http.Server
	https        *http.Server
	startTime    time.Time
	acl          ACL
	proxyProto   bool

	l zerolog.Logger
}

type Options struct {
	Name         string
	HTTPAddr     string
	HTTPSAddr    string
	CertProvider CertProvider
	Handler      http.Handler
	ACL          ACL

	SupportProxyProtocol bool
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

func StartServer(parent task.Parent, opt Options) (s *Server) {
	s = NewServer(opt)
	s.Start(parent, envHTTP3Enabled)
	return s
}

func NewServer(opt Options) (s *Server) {
	var httpSer, httpsSer *http.Server

	logger := log.With().Str("server", opt.Name).Logger()

	certAvailable := false
	if opt.CertProvider != nil {
		_, err := opt.CertProvider.GetCert(nil)
		certAvailable = err == nil
	}

	if opt.HTTPAddr != "" {
		httpSer = &http.Server{
			Addr:    opt.HTTPAddr,
			Handler: opt.Handler,
		}
	}
	if certAvailable && opt.HTTPSAddr != "" {
		httpsSer = &http.Server{
			Addr:    opt.HTTPSAddr,
			Handler: opt.Handler,
			TLSConfig: &tls.Config{
				GetCertificate: opt.CertProvider.GetCert,
				MinVersion:     tls.VersionTLS12,
			},
		}
	}
	return &Server{
		Name:         opt.Name,
		CertProvider: opt.CertProvider,
		http:         httpSer,
		https:        httpsSer,
		l:            logger,
		acl:          opt.ACL,
		proxyProto:   opt.SupportProxyProtocol,
	}
}

// Start will start the http and https servers.
//
// If both are not set, this does nothing.
//
// Start() is non-blocking.
func (s *Server) Start(parent task.Parent, http3Enabled bool) {
	taskName := func(proto string) string {
		return "server." + s.Name + "." + proto
	}
	s.startTime = time.Now()

	if s.https != nil && http3Enabled {
		if s.proxyProto {
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
			Start(subtask, h3, WithProxyProtocolSupport(s.proxyProto), WithACL(s.acl), WithLogger(&s.l))
			if s.http != nil {
				s.http.Handler = advertiseHTTP3(s.http.Handler, h3)
			}
			// s.https is not nil (checked above)
			s.https.Handler = advertiseHTTP3(s.https.Handler, h3)
		}
	}

	Start(parent.Subtask(taskName("http"), true), s.http, WithProxyProtocolSupport(s.proxyProto), WithACL(s.acl), WithLogger(&s.l))
	Start(parent.Subtask(taskName("https"), true), s.https, WithProxyProtocolSupport(s.proxyProto), WithACL(s.acl), WithLogger(&s.l))
}

type ServerStartOptions struct {
	tcpWrappers []func(l net.Listener) net.Listener
	udpWrappers []func(l net.PacketConn) net.PacketConn
	logger      *zerolog.Logger
	proxyProto  bool
}

type ServerStartOption func(opts *ServerStartOptions)

func WithTCPWrappers(wrappers ...TCPWrapper) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.tcpWrappers = wrappers
	}
}

func WithUDPWrappers(wrappers ...UDPWrapper) ServerStartOption {
	return func(opts *ServerStartOptions) {
		opts.udpWrappers = wrappers
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

func Start[Server httpServer](parent task.Parent, srv Server, optFns ...ServerStartOption) (port int) {
	if srv == nil {
		return port
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
	task := parent.Subtask(proto, true)

	var lc net.ListenConfig
	var serveFunc func() error

	switch srv := any(srv).(type) {
	case *http.Server:
		srv.BaseContext = func(l net.Listener) context.Context {
			return parent.Context()
		}
		l, err := lc.Listen(task.Context(), "tcp", srv.Addr)
		if err != nil {
			HandleError(opts.logger, err, "failed to listen on port")
			return port
		}
		port = l.Addr().(*net.TCPAddr).Port
		if opts.proxyProto {
			l = &proxyproto.Listener{Listener: l}
		}
		if srv.TLSConfig != nil {
			l = tls.NewListener(l, srv.TLSConfig)
		}
		for _, wrapper := range opts.tcpWrappers {
			l = wrapper(l)
		}
		if opts.proxyProto {
			serveFunc = getServeFunc(l, h2proxy.NewServer(srv, nil).Serve)
		} else {
			serveFunc = getServeFunc(l, srv.Serve)
		}
		task.OnCancel("stop", func() {
			stop(srv, l, proto, opts.logger)
		})
	case *http3.Server:
		l, err := lc.ListenPacket(task.Context(), "udp", srv.Addr)
		if err != nil {
			HandleError(opts.logger, err, "failed to listen on port")
			return port
		}
		port = l.LocalAddr().(*net.UDPAddr).Port
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
			HandleError(opts.logger, err, "failed to serve "+proto+" server")
		}
		task.Finish(err)
	}()
	return port
}

func stop[Server httpServer](srv Server, l io.Closer, proto string, logger *zerolog.Logger) {
	if srv == nil {
		return
	}

	ctx, cancel := context.WithTimeout(task.RootContext(), 1*time.Second)
	defer cancel()

	if err := convertError(errors.Join(srv.Shutdown(ctx), l.Close())); err != nil {
		HandleError(logger, err, "failed to shutdown "+proto+" server")
	} else {
		logger.Info().Str("proto", proto).Str("addr", addr(srv)).Msg("server stopped")
	}
}

func (s *Server) Uptime() time.Duration {
	return time.Since(s.startTime)
}
