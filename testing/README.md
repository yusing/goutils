# goutils/testing

Short, fail-fast assertions for Go tests, built only on the standard library. The
package name is `expect`, so the usual import uses an alias:

```go
import expect "github.com/yusing/goutils/testing"
```

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

The module needs Go 1.27 or newer. The package has no third-party dependencies.

## Quick start

```go
package example

import (
	"fmt"
	"io"
	"strconv"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

func TestExample(t *testing.T) {
	expect.Equal(t, []string{"ready"}, []string{"ready"}) // got, want

	n := expect.Must(strconv.Atoi("42")) // panics if the error is non-nil
	expect.Equal(t, n, 42)

	err := fmt.Errorf("read: %w", io.EOF)
	expect.ErrorIs(t, io.EOF, err) // expected first, then the error
	expect.ErrorContains(t, err, "read")
	expect.True(t, n > 40, "n = %d", n)
}
```

Running `go test` in the package directory prints the following, followed by the usual
`ok` summary line:

```text
=== RUN   TestExample
--- PASS: TestExample (0.00s)
PASS
```

A failed assertion reports its message at the caller's line and ends the test with
`t.Fatal`, so call assertions from the test goroutine, not from goroutines the test starts.

## Assertions

Every function takes the test first. Most end with `msgAndArgs ...any`: pass nothing, one
message, or a format string followed by its arguments (`"n = %d", n`).

| Group | Functions |
| --- | --- |
| Errors | `NoError`, `HasError`, `ErrorContains(t, err, substring)`, `ErrorIs(t, expected, err)`, `ErrorT[T](t, err)` |
| Booleans | `True`, `False` |
| Nil and empty | `Nil`, `NotNil`, `Empty`, `NotEmpty` |
| Equality | `Equal(t, got, want)`, `NotEqual(t, got, want)` |
| Ordering | `Greater`, `Less`, `GreaterOrEqual`, `LessOrEqual` (`t, got, bound`) |
| Membership | `Contains(t, element, slice)`, `StringsContain(t, text, substring)` |
| Panics and types | `Panics(t, fn)`, `Type[T](t, value)`, `Must(result, err)` |

### Which test types they accept

Most assertions take a `testing.TB`, so they work in tests, benchmarks, and fuzz targets.
Some take a `*testing.T` and will not compile in a benchmark:

- `testing.TB`: `NoError`, `HasError`, `ErrorContains`, `True`, `False`, `Nil`, `NotNil`,
  `Empty`, `NotEmpty`, `Panics`, `Greater`, `Less`, `GreaterOrEqual`, `LessOrEqual`.
- `*testing.T` only: `Equal`, `NotEqual`, `Contains`, `StringsContain`, `ErrorIs`,
  `ErrorT`, `Type`.
- `Must` takes no test value.

### Behavior notes

- **Argument order.** `Equal` is `(t, got, want)`. `ErrorIs` is `(t, expected, err)`, the
  reverse of `errors.Is(err, target)`. `Greater(t, got, bound)` checks `got > bound`.
- **`Equal`** uses `reflect.DeepEqual`, so a nil slice is not equal to an empty one. When
  the two values have different dynamic types (possible when `T` is an interface such as
  `any`), it also accepts convertible values, such as a named string type and `string`, or
  numbers of different kinds. Numeric equality requires lossless conversion in both
  directions, so `1.5` does not equal `1`.
  `NotEqual` is strict `reflect.DeepEqual` only.
- **`Nil`** is true for `nil` and for typed nil pointers, maps, slices, channels, and
  functions. `NotNil` is its negation.
- **`Empty`** is true for `nil`, zero-length strings, slices, maps, and channels, nil
  pointers and pointers to empty values, and any zero value (`0`, `false`, a zero struct, an
  array of zeros). `NotEmpty` is its negation.
- **`Greater` and the other ordering assertions** need both values to have the same ordered
  type; strings and named numeric types such as `time.Duration` work.
- **`Contains`** checks that a slice holds an element, by deep equality. For substrings use
  `StringsContain`.
- **`Type[T]`** asserts `value.(T)` and returns the converted value.
- **`ErrorT[T]`** passes when `errors.AsType[T]` finds an error of type `T` in the chain.
- **`Panics`** passes when the function panics, including `panic(nil)`.
- **`Must`** is for setup: `x := expect.Must(f())` returns the result or panics with the
  error.

## Verbose test output

Importing this package into a test binary turns on verbose output, as if you had run
`go test -v`. An `init` function inserts `-test.v` into the arguments when the executable
name ends in `.test`. With `go test` in a package directory you see the `=== RUN` and
`--- PASS` lines for each test. With a package list such as `go test ./...`, `go test`
still prints only a summary line for passing packages. The package does not configure
logging; see [`logging`](../logging/README.md) for installing a logger in tests.
