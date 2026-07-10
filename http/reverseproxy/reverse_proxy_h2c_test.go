package reverseproxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestH2CRoundTripper_UsesHTTP2(t *testing.T) {
	got := make(chan int, 1)

	srv := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.ProtoMajor
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}), &http2.Server{}))
	srv.Start()
	defer srv.Close()

	rt := newH2CRoundTripper(&http.Transport{})
	client := &http.Client{Transport: rt}

	resp, err := client.Get(srv.URL + "/ping")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	select {
	case proto := <-got:
		if proto != 2 {
			t.Fatalf("backend proto major = %d, want 2", proto)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for backend request")
	}
}

func TestReverseProxy_H2C_Scheme(t *testing.T) {
	type info struct {
		protoMajor int
		path       string
		rawQuery   string
	}

	got := make(chan info, 1)

	srv := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- info{
			protoMajor: r.ProtoMajor,
			path:       r.URL.Path,
			rawQuery:   r.URL.RawQuery,
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "proxied")
	}), &http2.Server{}))
	srv.Start()
	defer srv.Close()

	target, err := url.Parse("h2c://" + srv.Listener.Addr().String() + "/base")
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	rp := NewReverseProxy("test-h2c", target, &http.Transport{})

	req := httptest.NewRequest(http.MethodGet, "http://proxy.local/hello?x=1", nil)
	rr := httptest.NewRecorder()
	rp.ServeHTTP(rr, req)

	res := rr.Result()
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	select {
	case v := <-got:
		if v.protoMajor != 2 {
			t.Fatalf("backend proto major = %d, want 2", v.protoMajor)
		}
		if v.path != "/base/hello" {
			t.Fatalf("backend path = %q, want %q", v.path, "/base/hello")
		}
		if v.rawQuery != "x=1" {
			t.Fatalf("backend query = %q, want %q", v.rawQuery, "x=1")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for backend request")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestReverseProxySchemeRetryDoesNotMutateTargetURL(t *testing.T) {
	target, err := url.Parse("http://backend.local/base")
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	var gotURLs []string
	transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		gotURLs = append(gotURLs, req.URL.String())
		if len(gotURLs) == 1 {
			return nil, http.ErrSchemeMismatch
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
			Request:    req,
		}, nil
	})

	rp := NewReverseProxy("test-scheme-retry", target, transport)
	rp.OnSchemeMisMatch = func(currentScheme string) (string, bool) {
		if currentScheme != "http" {
			t.Fatalf("retry current scheme = %q, want http", currentScheme)
		}
		return "https", true
	}

	req := httptest.NewRequest(http.MethodGet, "http://proxy.local/hello?x=1", nil)
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	_, _ = io.ReadAll(res.Body)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if target.Scheme != "http" {
		t.Fatalf("target scheme = %q, want http", target.Scheme)
	}
	wantURLs := []string{
		"http://backend.local/base/hello?x=1",
		"https://backend.local/base/hello?x=1",
	}
	if len(gotURLs) != len(wantURLs) {
		t.Fatalf("round trips = %d, want %d (%v)", len(gotURLs), len(wantURLs), gotURLs)
	}
	for i, want := range wantURLs {
		if gotURLs[i] != want {
			t.Fatalf("round trip %d url = %q, want %q", i, gotURLs[i], want)
		}
	}
}

func TestReverseProxySchemeRetryBody(t *testing.T) {
	t.Run("replays recreatable body", func(t *testing.T) {
		target, err := url.Parse("http://backend.local")
		if err != nil { t.Fatal(err) }
		var bodies []string
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body, err := io.ReadAll(req.Body)
			if err != nil { t.Fatal(err) }
			bodies = append(bodies, string(body))
			if len(bodies) == 1 {
				return nil, http.ErrSchemeMismatch
			}
			return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: req}, nil
		})
		rp := NewReverseProxy("retry", target, transport)
		rp.OnSchemeMisMatch = func(string) (string, bool) { return "https", true }
		req := httptest.NewRequest(http.MethodPost, "http://proxy.local/", strings.NewReader("payload"))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("payload")), nil }
		rp.ServeHTTP(httptest.NewRecorder(), req)
		if !slices.Equal(bodies, []string{"payload", "payload"}) { t.Fatalf("bodies = %q", bodies) }
	})

	t.Run("does not replay non-recreatable body", func(t *testing.T) {
		target, err := url.Parse("http://backend.local")
		if err != nil { t.Fatal(err) }
		calls := 0
		transport := roundTripFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			_, _ = io.ReadAll(req.Body)
			return nil, http.ErrSchemeMismatch
		})
		rp := NewReverseProxy("retry", target, transport)
		rp.OnSchemeMisMatch = func(string) (string, bool) { return "https", true }
		rp.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "http://proxy.local/", strings.NewReader("payload")))
		if calls != 1 { t.Fatalf("round trips = %d, want 1", calls) }
	})
}

func TestH2CRoundTripper_AcceptsH2CSchemeAndRemovesUpgradeHeaders(t *testing.T) {
	got := make(chan http.Header, 1)

	srv := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("backend proto major = %d, want 2", r.ProtoMajor)
		}
		got <- r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}), &http2.Server{}))
	srv.Start()
	defer srv.Close()

	u, err := url.Parse("h2c://" + srv.Listener.Addr().String() + "/ping")
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Connection", "Upgrade, HTTP2-Settings")
	req.Header.Set("Upgrade", "h2c")
	req.Header.Set("HTTP2-Settings", "sentinel")
	req.Header.Set("Te", "trailers")

	resp, err := newH2CRoundTripper(&http.Transport{}).RoundTrip(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)

	select {
	case header := <-got:
		for _, name := range []string{"Connection", "Upgrade", "HTTP2-Settings"} {
			if value := header.Get(name); value != "" {
				t.Fatalf("backend header %s = %q, want removed", name, value)
			}
		}
		if gotTE := header.Get("Te"); gotTE != "trailers" {
			t.Fatalf("backend TE = %q, want trailers", gotTE)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for backend request")
	}
}

func TestReverseProxy_H2C_GRPCStyleTrailers(t *testing.T) {
	type info struct {
		protoMajor int
		method     string
		body       string
		te         string
	}

	got := make(chan info, 1)

	srv := httptest.NewUnstartedServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		got <- info{
			protoMajor: r.ProtoMajor,
			method:     r.Method,
			body:       string(body),
			te:         r.Header.Get("Te"),
		}

		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}), &http2.Server{}))
	srv.Start()
	defer srv.Close()

	target, err := url.Parse("h2c://" + srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	rp := NewReverseProxy("test-grpc-h2c", target, &http.Transport{})

	req := httptest.NewRequest(http.MethodPost, "http://proxy.local/pkg.Service/Unary", strings.NewReader("grpc-body"))
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Te", "trailers")
	rec := httptest.NewRecorder()
	rp.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if gotCT := res.Header.Get("Content-Type"); gotCT != "application/grpc" {
		t.Fatalf("content-type = %q, want application/grpc", gotCT)
	}
	if len(body) != 5 {
		t.Fatalf("response body length = %d, want 5", len(body))
	}
	if gotStatus := res.Trailer.Get("Grpc-Status"); gotStatus != "0" {
		t.Fatalf("grpc-status trailer = %q, want 0", gotStatus)
	}

	select {
	case v := <-got:
		if v.protoMajor != 2 {
			t.Fatalf("backend proto major = %d, want 2", v.protoMajor)
		}
		if v.method != http.MethodPost {
			t.Fatalf("backend method = %q, want POST", v.method)
		}
		if v.body != "grpc-body" {
			t.Fatalf("backend body = %q, want grpc-body", v.body)
		}
		if v.te != "trailers" {
			t.Fatalf("backend TE = %q, want trailers", v.te)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for backend request")
	}
}

type nonFlushingResponseWriter struct {
	http.ResponseWriter
}

func TestReverseProxy_GRPCResponseWithoutFlushSupport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	rp := NewReverseProxy("test-grpc-no-flush", target, &http.Transport{})

	req := httptest.NewRequest(http.MethodPost, "http://proxy.local/pkg.Service/Unary", strings.NewReader("grpc-body"))
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Te", "trailers")
	rec := httptest.NewRecorder()
	rp.ServeHTTP(nonFlushingResponseWriter{ResponseWriter: rec}, req)

	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if len(body) != 5 {
		t.Fatalf("response body length = %d, want 5", len(body))
	}
	if gotStatus := res.Trailer.Get("Grpc-Status"); gotStatus != "0" {
		t.Fatalf("grpc-status trailer = %q, want 0", gotStatus)
	}
}

type flushErrorResponseWriter struct {
	http.ResponseWriter
	err error
}

func (w flushErrorResponseWriter) FlushError() error {
	return w.err
}

func TestReverseProxy_GRPCResponseIgnoresUnsupportedHeaderFlush(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
	}))
	defer srv.Close()

	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	rp := NewReverseProxy("test-grpc-unsupported-flush", target, &http.Transport{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://proxy.local/pkg.Service/Unary", strings.NewReader("grpc-body"))
	req.Header.Set("Content-Type", "application/grpc")

	rp.ServeHTTP(flushErrorResponseWriter{ResponseWriter: rec, err: http.ErrNotSupported}, req)

	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("proxy status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if len(body) != 5 {
		t.Fatalf("response body length = %d, want 5", len(body))
	}
}

func TestFlushResponseHeadersForStreamingPropagatesFlushError(t *testing.T) {
	wantErr := errors.New("flush failed")
	rw := flushErrorResponseWriter{ResponseWriter: httptest.NewRecorder(), err: wantErr}
	rw.Header().Set("Content-Type", "application/grpc")

	err := flushResponseHeadersForStreaming(rw, http.StatusOK)
	if !errors.Is(err, wantErr) {
		t.Fatalf("flush error = %v, want %v", err, wantErr)
	}
}
