package gperr

import (
	"fmt"
	"sync"
)

type Builder struct {
	about string
	errs  []error
	mu    sync.RWMutex

	concurrent bool
}

// NewBuilder creates a new Builder.
//
// If context is not provided, the Builder will not have a subject
// and will expand when adding to another builder.
func NewBuilder(context ...string) Builder {
	if len(context) == 0 {
		return Builder{}
	}
	return Builder{about: context[0]}
}

func NewBuilderWithConcurrency(context ...string) Builder {
	if len(context) == 0 {
		return Builder{concurrent: true}
	}
	return Builder{about: context[0], concurrent: true}
}

func (b *Builder) EnableConcurrency() {
	b.concurrent = true
}

func (b *Builder) About() string {
	return b.about
}

func (b *Builder) HasError() bool {
	b.RLock()
	defer b.RUnlock()
	return len(b.errs) > 0
}

func (b *Builder) Error() Error {
	b.RLock()
	defer b.RUnlock()
	if len(b.errs) == 0 {
		return nil
	}
	if len(b.errs) == 1 && b.about == "" {
		return wrap(b.errs[0])
	}
	return &nestedError{Err: New(b.about), Extras: b.errs}
}

func (b *Builder) String() string {
	err := b.Error()
	if err == nil {
		return ""
	}
	return err.Error()
}

// Add adds an error to the Builder.
//
// adding nil is no-op.
func (b *Builder) Add(err error) {
	if err == nil {
		return
	}

	b.Lock()
	defer b.Unlock()

	b.add(err)
}

func (b *Builder) Adds(err string) {
	b.Lock()
	defer b.Unlock()
	b.errs = append(b.errs, newError(err))
}

func (b *Builder) Addf(format string, args ...any) {
	if len(args) > 0 {
		b.Lock()
		b.errs = append(b.errs, fmt.Errorf(format, args...))
		b.Unlock()
	} else {
		b.Adds(format)
	}
}

func (b *Builder) AddFrom(other *Builder, flatten bool) {
	if other == nil || !other.HasError() {
		return
	}

	b.Lock()
	defer b.Unlock()
	if flatten {
		b.errs = append(b.errs, other.errs...)
	} else {
		b.errs = append(b.errs, other.Error())
	}
}

func (b *Builder) AddRange(errs ...error) {
	nonNilErrs := make([]error, 0, len(errs))
	for _, err := range errs {
		if err != nil {
			nonNilErrs = append(nonNilErrs, err)
		}
	}

	b.Lock()
	defer b.Unlock()

	for _, err := range nonNilErrs {
		b.add(err)
	}
}

func (b *Builder) ForEach(fn func(error)) {
	b.RLock()
	errs := b.errs
	b.RUnlock()

	for _, err := range errs {
		fn(err)
	}
}

func (b *Builder) add(err error) {
	switch err := err.(type) { //nolint:errorlint
	case *baseError:
		b.errs = append(b.errs, err.Err)
	case *nestedError:
		if err.Err == nil {
			b.errs = append(b.errs, err.Extras...)
		} else {
			b.errs = append(b.errs, err)
		}
	case *MultilineError:
		b.add(&err.nestedError)
	default:
		b.errs = append(b.errs, err)
	}
}

func (b *Builder) Lock() {
	if b.concurrent {
		b.mu.Lock()
	}
}

func (b *Builder) Unlock() {
	if b.concurrent {
		b.mu.Unlock()
	}
}
func (b *Builder) RLock() {
	if b.concurrent {
		b.mu.RLock()
	}
}

func (b *Builder) RUnlock() {
	if b.concurrent {
		b.mu.RUnlock()
	}
}
