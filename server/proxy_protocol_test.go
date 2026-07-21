package server

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/pires/go-proxyproto"
)

func TestProxyProtocolPolicy(t *testing.T) {
	validHeader := proxyProtocolTestHeader(t)
	unknownFutureHeader := append([]byte("\r\n\r\n\x00\r\nQUIT\n"), 0x31, 0x11, 0, 0)

	tests := []struct {
		name         string
		legacy       bool
		config       ProxyProtocolConfig
		peerIP       string
		input        []byte
		wantPayload  []byte
		wantRemoteIP string
		wantErr      error
		wantAnyError bool
	}{
		{
			name:         "legacy raw client",
			legacy:       true,
			peerIP:       "192.0.2.10",
			input:        []byte("GET / HTTP/1.1\r\n\r\n"),
			wantPayload:  []byte("GET / HTTP/1.1\r\n\r\n"),
			wantRemoteIP: "192.0.2.10",
		},
		{
			name:         "legacy valid header",
			legacy:       true,
			peerIP:       "192.0.2.10",
			input:        append(validHeader, []byte("payload")...),
			wantPayload:  []byte("payload"),
			wantRemoteIP: "203.0.113.25",
		},
		{
			name: "mixed trusted proxy valid header",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeMixed,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:       "10.1.2.3",
			input:        append(validHeader, []byte("payload")...),
			wantPayload:  []byte("payload"),
			wantRemoteIP: "203.0.113.25",
		},
		{
			name: "mixed trusted proxy missing header",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeMixed,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:  "10.1.2.3",
			input:   []byte("GET / HTTP/1.1\r\n\r\n"),
			wantErr: proxyproto.ErrNoProxyProtocol,
		},
		{
			name: "mixed trusted proxy malformed header",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeMixed,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:       "10.1.2.3",
			input:        []byte("PROXY TCP4 not-an-ip 192.0.2.1 1234 443\r\npayload"),
			wantAnyError: true,
		},
		{
			name: "mixed untrusted raw client",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeMixed,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:       "192.0.2.10",
			input:        []byte("GET / HTTP/1.1\r\n\r\n"),
			wantPayload:  []byte("GET / HTTP/1.1\r\n\r\n"),
			wantRemoteIP: "192.0.2.10",
		},
		{
			name: "mixed untrusted forged header stays application data",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeMixed,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:       "192.0.2.10",
			input:        append(validHeader, []byte("payload")...),
			wantPayload:  append(validHeader, []byte("payload")...),
			wantRemoteIP: "192.0.2.10",
		},
		{
			name:    "legacy unrelated signature collision fails closed",
			legacy:  true,
			peerIP:  "192.0.2.10",
			input:   []byte("PROXYX is application data"),
			wantErr: proxyproto.ErrCantReadVersion1Header,
		},
		{
			name: "required trusted proxy unknown future version",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeRequired,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:  "10.1.2.3",
			input:   unknownFutureHeader,
			wantErr: proxyproto.ErrUnsupportedProtocolVersionAndCommand,
		},
		{
			name: "required trusted proxy valid header",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeRequired,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:       "10.1.2.3",
			input:        append(validHeader, []byte("payload")...),
			wantPayload:  []byte("payload"),
			wantRemoteIP: "203.0.113.25",
		},
		{
			name: "required trusted proxy missing header",
			config: ProxyProtocolConfig{
				Mode:           ProxyProtocolModeRequired,
				TrustedProxies: []string{"10.0.0.0/8"},
			},
			peerIP:  "10.1.2.3",
			input:   []byte("GET / HTTP/1.1\r\n\r\n"),
			wantErr: proxyproto.ErrNoProxyProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var policy ProxyProtocolPolicy
			if tt.legacy {
				policy = NewLegacyProxyProtocolPolicy()
			} else {
				var err error
				policy, err = NewProxyProtocolPolicy(tt.config)
				if err != nil {
					t.Fatalf("create policy: %v", err)
				}
			}

			conn := &proxyProtocolMemoryConn{
				reader: bytes.NewReader(tt.input),
				local:  tcpAddr("198.51.100.2"),
				remote: tcpAddr(tt.peerIP),
			}
			listener := policy.Wrap(&proxyProtocolSingleListener{conn: conn})
			accepted, err := listener.Accept()
			if err != nil {
				t.Fatalf("accept: %v", err)
			}

			payload, readErr := io.ReadAll(accepted)
			switch {
			case tt.wantErr != nil && !errors.Is(readErr, tt.wantErr):
				t.Fatalf("read error = %v, want %v", readErr, tt.wantErr)
			case tt.wantAnyError && readErr == nil:
				t.Fatal("read unexpectedly succeeded")
			case tt.wantErr == nil && !tt.wantAnyError && readErr != nil:
				t.Fatalf("read: %v", readErr)
			}
			if !bytes.Equal(payload, tt.wantPayload) {
				t.Fatalf("payload = %q, want %q", payload, tt.wantPayload)
			}
			if tt.wantRemoteIP != "" {
				got := accepted.RemoteAddr().(*net.TCPAddr).IP.String()
				if got != tt.wantRemoteIP {
					t.Fatalf("remote IP = %q, want %q", got, tt.wantRemoteIP)
				}
			}
		})
	}
}

func TestRequiredProxyProtocolPolicyRejectsUntrustedPeer(t *testing.T) {
	policy, err := NewProxyProtocolPolicy(ProxyProtocolConfig{
		Mode:           ProxyProtocolModeRequired,
		TrustedProxies: []string{"10.0.0.0/8"},
	})
	if err != nil {
		t.Fatalf("create policy: %v", err)
	}

	got, err := policy.connPolicy(proxyproto.ConnPolicyOptions{
		Upstream:   tcpAddr("192.0.2.10"),
		Downstream: tcpAddr("198.51.100.2"),
	})
	if got != proxyproto.REJECT {
		t.Fatalf("policy = %v, want REJECT", got)
	}
	if !errors.Is(err, proxyproto.ErrInvalidUpstream) {
		t.Fatalf("error = %v, want ErrInvalidUpstream", err)
	}
}

func TestNewProxyProtocolPolicyRejectsInvalidTrustedProxy(t *testing.T) {
	if _, err := NewProxyProtocolPolicy(ProxyProtocolConfig{
		Mode:           ProxyProtocolModeRequired,
		TrustedProxies: []string{"not-an-ip-or-cidr"},
	}); err == nil {
		t.Fatal("expected invalid trusted proxy error")
	}
}

func TestProxyProtocolConfigValidation(t *testing.T) {
	tests := []ProxyProtocolConfig{
		{},
		{Mode: "opportunistic", TrustedProxies: []string{"127.0.0.1"}},
		{Mode: ProxyProtocolModeRequired},
		{Mode: ProxyProtocolModeMixed},
	}
	for _, cfg := range tests {
		if err := cfg.Validate(); err == nil {
			t.Errorf("config %#v unexpectedly passed validation", cfg)
		}
	}

	disabled, err := NewProxyProtocolPolicy(ProxyProtocolConfig{Mode: ProxyProtocolModeDisabled})
	if err != nil {
		t.Fatalf("disabled config: %v", err)
	}
	if disabled.Enabled() {
		t.Fatal("disabled policy is enabled")
	}
	if err := (ProxyProtocolConfig{
		Mode:           ProxyProtocolModeDisabled,
		TrustedProxies: []string{"127.0.0.1"},
	}).Validate(); err != nil {
		t.Fatalf("disabled config should ignore trusted proxies: %v", err)
	}
}

func TestExplicitDisabledPolicyOverridesLegacyCompatibility(t *testing.T) {
	disabled, err := NewProxyProtocolPolicy(ProxyProtocolConfig{Mode: ProxyProtocolModeDisabled})
	if err != nil {
		t.Fatalf("disabled config: %v", err)
	}

	srv := NewServer(Options{
		SupportProxyProtocol: true,
		ProxyProtocolPolicy:  disabled,
	})
	if srv.proxyProtocolPolicy.Enabled() {
		t.Fatal("NewServer enabled legacy policy despite explicit disabled policy")
	}

	var opts ServerStartOptions
	WithProxyProtocolSupport(true)(&opts)
	WithProxyProtocolPolicy(disabled)(&opts)
	if resolved := resolveProxyProtocolPolicy(opts.proxyProtocolPolicy, opts.proxyProto); resolved.Enabled() {
		t.Fatal("Start options enabled legacy policy despite explicit disabled policy")
	}
}

func proxyProtocolTestHeader(t *testing.T) []byte {
	t.Helper()
	header := &proxyproto.Header{
		Version:           2,
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        &net.TCPAddr{IP: net.ParseIP("203.0.113.25"), Port: 43210},
		DestinationAddr:   &net.TCPAddr{IP: net.ParseIP("198.51.100.2"), Port: 443},
	}
	var buf bytes.Buffer
	if _, err := header.WriteTo(&buf); err != nil {
		t.Fatalf("format PROXY header: %v", err)
	}
	return buf.Bytes()
}

func tcpAddr(ip string) *net.TCPAddr {
	return &net.TCPAddr{IP: net.ParseIP(ip), Port: 12345}
}

type proxyProtocolSingleListener struct {
	conn net.Conn
}

func (l *proxyProtocolSingleListener) Accept() (net.Conn, error) {
	if l.conn == nil {
		return nil, net.ErrClosed
	}
	conn := l.conn
	l.conn = nil
	return conn, nil
}

func (*proxyProtocolSingleListener) Close() error   { return nil }
func (*proxyProtocolSingleListener) Addr() net.Addr { return tcpAddr("198.51.100.2") }

type proxyProtocolMemoryConn struct {
	reader *bytes.Reader
	writes bytes.Buffer
	local  net.Addr
	remote net.Addr
}

func (c *proxyProtocolMemoryConn) Read(p []byte) (int, error)     { return c.reader.Read(p) }
func (c *proxyProtocolMemoryConn) Write(p []byte) (int, error)    { return c.writes.Write(p) }
func (*proxyProtocolMemoryConn) Close() error                     { return nil }
func (c *proxyProtocolMemoryConn) LocalAddr() net.Addr            { return c.local }
func (c *proxyProtocolMemoryConn) RemoteAddr() net.Addr           { return c.remote }
func (*proxyProtocolMemoryConn) SetDeadline(time.Time) error      { return nil }
func (*proxyProtocolMemoryConn) SetReadDeadline(time.Time) error  { return nil }
func (*proxyProtocolMemoryConn) SetWriteDeadline(time.Time) error { return nil }
