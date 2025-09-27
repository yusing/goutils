package ioutils

import (
	"context"
	"io"
)

type (
	ContextReader struct {
		ctx context.Context
		io.Reader
	}

	ContextWriter struct {
		ctx context.Context
		io.Writer
	}
)

func NewContextReader(ctx context.Context, r io.Reader) *ContextReader {
	return &ContextReader{ctx: ctx, Reader: r}
}

func NewContextWriter(ctx context.Context, w io.Writer) *ContextWriter {
	return &ContextWriter{ctx: ctx, Writer: w}
}

func (r *ContextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.Reader.Read(p)
	}
}

func (w *ContextWriter) Write(p []byte) (int, error) {
	select {
	case <-w.ctx.Done():
		return 0, w.ctx.Err()
	default:
		return w.Writer.Write(p)
	}
}
