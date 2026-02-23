package gperr

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
)

func New(message string) Error {
	if message == "" {
		return nil
	}
	return baseError{errors.New(message)}
}

type noUnwrap struct {
	error
}

func (e noUnwrap) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Error())
}

func (e noUnwrap) Is(target error) bool {
	return errors.Is(e.error, target)
}

func (e noUnwrap) As(target any) bool {
	return errors.As(e.error, target)
}

func Errorf(format string, args ...any) Error {
	return baseError{noUnwrap{fmt.Errorf(format, args...)}}
}

// Wrap wraps message in front of the error message.
func Wrap(err error, message ...string) Error {
	if err == nil {
		return nil
	}
	if len(message) == 0 || message[0] == "" {
		return wrap(err)
	}
	//nolint:errorlint
	switch err := wrap(err).(type) {
	case baseError:
		return baseError{&wrappedError{err.Err, message[0]}}
	case *nestedError:
		return &nestedError{Extras: slices.Clone(err.Extras), Err: &wrappedError{err.Err, message[0]}}
	}
	return baseError{&wrappedError{err, message[0]}}
}

func Unwrap(err error) Error {
	//nolint:errorlint
	switch err := err.(type) {
	case interface{ Unwrap() []error }:
		return &nestedError{Extras: err.Unwrap()}
	case interface{ Unwrap() error }:
		return baseError{err.Unwrap()}
	default:
		return baseError{err}
	}
}

var (
	// errors.errorString
	errorStringType = reflect.TypeOf(errors.New(""))
	// fmt.wrapError
	wrapErrorType = reflect.TypeOf(fmt.Errorf("foo: %w", errors.New("bar")))
	// fmt.wrapErrors
	wrapErrorsType = reflect.TypeOf(fmt.Errorf("foo: %w, bar: %w", errors.New("bar"), errors.New("baz")))
)

func wrap(err error) Error {
	if err == nil {
		return nil
	}
	//nolint:errorlint
	switch err := err.(type) {
	case Error:
		return err
	}
	switch reflect.TypeOf(err) {
	case errorStringType, wrapErrorType, wrapErrorsType:
		// prevent unwrapping causing MarshalJSON to resulting in {} or empty string
		return baseError{noUnwrap{err}}
	}
	return baseError{err}
}

func Join(errors ...error) Error {
	n := 0
	for _, err := range errors {
		if err != nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	errs := make([]error, n)
	i := 0
	for _, err := range errors {
		if err != nil {
			errs[i] = err
			i++
		}
	}
	return &nestedError{Extras: errs}
}

func JoinLines(main error, lines ...string) Error {
	errs := make([]error, len(lines))
	for i, line := range lines {
		if line == "" {
			continue
		}
		errs[i] = errors.New(line)
	}
	return &nestedError{Err: main, Extras: errs}
}

func Collect[T any, Err error, Arg any, Func func(Arg) (T, Err)](eb *Builder, fn Func, arg Arg) T {
	result, err := fn(arg)
	eb.Add(err)
	return result
}

func Normal(err error) []byte {
	if err == nil {
		return nil
	}
	return []byte(err.Error())
}

func Plain(err error) []byte {
	if err == nil {
		return nil
	}
	if p, ok := err.(PlainError); ok {
		return p.Plain()
	}
	return []byte(err.Error())
}

func Markdown(err error) []byte {
	if err == nil {
		return nil
	}
	switch err := err.(type) {
	case MarkdownError:
		return err.Markdown()
	case interface{ Unwrap() []error }:
		return appendLines(nil, err.Unwrap(), 0, appendLineMd)
	default:
		return []byte(err.Error())
	}
}
