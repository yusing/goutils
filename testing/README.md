# testing

Testing utilities with testify wrappers and logging support.

## Overview

The `testing` package provides wrapper functions around testify/require for common assertions.

## API Reference

### Panic Handler

```go
func Must[Result any](r Result, err error) Result
```

### Assertions

```go
var (
    NoError        = require.NoError
    HasError       = require.Error
    True           = require.True
    False          = require.False
    Nil            = require.Nil
    NotNil         = require.NotNil
    ErrorContains  = require.ErrorContains
    Panics         = require.Panics
    Greater        = require.Greater
    Less           = require.Less
    GreaterOrEqual = require.GreaterOrEqual
    LessOrEqual    = require.LessOrEqual
)
```

### Custom Assertions

```go
func ErrorIs(t *testing.T, expected error, err error, msgAndArgs ...any)
func ErrorT[T error](t *testing.T, err error, msgAndArgs ...any)
func Equal[T any](t *testing.T, got T, want T, msgAndArgs ...any)
func NotEqual[T any](t *testing.T, got T, want T, msgAndArgs ...any)
func Contains[T any](t *testing.T, got T, wants []T, msgAndArgs ...any)
func StringsContain(t *testing.T, got string, want string, msgAndArgs ...any)
func Type[T any](t *testing.T, got any, msgAndArgs ...any) T
```

## Usage

```go
package mypkg

import (
    "testing"
    "github.com/yusing/goutils/expect"
)

func TestMyFunc(t *testing.T) {
    result, err := MyFunc()
    expect.NoError(t, err)
    expect.Equal(t, result, expectedValue)

    // Type assertion with generics
    typed := expect.Type[MyType](t, someValue)
}
```

## Features

- Simplified assertion API
- Generic type support
- Automatic verbose mode in tests
- Debug-level logging in tests
