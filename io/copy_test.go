package ioutils

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type flushCountingResponseWriter struct {
	header     http.Header
	writes     int
	flushCount int
}

func (w *flushCountingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *flushCountingResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func (w *flushCountingResponseWriter) WriteHeader(int) {}

func (w *flushCountingResponseWriter) Flush() {
	w.flushCount++
}

func TestCopyClose_FlushesEventStreamResponses(t *testing.T) {
	dst := &flushCountingResponseWriter{}
	dst.Header().Set("Content-Type", "text/event-stream")

	src := strings.NewReader("data: one\n\ndata: two\n\n")

	err := CopyClose(dst, src, 5)
	require.NoError(t, err)
	require.Positive(t, dst.writes)
	require.Equal(t, dst.writes, dst.flushCount)
}

func TestCopyClose_DoesNotFlushBufferedVideoResponses(t *testing.T) {
	dst := &flushCountingResponseWriter{}
	dst.Header().Set("Content-Type", "video/mp4")
	dst.Header().Set("Content-Length", "12")

	src := strings.NewReader("hello world!")

	err := CopyClose(dst, src, 4)
	require.NoError(t, err)
	require.Positive(t, dst.writes)
	require.Zero(t, dst.flushCount)
}

func TestCopyClose_FlushesChunkedResponsesWithoutContentLength(t *testing.T) {
	dst := &flushCountingResponseWriter{}
	dst.Header().Set("Content-Type", "application/octet-stream")
	dst.Header().Set("Transfer-Encoding", "chunked")

	src := io.NopCloser(strings.NewReader("chunked body"))

	err := CopyClose(dst, src, 3)
	require.NoError(t, err)
	require.Positive(t, dst.writes)
	require.Equal(t, dst.writes, dst.flushCount)
}

func TestCopyClose_FlushesStreamingResponsesWithoutLength(t *testing.T) {
	dst := &flushCountingResponseWriter{}
	dst.Header().Set("Content-Type", "application/octet-stream")

	src := strings.NewReader("streaming body")

	err := CopyClose(dst, src, -1)
	require.NoError(t, err)
	require.Positive(t, dst.writes)
	require.Equal(t, dst.writes, dst.flushCount)
}

func TestCopyClose_FlushesGRPCResponses(t *testing.T) {
	for _, contentType := range []string{"application/grpc", "application/grpc+proto"} {
		t.Run(contentType, func(t *testing.T) {
			dst := &flushCountingResponseWriter{}
			dst.Header().Set("Content-Type", contentType)
			dst.Header().Set("Content-Length", "12")

			src := strings.NewReader("hello world!")

			err := CopyClose(dst, src, 4)
			require.NoError(t, err)
			require.Positive(t, dst.writes)
			require.Equal(t, dst.writes, dst.flushCount)
		})
	}
}

type unsupportedFlushResponseWriter struct {
	flushCountingResponseWriter
}

func (w *unsupportedFlushResponseWriter) FlushError() error {
	w.flushCount++
	return http.ErrNotSupported
}

func (w *unsupportedFlushResponseWriter) Write(p []byte) (int, error) {
	w.writes++
	return len(p), nil
}

func TestCopyClose_IgnoresUnsupportedFlush(t *testing.T) {
	dst := &unsupportedFlushResponseWriter{}
	dst.Header().Set("Content-Type", "text/html")

	src := strings.NewReader("buffered body")

	err := CopyClose(dst, src, -1)
	require.NoError(t, err)
	require.Positive(t, dst.writes)
	require.Equal(t, dst.writes, dst.flushCount)
}

func TestCopyClose_IgnoresUnsupportedFlushForGRPCAndSSE(t *testing.T) {
	for _, contentType := range []string{"application/grpc", "application/grpc+proto", "text/event-stream"} {
		t.Run(contentType, func(t *testing.T) {
			dst := &unsupportedFlushResponseWriter{}
			dst.Header().Set("Content-Type", contentType)

			src := strings.NewReader("buffered body")

			err := CopyClose(dst, src, -1)
			require.NoError(t, err)
			require.Positive(t, dst.writes)
			require.Equal(t, dst.writes, dst.flushCount)
		})
	}
}
