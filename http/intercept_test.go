package httputils

import (
	"testing"
)

func TestAsRequestInterceptedError(t *testing.T) {
	err := NewRequestInterceptedError(nil, nil)
	var target *RequestInterceptedError
	if !AsRequestInterceptedError(err, &target) {
		t.Errorf("expected true, got false")
	}
}

func TestAsRequestInterceptedError_NilTarget(t *testing.T) {
	err := NewRequestInterceptedError(nil, nil)
	var target **RequestInterceptedError
	if AsRequestInterceptedError(err, target) {
		t.Errorf("expected false, got true")
	}
}

func TestAsRequestInterceptedError_Nil(t *testing.T) {
	err := func() error {
		var nilErr *RequestInterceptedError
		return nilErr
	}()
	var target *RequestInterceptedError
	if AsRequestInterceptedError(err, &target) {
		t.Errorf("expected false, got true")
	}
}
