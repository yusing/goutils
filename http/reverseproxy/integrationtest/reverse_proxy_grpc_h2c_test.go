package integrationtest

import (
	"context"
	stdjson "encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	reverseproxy "github.com/yusing/goutils/http/reverseproxy"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

func TestReverseProxyH2CGRPCUnaryIntegration(t *testing.T) {
	conn := startGRPCOverH2CProxy(t)
	defer conn.Close()

	var reply testGRPCMessage
	var header, trailer metadata.MD
	err := conn.Invoke(
		t.Context(),
		"/test.Echo/Unary",
		&testGRPCMessage{Value: "hello"},
		&reply,
		grpc.Header(&header),
		grpc.Trailer(&trailer),
	)
	if err != nil {
		t.Fatalf("invoke unary: %v", err)
	}

	if reply.Value != "echo:hello" {
		t.Fatalf("reply value = %q, want echo:hello", reply.Value)
	}
	if got := header.Get("x-test-header"); len(got) != 1 || got[0] != "proxied" {
		t.Fatalf("header x-test-header = %v, want [proxied]", got)
	}
	if got := trailer.Get("x-test-trailer"); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("trailer x-test-trailer = %v, want [ok]", got)
	}
}

func TestReverseProxyH2CGRPCBidiStreamIntegration(t *testing.T) {
	conn := startGRPCOverH2CProxy(t)
	defer conn.Close()

	stream, err := conn.NewStream(t.Context(), &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.Echo/Chat")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}

	for _, value := range []string{"one", "two"} {
		if err := stream.SendMsg(&testGRPCMessage{Value: value}); err != nil {
			t.Fatalf("send %q: %v", value, err)
		}

		var reply testGRPCMessage
		if err := stream.RecvMsg(&reply); err != nil {
			t.Fatalf("recv %q: %v", value, err)
		}
		if reply.Value != "echo:"+value {
			t.Fatalf("reply value = %q, want %q", reply.Value, "echo:"+value)
		}
	}

	header, err := stream.Header()
	if err != nil {
		t.Fatalf("stream header: %v", err)
	}
	if got := header.Get("x-stream-header"); len(got) != 1 || got[0] != "proxied" {
		t.Fatalf("stream header x-stream-header = %v, want [proxied]", got)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("close send: %v", err)
	}

	var reply testGRPCMessage
	err = stream.RecvMsg(&reply)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("final recv err = %v, want EOF", err)
	}

	if got := stream.Trailer().Get("x-stream-trailer"); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("stream trailer x-stream-trailer = %v, want [ok]", got)
	}
}

func TestReverseProxyH2CGRPCHeaderOnlyBidiStreamIntegration(t *testing.T) {
	conn := startGRPCOverH2CProxy(t)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}, "/test.Echo/HeaderOnly")
	if err != nil {
		t.Fatalf("new stream: %v", err)
	}
	defer stream.CloseSend()

	header, err := stream.Header()
	if err != nil {
		t.Fatalf("stream header: %v", err)
	}
	if got := header.Get("x-stream-header"); len(got) != 1 || got[0] != "ready" {
		t.Fatalf("stream header x-stream-header = %v, want [ready]", got)
	}
}

func startGRPCOverH2CProxy(t *testing.T) *grpc.ClientConn {
	t.Helper()

	grpcServer := grpc.NewServer(grpc.ForceServerCodec(testJSONCodec{}))
	grpcServer.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.Echo",
		Methods: []grpc.MethodDesc{
			{
				MethodName: "Unary",
				Handler:    testGRPCUnaryHandler,
			},
		},
		Streams: []grpc.StreamDesc{
			{
				StreamName:    "Chat",
				Handler:       testGRPCChatHandler,
				ServerStreams: true,
				ClientStreams: true,
			},
			{
				StreamName:    "HeaderOnly",
				Handler:       testGRPCHeaderOnlyHandler,
				ServerStreams: true,
				ClientStreams: true,
			},
		},
	}, nil)
	t.Cleanup(grpcServer.Stop)

	backend := httptest.NewUnstartedServer(h2c.NewHandler(grpcServer, &http2.Server{}))
	backend.Start()
	t.Cleanup(backend.Close)

	target, err := url.Parse("h2c://" + backend.Listener.Addr().String())
	if err != nil {
		t.Fatalf("parse grpc backend target: %v", err)
	}
	rp := reverseproxy.NewReverseProxy("test-grpc-integration", target, &http.Transport{})

	proxy := httptest.NewUnstartedServer(h2c.NewHandler(rp, &http2.Server{}))
	proxy.Start()
	t.Cleanup(proxy.Close)

	conn, err := grpc.NewClient(
		"passthrough:///"+proxy.Listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(testJSONCodec{})),
	)
	if err != nil {
		t.Fatalf("new grpc client: %v", err)
	}
	return conn
}

type testGRPCMessage struct {
	Value string `json:"value"`
}

type testJSONCodec struct{}

func (testJSONCodec) Name() string {
	return "json"
}

func (testJSONCodec) Marshal(v any) ([]byte, error) {
	return stdjson.Marshal(v)
}

func (testJSONCodec) Unmarshal(data []byte, v any) error {
	return stdjson.Unmarshal(data, v)
}

func testGRPCUnaryHandler(_ any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
	var req testGRPCMessage
	if err := dec(&req); err != nil {
		return nil, err
	}

	handler := func(ctx context.Context, req any) (any, error) {
		if err := grpc.SetHeader(ctx, metadata.Pairs("x-test-header", "proxied")); err != nil {
			return nil, err
		}
		grpc.SetTrailer(ctx, metadata.Pairs("x-test-trailer", "ok"))
		return &testGRPCMessage{Value: "echo:" + req.(*testGRPCMessage).Value}, nil
	}

	if interceptor == nil {
		return handler(ctx, &req)
	}
	return interceptor(ctx, &req, &grpc.UnaryServerInfo{
		FullMethod: "/test.Echo/Unary",
	}, handler)
}

func testGRPCChatHandler(_ any, stream grpc.ServerStream) error {
	if err := stream.SetHeader(metadata.Pairs("x-stream-header", "proxied")); err != nil {
		return err
	}

	for {
		var req testGRPCMessage
		err := stream.RecvMsg(&req)
		if errors.Is(err, io.EOF) {
			stream.SetTrailer(metadata.Pairs("x-stream-trailer", "ok"))
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.SendMsg(&testGRPCMessage{Value: "echo:" + req.Value}); err != nil {
			return err
		}
	}
}

func testGRPCHeaderOnlyHandler(_ any, stream grpc.ServerStream) error {
	if err := stream.SendHeader(metadata.Pairs("x-stream-header", "ready")); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}
