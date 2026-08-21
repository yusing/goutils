package httputils

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
)

func TestIsUnexpectedError(t *testing.T) {
	expected := map[string]error{
		"stream closed":        errStreamClosed,
		"client disconnected":  errClientDisconnected,
		"response body closed": errClosedResponseBody,
	}
	for name, err := range expected {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, err)
			require.False(t, IsUnexpectedError(err))
			require.False(t, IsUnexpectedError(fmt.Errorf("wrapped: %w", err)))
		})
	}

	require.False(t, IsUnexpectedError(http2.StreamError{Code: http2.ErrCodeCancel}))
	require.True(t, IsUnexpectedError(errors.New("ordinary error")))
}

type flushErrorResponseWriter struct {
	header http.Header
	body   bytes.Buffer
	err    error
}

func (w *flushErrorResponseWriter) Header() http.Header {
	return w.header
}

func (w *flushErrorResponseWriter) Write(p []byte) (int, error) {
	return w.body.Write(p)
}

func (*flushErrorResponseWriter) WriteHeader(int) {}

func (w *flushErrorResponseWriter) FlushError() error {
	return w.err
}

func TestResponseModifierFlushErrorClassification(t *testing.T) {
	tests := []struct {
		name     string
		flushErr error
		wantErr  string
	}{
		{name: "expected stream close", flushErr: errStreamClosed},
		{name: "unexpected failure", flushErr: errors.New("flush failed"), wantErr: "flush error: flush failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &flushErrorResponseWriter{header: make(http.Header), err: tt.flushErr}
			rm := NewPassthroughResponseModifier(w)

			_, err := rm.Write([]byte("favicon"))
			require.NoError(t, err)
			_, err = rm.FlushRelease()
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.EqualError(t, err, tt.wantErr)
			}
			require.Equal(t, "favicon", w.body.String())
		})
	}
}
