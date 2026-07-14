package ioutils

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type closeBuffer struct {
	bytes.Buffer
	closes int
}

func (b *closeBuffer) Close() error {
	b.closes++
	return nil
}

func TestBufferedWriterResizeClose(t *testing.T) {
	dst := new(closeBuffer)
	w := NewBufferedWriter(dst, 8)
	if _, err := w.WriteString("before"); err != nil {
		t.Fatal(err)
	}
	if err := w.Resize(32); err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("after"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if got, want := dst.String(), "beforeafter"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if dst.closes != 1 {
		t.Fatalf("underlying closes = %d, want 1", dst.closes)
	}
	if w.Size() != 0 {
		t.Fatalf("size after close = %d, want 0", w.Size())
	}

	if err := w.WriteByte('x'); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("WriteByte after close error = %v, want ErrClosedPipe", err)
	}
	if _, err := w.WriteString("x"); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("WriteString after close error = %v, want ErrClosedPipe", err)
	}
	if _, err := w.Write([]byte("x")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Write after close error = %v, want ErrClosedPipe", err)
	}
	if err := w.Close(); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("second Close error = %v, want ErrClosedPipe", err)
	}
	if dst.closes != 1 {
		t.Fatalf("underlying closes after second Close = %d, want 1", dst.closes)
	}
}

type closeErrorWriter struct {
	writeErr error
	closeErr error
}

func (w closeErrorWriter) Write(p []byte) (int, error) { return len(p), w.writeErr }
func (w closeErrorWriter) Close() error                { return w.closeErr }

func TestBufferedWriterCloseReturnsStickyAndCloseErrors(t *testing.T) {
	writeErr := errors.New("write failed")
	closeErr := errors.New("close failed")
	w := NewBufferedWriter(closeErrorWriter{writeErr: writeErr, closeErr: closeErr}, 8)
	if _, err := w.Write(make([]byte, 9)); !errors.Is(err, writeErr) {
		t.Fatalf("Write error = %v, want %v", err, writeErr)
	}
	if err := w.Close(); !errors.Is(err, writeErr) || !errors.Is(err, closeErr) {
		t.Fatalf("Close error = %v, want both %v and %v", err, writeErr, closeErr)
	}
}

func TestBufferedWriterNonPositiveSizeUsesDefault(t *testing.T) {
	for _, size := range []int{0, -1} {
		w := NewBufferedWriter(io.Discard, size)
		if got := w.Size(); got != defaultBufferSize {
			t.Fatalf("NewBufferedWriter size %d: buffer size = %d, want %d", size, got, defaultBufferSize)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}

	w := NewBufferedWriter(io.Discard, 8)
	for _, size := range []int{0, -1} {
		if err := w.Resize(size); err != nil {
			t.Fatal(err)
		}
		if got := w.Size(); got != defaultBufferSize {
			t.Fatalf("Resize(%d): buffer size = %d, want %d", size, got, defaultBufferSize)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestBufferedWriterNonPositiveSizeReturnsExistingWriter(t *testing.T) {
	for _, size := range []int{0, -1} {
		var dst bytes.Buffer
		w := NewBufferedWriter(&dst, 8)
		got := NewBufferedWriter(w, size)
		if got != w {
			t.Fatalf("NewBufferedWriter with size %d returned a nested writer", size)
		}
		if _, err := got.WriteString("output"); err != nil {
			t.Fatal(err)
		}
		if err := got.Flush(); err != nil {
			t.Fatal(err)
		}
		if dst.String() != "output" {
			t.Fatalf("size %d: output = %q, want %q", size, dst.String(), "output")
		}
		if err := got.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBufferedWriterAvailableBufferCapacity(t *testing.T) {
	w := NewBufferedWriter(io.Discard, 3000)
	defer w.Close()
	if got, want := cap(w.AvailableBuffer()), w.Available(); got != want {
		t.Fatalf("AvailableBuffer capacity = %d, want %d", got, want)
	}
}

func TestBufferedWriterPooledBufferCleared(t *testing.T) {
	const size = 4096
	buf := getBuffer(size)
	for i := range buf {
		buf[i] = 0xff
	}
	bytesPool.Put(buf)

	buf = getBuffer(size)
	defer bytesPool.Put(buf)
	for i, value := range buf {
		if value != 0 {
			t.Fatalf("pooled buffer byte %d = %#x, want 0", i, value)
		}
	}
}
