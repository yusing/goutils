package task

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
)

type Dependencies[T comparable] struct {
	m     *xsync.Map[T, struct{}]
	count atomic.Int64
	done  atomic.Pointer[chan struct{}]
}

func NewDependencies[T comparable]() *Dependencies[T] {
	return &Dependencies[T]{
		m: xsync.NewMap[T, struct{}](xsync.WithGrowOnly()),
	}
}

func (w *Dependencies[T]) Add(ele T) {
	w.m.Store(ele, struct{}{})
	if w.count.Add(1) == 1 {
		// Create new channel to block Wait()
		ch := make(chan struct{})
		w.done.Store(&ch)
	}
}

func (w *Dependencies[T]) Delete(ele T) {
	if _, exists := w.m.LoadAndDelete(ele); exists {
		if w.count.Add(-1) == 0 {
			// Close channel to unblock Wait()
			done := w.done.Load()
			if done != nil {
				close(*done)
			}
		}
	}
}

func (w *Dependencies[T]) Wait(ctx context.Context) error {
	done := w.done.Load()
	if done == nil {
		if w.count.Load() != 0 {
			return errors.New("bug: count != 0")
		}
		w.m.Clear()
		return nil
	}

	select {
	case <-*done:
		w.m.Clear()
		count := w.count.Load()
		if count > 0 {
			return errors.New("bug: new dependencies added after Wait")
		}
		if count < 0 {
			return errors.New("bug: count < 0")
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *Dependencies[T]) Range(yield func(T) bool) {
	for ele := range w.m.Range {
		if !yield(ele) {
			break
		}
	}
}
