package task

import (
	"context"
	"sync/atomic"

	"github.com/puzpuzpuz/xsync/v4"
	"golang.org/x/sync/semaphore"
)

type Dependencies[T comparable] struct {
	m     *xsync.Map[T, struct{}]
	sem   *semaphore.Weighted
	count atomic.Int64
}

func NewDependencies[T comparable]() *Dependencies[T] {
	return &Dependencies[T]{
		m:   xsync.NewMap[T, struct{}](xsync.WithGrowOnly()),
		sem: semaphore.NewWeighted(1),
	}
}

func (w *Dependencies[T]) Add(ele T) {
	w.m.Store(ele, struct{}{})
	if w.count.Add(1) == 1 {
		// First element, acquire the semaphore
		w.sem.Acquire(context.Background(), 1)
	}
}

func (w *Dependencies[T]) Delete(key T) {
	if _, exists := w.m.LoadAndDelete(key); exists {
		if w.count.Add(-1) == 0 {
			// Last element, release the semaphore
			w.sem.Release(1)
		}
	}
}

func (w *Dependencies[T]) Wait(ctx context.Context) error {
	err := w.sem.Acquire(ctx, 1)
	w.m.Clear()
	return err
}

func (w *Dependencies[T]) Range(yield func(T) bool) {
	for ele := range w.m.Range {
		if !yield(ele) {
			break
		}
	}
}
