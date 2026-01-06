package gperr

import (
	"fmt"
	"sync"
)

type Builder struct {
	about string
	errs  []error
	mu    sync.RWMutex
}

// NewBuilder creates a new Builder.
//
// If context is not provided, the Builder will not have a subject
// and will expand when adding to another builder.
func NewBuilder(context string) Builder {
	return Builder{about: context}
}

func (b *Builder) About() string {
	return b.about
}

func (b *Builder) HasError() bool {
	return len(b.errs) > 0
}

func (b *Builder) Error() Error {
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

	b.add(err)
}

func (b *Builder) AddSubjectf(err error, format string, args ...any) {
	if err == nil {
		return
	}
	if len(args) > 0 {
		err = PrependSubject(fmt.Sprintf(format, args...), err)
	} else {
		err = PrependSubject(format, err)
	}
	b.add(err)
}

func (b *Builder) Adds(err string) {
	b.errs = append(b.errs, newError(err))
}

func (b *Builder) Addf(format string, args ...any) {
	if len(args) > 0 {
		b.errs = append(b.errs, fmt.Errorf(format, args...))
	} else {
		b.Adds(format)
	}
}

func (b *Builder) AddFrom(other *Builder, flatten bool) {
	if other == nil || !other.HasError() {
		return
	}

	if flatten {
		b.errs = append(b.errs, other.errs...)
	} else {
		b.errs = append(b.errs, other.Error())
	}
}

func (b *Builder) AddRange(errs ...error) {
	for _, err := range errs {
		if err != nil {
			b.add(err)
		}
	}
}

func (b *Builder) ForEach(fn func(error)) {
	for _, err := range b.errs {
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
		if err.currentParent != nil {
			b.add(err.currentParent)
		}
	default:
		b.errs = append(b.errs, err)
	}
}
