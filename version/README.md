# goutils/version

A small `Version` type for `v<generation>.<major>.<minor>` release tags, with parsing,
comparison, text and JSON encoding, and access to the version stamped into your binary
at build time.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/version"
```

The package name is `version`. It uses only the standard library and needs Go 1.27 or
newer.

## Quick start

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/yusing/goutils/version"
)

func main() {
	current := version.Parse("v1.2.3")
	latest := version.Parse("v1.2.4-beta")

	fmt.Println(latest, latest.IsNewerThan(current), latest.IsNewerThanMajor(current))
	fmt.Println(version.Parse("1.2.3"), version.Parse("v1.2.3-rc.1"))

	data, _ := json.Marshal(struct{ V version.Version }{current})
	fmt.Println(string(data))

	fmt.Println(version.Get())
}
```

Output:

```text
v1.2.4 true false
v0.0.0 v0.0.0
{"V":"v1.2.3"}
v0.0.0
```

## The type

```go
type Version struct{ Generation, Major, Minor int }
```

`v1.2.3` is generation 1, major 2, minor 3. Create one with `New(gen, major, minor)` or
`Parse`. `Version` is comparable with `==`, and `String()` always returns
`v<generation>.<major>.<minor>`.

## Parsing

`Parse(s)` never returns an error. It accepts only `v<digits>.<digits>.<digits>`, with an
optional suffix of a hyphen and word characters. Any such suffix is dropped:
`v3.1.0-beta` parses as `v3.1.0`. Everything else yields the zero `Version`
(`v0.0.0`) with no indication that parsing failed:

- no leading `v` (`1.2.3`)
- fewer or more than three numbers (`v1.2`, `v1.2.3.4`)
- a suffix with a dot or a second hyphen (`v1.2.3-rc.1`, `v1.2.3-beta-1`)
- numbers too large for an `int`, empty input, or branch names such as `feat/x`

`v0.0.0` is also a valid result for the input `v0.0.0`, so the two cannot be told apart.
Validate input yourself when that matters.

## Build version

`Get()` returns the version stamped into the binary, parsed once at program start. The
value comes from an unexported string that you set with the linker:

```sh
go build -ldflags "-X github.com/yusing/goutils/version.version=v1.2.3" ./cmd/app
```

Without the flag it is `unset`, so `Get()` returns `v0.0.0`. A value that does not match
the format above, such as a branch name, also gives `v0.0.0`.

## Comparing

| Method | True when |
| --- | --- |
| `IsEqual(o)` | all three numbers are equal |
| `IsNewerThan(o)` | newer by generation, then major, then minor |
| `IsNewerThanMajor(o)` | newer by generation or major; minor is ignored |
| `IsOlderThanMajor(o)` | older by generation or major; minor is ignored |
| `IsOlderThan(o)` | **not** newer than `o`, so also true for equal versions |
| `IsOlderMajorThan(o)` | **not** newer by generation or major, so also true when those are equal |

`IsOlderThan` and `IsOlderMajorThan` are the negations of the `IsNewer*` methods, not
strict comparisons: comparing a version with an equal one, `IsOlderThan` is `true` while
`IsOlderThanMajor` is `false`. For a strict "older" test on all three numbers, use
`o.IsNewerThan(v)`.

## Text and JSON

`Version` implements `encoding.TextMarshaler` and `TextUnmarshaler`, so JSON and other text
encodings write it as a string such as `"v1.2.3"`. `UnmarshalText` uses `Parse`: invalid
text produces `v0.0.0` and no error.
