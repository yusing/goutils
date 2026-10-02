# goutils/intern

Value interning for any comparable type: equal values share one stored copy, and
handles to them compare with `==` in constant time. It is a thin wrapper around the
standard library's `unique.Handle[T]` that adds JSON support, so interned values can be
struct fields.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/intern"
```

The package name is `intern`. It needs Go 1.27 or newer and uses `goutils/strings` for
JSON.

## Quick start

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/yusing/goutils/intern"
)

type Route struct {
	Host intern.Handle[string] `json:"host"`
}

func main() {
	h1 := intern.Make("example.com")
	h2 := intern.Make("example.com")
	fmt.Println(h1 == h2, h1.Value())

	data, _ := json.Marshal(Route{Host: h1})
	fmt.Println(string(data))

	var r Route
	if err := json.Unmarshal([]byte(`{"host":"example.com"}`), &r); err != nil {
		panic(err)
	}
	fmt.Println(r.Host == h1)

	var missing Route
	json.Unmarshal([]byte(`{}`), &missing)
	fmt.Println(missing.Host == intern.Handle[string]{})
}
```

Output:

```text
true example.com
{"host":"example.com"}
true
true
```

## API

| Function | Behavior |
| --- | --- |
| `Make[T comparable](v T) Handle[T]` | Returns the handle for `v`; equal values give equal handles. |
| `(Handle[T]) Value() T` | Returns the interned value. |
| `MakeValue[T comparable](v T) T` | Shortcut for `unique.Make(v).Value()`: returns the shared copy of `v` without keeping the handle. For strings, equal inputs end up sharing one backing array. |

`Handle[T]` works for any comparable `T`, not only strings, and is itself comparable, so
it can be a map key.

## Pitfalls

- **The zero `Handle` is not usable.** Calling `Value()` or marshaling it panics with a nil
  pointer dereference. A handle only becomes valid through `Make` or a successful
  `UnmarshalJSON`. Unmarshaling `null`, `{}`, or empty input does nothing, so a missing
  JSON field leaves the zero handle. Compare against `intern.Handle[T]{}` before calling
  `Value()` on data you did not construct.
- JSON encoding and decoding go through `goutils/strings` (`encoding/json/v2` rules), and
  the type also works with `encoding/json`.
- Interned copies are released by the garbage collector once no `Handle` refers to them.
  `MakeValue` keeps no handle, so strings it returned earlier can stop being shared after
  a collection unless a `Handle` for the same value is still alive. Keep the `Handle` from
  `Make` when you depend on the sharing.

## When to use it

Use it when a program keeps many copies of a small set of strings or other values, such as
hostnames, container names, or labels parsed from configuration, and you want less memory
or cheap equality. For a handful of values the saving is not worth the indirection, and a
plain `string` is easier to work with.
