package synk

import (
	"sync/atomic"

	"github.com/bytedance/sonic"
)

// TODO: moved to goutils
type Value[T any] struct {
	atomic.Value
}

func (a *Value[T]) Load() T {
	if v := a.Value.Load(); v != nil {
		return v.(T)
	}
	var zero T
	return zero
}

func (a *Value[T]) Store(v T) {
	a.Value.Store(v)
}

func (a *Value[T]) Swap(v T) T {
	if v := a.Value.Swap(v); v != nil {
		return v.(T)
	}
	var zero T
	return zero
}

func (a *Value[T]) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(a.Load())
}
