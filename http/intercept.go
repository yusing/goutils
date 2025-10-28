// Package httputils provides HTTP utilities including response interception capabilities.
//
// HTTP Interception Usage Flow:
//
//  1. Setup Interception: Call client.InterceptHTTPClient(interceptFunc) to register
//     an intercept function with the HTTP client.
//
//  2. Implement Intercept Function: Create a function that receives *http.Response
//     and returns (intercepted bool, err error). When interception is desired,
//     return true and httputils.NewRequestInterceptedError(resp, processedData).
//
//  3. Handle Intercepted Responses: Use httputils.AsRequestInterceptedError() to
//     check for intercepted errors and extract the processed data from the Data field.
//
// This pattern enables preprocessing API responses before returning to the caller,
// allowing custom response handling without modifying the original API calls.
// See internal/watcher/health/monitor/docker.go for a complete example.
package httputils

import (
	"context"
	"errors"
	"net/http"
)

type RequestInterceptedError struct {
	Response *http.Response
	Data     any
}

var interceptedError = "request intercepted"

func (e *RequestInterceptedError) Error() string {
	return interceptedError
}

func (e *RequestInterceptedError) Is(target error) bool {
	// fastpath to prevent docker from further parsing the error
	if target == context.Canceled || target == context.DeadlineExceeded {
		return true
	}
	_, ok := target.(*RequestInterceptedError)
	return ok
}

func NewRequestInterceptedError(response *http.Response, data any) *RequestInterceptedError {
	return &RequestInterceptedError{Response: response, Data: data}
}

func AsRequestInterceptedError(err error, target **RequestInterceptedError) bool {
	// fastpath to prevent reflection overhead in errors.As
	if err, ok := err.(*RequestInterceptedError); ok {
		if target == nil || err == nil {
			return false
		}
		*target = err
		return true
	}
	return errors.As(err, target)
}

// InterceptedTransport wraps an http.RoundTripper to intercept Docker responses
type InterceptedTransport struct {
	transport http.RoundTripper
	intercept InterceptFunc
}

type InterceptFunc func(resp *http.Response) (intercepted bool, err error)

func NewInterceptedTransport(transport http.RoundTripper, intercept InterceptFunc) *InterceptedTransport {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &InterceptedTransport{
		transport: transport,
		intercept: intercept,
	}
}

func (t *InterceptedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	intercepted, err := t.intercept(resp)
	var interceptedErr *RequestInterceptedError
	if err != nil && !AsRequestInterceptedError(err, &interceptedErr) {
		return nil, err
	}
	if intercepted {
		if interceptedErr == nil {
			interceptedErr = NewRequestInterceptedError(resp, nil)
		}
		// cannot return both resp and err, so return nil, err with resp wrapped
		// otherwise "RoundTripper returned a response & error; ignoring response" will be logged
		return nil, interceptedErr
	}
	return resp, nil
}
