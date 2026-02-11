package workerpool

import (
	"context"
	"runtime"
	"sync/atomic"
)

// Pool is a pool of workers that can be used to execute functions concurrently.
//
// The pool is a semaphore that limits the number of active workers, by default it is set to the number of CPUs.
type Pool interface {
	Go(fn func(ctx context.Context, idx int))
	Wait()
}

type pool struct {
	ctx  context.Context
	sem  chan struct{}
	next atomic.Int64
}

type options struct {
	n int
}

type option func(opts *options)

func WithN(n int) option {
	return func(opts *options) {
		opts.n = n
	}
}

// New creates a new Pool with the given context and options.
func New(ctx context.Context, opts ...option) Pool {
	wopts := options{
		n: runtime.GOMAXPROCS(0),
	}
	for _, opt := range opts {
		opt(&wopts)
	}

	if ctx == nil {
		ctx = context.Background()
	}

	sem := make(chan struct{}, wopts.n)

	// fill the semaphore with n slots
	for range wopts.n {
		sem <- struct{}{}
	}

	return &pool{
		ctx: ctx,
		sem: sem,
	}
}

func (p *pool) Go(fn func(ctx context.Context, idx int)) {
	if fn == nil {
		return
	}

	select {
	case <-p.ctx.Done():
		return
	case <-p.sem:
	}

	idx := int(p.next.Add(1) - 1)
	go func() {
		defer func() { p.sem <- struct{}{} }()
		fn(p.ctx, idx)
	}()
}

func (p *pool) Wait() {
	n := cap(p.sem)
	acquired := 0
	for range n {
		select {
		case <-p.ctx.Done():
			for range acquired {
				p.sem <- struct{}{}
			}
			return
		case <-p.sem:
			acquired++
		}
	}
	for range n {
		p.sem <- struct{}{}
	}
}
