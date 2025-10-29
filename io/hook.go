package ioutils

import "io"

type HookReadCloser struct {
	c    io.ReadCloser
	hook func()
}

// NewHookReadCloser wraps a io.ReadCloser and calls the hook function when the closer is closed.
func NewHookReadCloser(c io.ReadCloser, hook func()) *HookReadCloser {
	return &HookReadCloser{hook: hook, c: c}
}

// Close calls the hook function and closes the underlying reader
func (r *HookReadCloser) Close() error {
	err := r.c.Close()
	r.hook()
	return err
}

// Read reads from the underlying reader.
func (r *HookReadCloser) Read(p []byte) (int, error) {
	return r.c.Read(p)
}
