package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yusing/goutils/task"
)

type regressionCertProvider struct {
	cert *tls.Certificate
	err  error
}

func (p regressionCertProvider) GetCert(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return p.cert, p.err
}
func regressionCertificate(t *testing.T) *tls.Certificate {
	t.Helper()
	s := httptest.NewTLSServer(http.NotFoundHandler())
	defer s.Close()
	return &s.TLS.Certificates[0]
}
func regressionListener(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l
}

func TestTLSProxyProtocolHTTP2Regression(t *testing.T) {
	for _, h3 := range []bool{false, true} {
		t.Run(fmt.Sprint(h3), func(t *testing.T) {
			l := regressionListener(t)
			parent := task.GetTestTask(t).Subtask("tls-proxy", true)
			t.Cleanup(func() { parent.FinishAndWait(nil) })
			s := NewServer(Options{HTTPSAddr: l.Addr().String(), HTTPSListener: l, CertProvider: regressionCertProvider{cert: regressionCertificate(t)}, SupportProxyProtocol: true, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "ok") })})
			if err := s.Start(parent, h3); err != nil {
				t.Fatal(err)
			}
			tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, ForceAttemptHTTP2: true, DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				c, err := (&net.Dialer{}).DialContext(ctx, network, addr)
				if err != nil {
					return nil, err
				}
				if _, err = io.WriteString(c, "PROXY TCP4 192.0.2.1 127.0.0.1 12345 443\r\n"); err != nil {
					_ = c.Close()
					return nil, err
				}
				return c, nil
			}}
			defer tr.CloseIdleConnections()
			resp, err := (&http.Client{Transport: tr, Timeout: 2 * time.Second}).Get("https://" + l.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "ok" || resp.ProtoMajor != 2 {
				t.Fatalf("response = %q, proto %s, error %v", body, resp.Proto, err)
			}
		})
	}
}

func TestCertificateFailureStartsNoListenersRegression(t *testing.T) {
	failure := errors.New("certificate unavailable")
	for _, provider := range []CertProvider{nil, regressionCertProvider{err: failure}} {
		for _, h3 := range []bool{false, true} {
			t.Run(fmt.Sprintf("%T/%v", provider, h3), func(t *testing.T) {
				l := regressionListener(t)
				tracked := &regressionTrackedListener{Listener: l}
				parent := task.GetTestTask(t).Subtask("bad-cert", true)
				t.Cleanup(func() { parent.FinishAndWait(nil) })
				s := NewServer(Options{HTTPAddr: l.Addr().String(), HTTPListener: tracked, HTTPSAddr: "127.0.0.1:0", CertProvider: provider, Handler: http.NotFoundHandler()})
				err := s.Start(parent, h3)
				if err == nil {
					t.Fatal("missing/failing cert provider accepted")
				}
				if provider != nil && !errors.Is(err, failure) {
					t.Errorf("Start = %v, want provider error", err)
				}
				if tracked.accepted.Load() != 0 {
					t.Errorf("HTTP listener started before certificate validation")
				}
				if parent.Context().Err() != nil {
					t.Error("parent canceled")
				}
			})
		}
	}
}

type regressionTrackedListener struct {
	net.Listener
	accepted atomic.Int32
	closed   atomic.Bool
}

func (l *regressionTrackedListener) Accept() (net.Conn, error) {
	l.accepted.Add(1)
	return l.Listener.Accept()
}
func (l *regressionTrackedListener) Close() error { l.closed.Store(true); return l.Listener.Close() }

func TestPartialStartRollsBackRegression(t *testing.T) {
	cert := regressionCertificate(t)
	t.Run("http then https", func(t *testing.T) {
		busy := regressionListener(t)
		l := &regressionTrackedListener{Listener: regressionListener(t)}
		parent := task.GetTestTask(t).Subtask("rollback", true)
		t.Cleanup(func() { parent.FinishAndWait(nil) })
		s := NewServer(Options{HTTPAddr: l.Addr().String(), HTTPListener: l, HTTPSAddr: busy.Addr().String(), CertProvider: regressionCertProvider{cert: cert}, Handler: http.NotFoundHandler()})
		if err := s.Start(parent, false); err == nil {
			t.Fatal("expected HTTPS listen failure")
		}
		if !l.closed.Load() {
			t.Error("started HTTP listener not closed before Start returned")
		}
		if parent.Context().Err() != nil {
			t.Error("parent canceled")
		}
	})
	t.Run("http3 then http", func(t *testing.T) {
		busy := regressionListener(t)
		// Reserve a UDP port, then release it immediately before Start.
		udp, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		addr := udp.LocalAddr().String()
		_ = udp.Close()
		parent := task.GetTestTask(t).Subtask("rollback-h3", true)
		t.Cleanup(func() { parent.FinishAndWait(nil) })
		s := NewServer(Options{HTTPAddr: busy.Addr().String(), HTTPSAddr: addr, CertProvider: regressionCertProvider{cert: cert}, Handler: http.NotFoundHandler()})
		if err := s.Start(parent, true); err == nil {
			t.Fatal("expected HTTP listen failure")
		}
		rebound, err := net.ListenPacket("udp", addr)
		if err != nil {
			t.Errorf("HTTP3 UDP listener still bound: %v", err)
		} else {
			_ = rebound.Close()
		}
		if parent.Context().Err() != nil {
			t.Error("parent canceled")
		}
	})
}

type regressionACL struct{ tcp, udp int }

func (a *regressionACL) WrapTCP(l net.Listener) net.Listener     { a.tcp++; return l }
func (a *regressionACL) WrapUDP(l net.PacketConn) net.PacketConn { a.udp++; return l }
func TestWrapperOptionsAppendRegression(t *testing.T) {
	for _, aclFirst := range []bool{false, true} {
		t.Run(fmt.Sprint(aclFirst), func(t *testing.T) {
			acl := &regressionACL{}
			tcp, udp := 0, 0
			options := []ServerStartOption{WithTCPWrappers(func(l net.Listener) net.Listener { tcp++; return l }), WithUDPWrappers(func(l net.PacketConn) net.PacketConn { udp++; return l })}
			if aclFirst {
				options = append([]ServerStartOption{WithACL(acl)}, options...)
			} else {
				options = append(options, WithACL(acl))
			}
			var opts ServerStartOptions
			for _, option := range options {
				option(&opts)
			}
			for _, wrap := range opts.tcpWrappers {
				wrap(nil)
			}
			for _, wrap := range opts.udpWrappers {
				wrap(nil)
			}
			if tcp != 1 || udp != 1 || acl.tcp != 1 || acl.udp != 1 {
				t.Fatalf("wrappers called custom=(%d,%d) ACL=(%d,%d)", tcp, udp, acl.tcp, acl.udp)
			}
		})
	}
}

type regressionNonTCPListener struct{ net.Listener }

func (l regressionNonTCPListener) Addr() net.Addr { return &net.UnixAddr{Name: "fake", Net: "unix"} }
func TestStartRejectsNonTCPListenerRegression(t *testing.T) {
	parent := task.GetTestTask(t).Subtask("non-tcp", true)
	defer parent.FinishAndWait(nil)
	if _, err := Start(parent, &http.Server{Handler: http.NotFoundHandler()}, WithListener(regressionNonTCPListener{regressionListener(t)})); err == nil {
		t.Fatal("non-TCP listener accepted")
	}
}
