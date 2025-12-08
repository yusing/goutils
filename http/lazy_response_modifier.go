package httputils

import (
	"bufio"
	"net"
	"net/http"
)

// LazyResponseModifier wraps http.ResponseWriter and only buffers responses
// that need modification (determined by the shouldBuffer callback).
// For responses that don't need buffering (e.g., video streams), it passes
// through directly to avoid memory overhead.
type LazyResponseModifier struct {
	w            http.ResponseWriter
	shouldBuffer func(http.Header) bool

	decided bool

	// Used in buffered mode (rm != nil means buffered)
	rm *ResponseModifier

	// Used in passthrough mode
	statusCode int
}

// NewLazyResponseModifier creates a new LazyResponseModifier.
// shouldBuffer is called when WriteHeader is invoked to determine if the response
// should be buffered for modification.
func NewLazyResponseModifier(w http.ResponseWriter, shouldBuffer func(http.Header) bool) *LazyResponseModifier {
	return &LazyResponseModifier{
		w:            w,
		shouldBuffer: shouldBuffer,
	}
}

func (lrm *LazyResponseModifier) Header() http.Header {
	if lrm.rm != nil {
		return lrm.rm.Header()
	}
	return lrm.w.Header()
}

func (lrm *LazyResponseModifier) WriteHeader(code int) {
	if !lrm.decided {
		lrm.decide()
	}

	if lrm.rm != nil {
		lrm.rm.WriteHeader(code)
	} else {
		lrm.statusCode = code
		lrm.w.WriteHeader(code)
	}
}

func (lrm *LazyResponseModifier) Write(b []byte) (int, error) {
	if !lrm.decided {
		lrm.decide()
	}

	if lrm.rm != nil {
		return lrm.rm.Write(b)
	}
	return lrm.w.Write(b)
}

// decide determines whether to buffer based on content-type header.
// Must be called before first write.
func (lrm *LazyResponseModifier) decide() {
	lrm.decided = true
	if lrm.shouldBuffer(lrm.w.Header()) {
		lrm.rm = NewResponseModifier(lrm.w)
	}
}

// IsBuffered returns true if the response was buffered for modification.
func (lrm *LazyResponseModifier) IsBuffered() bool {
	return lrm.rm != nil
}

// ResponseModifier returns the underlying ResponseModifier if buffered.
// Returns nil if not buffered.
func (lrm *LazyResponseModifier) ResponseModifier() *ResponseModifier {
	return lrm.rm
}

// FlushRelease flushes the response and releases resources.
func (lrm *LazyResponseModifier) FlushRelease() (int, error) {
	if lrm.rm != nil {
		return lrm.rm.FlushRelease()
	}
	return 0, nil
}

// Hijack implements http.Hijacker.
func (lrm *LazyResponseModifier) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if lrm.rm != nil {
		return lrm.rm.Hijack()
	}
	if hijacker, ok := lrm.w.(http.Hijacker); ok {
		return hijacker.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

// Unwrap returns the underlying ResponseWriter.
func (lrm *LazyResponseModifier) Unwrap() http.ResponseWriter {
	return lrm.w
}
