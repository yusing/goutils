package httputils

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type noopResponseWriter struct {
	header http.Header
}

func (w noopResponseWriter) Header() http.Header {
	return w.header
}

func (noopResponseWriter) Write(b []byte) (int, error) {
	return len(b), nil
}

func (noopResponseWriter) WriteHeader(int) {}

func TestModifyResponseWriterWriteHeaderProvidesEmptyBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	var gotStatusCode int
	var gotContentLength int64
	var gotBody string
	mrw := NewModifyResponseWriter(rec, req, func(resp *http.Response) error {
		gotStatusCode = resp.StatusCode
		gotContentLength = resp.ContentLength
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read modifier body: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatalf("close modifier body: %v", err)
		}
		gotBody = string(body)
		return nil
	})

	mrw.WriteHeader(http.StatusCreated)

	if gotStatusCode != http.StatusCreated {
		t.Fatalf("modifier status code = %d, want %d", gotStatusCode, http.StatusCreated)
	}
	if gotContentLength != 0 {
		t.Fatalf("modifier content length = %d, want 0", gotContentLength)
	}
	if gotBody != "" {
		t.Fatalf("modifier body = %q, want empty", gotBody)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("recorder status code = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func BenchmarkModifyResponseWriterWriteHeader(b *testing.B) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	header := make(http.Header)
	w := noopResponseWriter{header: header}
	modifier := func(*http.Response) error { return nil }

	b.ReportAllocs()
	for b.Loop() {
		clear(header)
		mrw := NewModifyResponseWriter(w, req, modifier)
		mrw.WriteHeader(http.StatusNoContent)
	}
}
