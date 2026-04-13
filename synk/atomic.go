package synk

import (
	"encoding/json"
	"sync/atomic"
)

type Value[T any] struct {
	atomic.Value
}

// Load returns the current value of the [Value] v.
// If the value is nil, it will return the zero value of the type T.
func (a *Value[T]) Load() T {
	if v := a.Value.Load(); v != nil {
		return v.(T)
	}
	var zero T
	return zero
}

// Store sets the value of the [Value] v to val.
// Setting a nil value will panic.
func (a *Value[T]) Store(v T) {
	a.Value.Store(v)
}

// Swap exchanges the current value of the [Value] v with new.
// If the current value is nil, it will return the zero value of the type T.
func (a *Value[T]) Swap(v T) T {
	if v := a.Value.Swap(v); v != nil {
		return v.(T)
	}
	var zero T
	return zero
}

// MarshalJSON returns the JSON encoding of the current value of the [Value] v.
// If the value is nil, it will return the JSON encoding of the zero value of the type T.
func (a *Value[T]) MarshalJSON() ([]byte, error) {
	if v := a.Value.Load(); v != nil {
		return json.Marshal(v)
	}
	var zero T
	return json.Marshal(zero)
}
