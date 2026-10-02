# goutils/testing

Dependency-free, fail-fast testing helpers built on the Go standard library.
The package name is `expect`.

## Usage

```go
package example

import (
    "testing"
    expect "github.com/yusing/goutils/testing"
)

func TestValue(t *testing.T) {
    expect.Equal(t, []string{"ready"}, []string{"ready"})
    expect.True(t, true)
}
```

Assertions mark themselves as helpers and stop the current test on failure.
An optional message may be a plain string or a format string followed by arguments.

## Assertions

- `NoError`, `HasError`, `ErrorContains`, and `ErrorIs` inspect errors.
  `ErrorIs(t, expected, err)` uses `errors.Is`; `ErrorT[T](t, err)` finds a wrapped
  error of type `T`.
- `True`, `False`, `Nil`, `NotNil`, `Empty`, and `NotEmpty` inspect values.
  Nil checks include typed nil pointers, maps, slices, channels, and functions.
  Empty checks include zero-length collections and strings, zero values
  (including arrays whose elements are all zero), and
  pointers to empty values.
- `Equal(t, got, want)` compares deep values and convertible numeric types.
  `NotEqual` checks strict deep inequality, including concrete types.
- `Greater`, `Less`, `GreaterOrEqual`, and `LessOrEqual` accept values of the same
  ordered type, including strings and named numeric types such as durations.
- `Contains(t, element, choices)` checks deep membership in a slice.
  `StringsContain(t, text, substring)` checks substring containment.
- `Panics` requires the supplied function to panic.
  `Type[T](t, value)` asserts and returns a value of type `T`.
- `Must(result, err)` returns the result or panics with the non-nil error.

## Test Configuration

Importing this package into a test executable enables verbose test output.
The package does not configure logging or change framework log levels. Tests that
need diagnostics should install a logger through
[`goutils/logging`](../logging/README.md) and configure their chosen framework.
