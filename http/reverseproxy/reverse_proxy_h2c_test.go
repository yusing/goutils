package reverseproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
