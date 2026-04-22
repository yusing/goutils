package reverseproxy

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type errorTransport struct {
	err error
}

func (t errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

func newTestReverseProxy(t *testing.T, transport http.RoundTripper) *ReverseProxy {
	t.Helper()

	targetURL, err := url.Parse("http://origin.internal")
	if err != nil {
		t.Fatalf("parse target url: %v", err)
	}

	return NewReverseProxy("test", targetURL, transport)
}

func TestReverseProxyReturnsRetryPageForHTMLRequests(t *testing.T) {
	t.Parallel()

	rp := newTestReverseProxy(t, errorTransport{err: errors.New("dial tcp: connect: connection refused")})
	req := httptest.NewRequest(http.MethodGet, "http://proxy.local/", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	rp.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusBadGateway)
	}
	if got := res.Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Fatalf("content type = %q, want %q", got, "text/html; charset=utf-8")
	}
	if got := res.Header.Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, max-age=0" {
		t.Fatalf("cache-control = %q", got)
	}
	if got := res.Header.Get("Pragma"); got != "no-cache" {
		t.Fatalf("pragma = %q", got)
	}
	if got := res.Header.Get("Expires"); got != "0" {
		t.Fatalf("expires = %q", got)
	}
	bodyText := string(body)
	for _, want := range []string{
		"Origin server is not reachable.",
		"Retrying in",
		"countdown",
		"location.reload()",
	} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("body does not contain %q: %s", want, bodyText)
		}
	}
}

func TestReverseProxyKeepsPlainTextForNonPageRequests(t *testing.T) {
	t.Parallel()

	rp := newTestReverseProxy(t, errorTransport{err: errors.New("dial tcp: connect: connection refused")})
	req := httptest.NewRequest(http.MethodPost, "http://proxy.local/submit", strings.NewReader("name=test"))
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	rp.ServeHTTP(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status code = %d, want %d", res.StatusCode, http.StatusBadGateway)
	}
	if got := res.Header.Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("content type = %q, want %q", got, "text/plain; charset=utf-8")
	}
	bodyText := string(body)
	if bodyText != "Origin server is not reachable." {
		t.Fatalf("body = %q, want %q", bodyText, "Origin server is not reachable.")
	}
	if strings.Contains(bodyText, "<!doctype html>") {
		t.Fatalf("plain text body unexpectedly contains html: %s", bodyText)
	}
}
