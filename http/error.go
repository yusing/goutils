package httputils

import (
	"errors"
	"reflect"

	"golang.org/x/net/http2"

	_ "unsafe"
)

// Go 1.27's x/net/http2 delegates server and transport behavior to the
// standard library, so expected connection-close errors originate there.
//
//go:linkname errStreamClosed net/http/internal/http2.errStreamClosed
var errStreamClosed error

//go:linkname errClientDisconnected net/http/internal/http2.errClientDisconnected
var errClientDisconnected error

//go:linkname errClosedResponseBody net/http/internal/http2.errClosedResponseBody
var errClosedResponseBody error

func IsUnexpectedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errStreamClosed) || errors.Is(err, errClientDisconnected) || errors.Is(err, errClosedResponseBody) {
		return false
	}

	// *http2.StreamError
	if h2Err, ok := errors.AsType[http2.StreamError](err); ok {
		// ignore these errors
		switch h2Err.Code {
		case http2.ErrCodeStreamClosed, http2.ErrCodeCancel:
			return false
		}
	}

	rv := reflect.ValueOf(err)
	for rv.Kind() == reflect.Pointer {
		if !rv.IsValid() {
			return false
		}
		rv = rv.Elem()
	}
	// *http3.Error
	// https://github.com/quic-go/quic-go/blob/master/http3/error_codes.go
	if field := rv.FieldByName("ErrorCode"); field.CanUint() {
		// ignore these errors
		switch field.Uint() {
		case
			0x100, // http3.ErrCodeNoError
			0x10c: // http3.ErrCodeRequestCanceled
			return false
		}
	}
	return true
}
