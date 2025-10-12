package httputils

import (
	"errors"
	"net/http"
)

var ErrRequestIntercepted = errors.New("docker request intercepted")

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
	if err != nil {
		return nil, err
	}
	if intercepted {
		return resp, ErrRequestIntercepted
	}

	return resp, nil
}
