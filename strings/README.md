# goutils/strings

String helpers for Go services: case-insensitive search, human-readable durations,
byte sizes and times, JSON through `encoding/json/v2`, secret masking, URI and filename
checks, and a few small utilities.

The import path ends in `strings`, but the package name is `strutils` (so it does not
collide with the standard library's `strings`):

```go
import strutils "github.com/yusing/goutils/strings"
```

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

The module needs Go 1.27 or newer. The only third-party dependency this package pulls in
is `golang.org/x/text` (for `Title`).

## Quick start

```go
package main

import (
	"fmt"
	"time"

	strutils "github.com/yusing/goutils/strings"
)

func main() {
	fmt.Println(strutils.FormatDuration(51*time.Hour + 45*time.Minute))
	fmt.Println(strutils.FormatByteSize(1536))

	ref := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	fmt.Println(strutils.FormatTimeWithReference(ref.Add(-5*time.Minute), ref))

	fmt.Println(strutils.CommaSeperatedList("a, b, c"))
	fmt.Println(strutils.ContainsFold("Hello World", "world"))
	fmt.Println(strutils.Redact("hunter2secret"))
	fmt.Println(strutils.SanitizeURI("docs/../admin"))
}
```

Output:

```text
2 days, 3 hours and 45 minutes
1.5 KiB
5 minutes ago
[a b c]
true
hu*********et
/admin
```

## Text helpers

| Function | Behavior |
| --- | --- |
| `ContainsFold(s, sub)`, `IndexFold(s, sub)` | Case-insensitive search. `IndexFold` returns a byte offset into `s`, or `-1`. ASCII inputs take an allocation-free path. |
| `HasPrefixFold(s, prefix)`, `HasSuffixFold(s, suffix)` | Case-insensitive prefix and suffix tests (`strings.EqualFold` rules). |
| `CommaSeperatedList(s)` | Splits on commas **and any whitespace**, dropping empty items: `"New York, Paris"` gives `["New" "York" "Paris"]`. `""` gives an empty, non-nil slice. (The misspelling is the real API name.) |
| `Title(s)` | Title case with American English rules; the rest of each word is lowercased (`"hello wORLD"` gives `Hello World`). |
| `ToLowerNoSnake(s)` | Removes `_` and lowercases ASCII letters: `Foo_Bar` gives `foobar`. |
| `LevenshteinDistance(a, b)` | Edit distance computed over **bytes**, so each non-ASCII character counts as several edits. |
| `Pluralize(n)` | `"s"` when `n > 1`, otherwise `""`. |
| `NewUUIDv7()` | Deprecated: use `uuid.NewV7().String()` from the standard library. See [UUIDs](#uuids). |

## Formatting durations, sizes, and times

Each `Format*` function has an `Append*` twin that appends to a `[]byte` instead of
allocating a string.

`FormatDuration` renders at most three of days, hours, and minutes, separated by commas
with the word "and" before the last unit. Seconds are shown only below one hour, and
sub-second values use `ns` or `ms`:

| Input | Output |
| --- | --- |
| `0` | `0 Seconds` |
| `500 * time.Nanosecond` | `500 ns` |
| `5 * time.Millisecond` | `5 ms` |
| `61 * time.Second` | `1 minute and 1 second` |
| `3661 * time.Second` | `1 hour and 1 minute` |
| `51*time.Hour + 45*time.Minute + 15*time.Second` | `2 days, 3 hours and 45 minutes` |
| `-90 * time.Second` | `-1 minute and 30 seconds` |

`FormatByteSize` accepts `int`, `uint`, `int64`, `uint64`, and `float64` and uses binary
units (`B`, `KiB`, `MiB`, `GiB`, `TiB`, `PiB`) with up to two decimals:

| Input | Output |
| --- | --- |
| `512` | `512 B` |
| `1536` | `1.5 KiB` |
| `1 << 20` | `1 MiB` |
| `1536000` | `1.46 MiB` |
| `5 << 40` | `5 TiB` |

Named types with these underlying types are supported too. Zero prints `0 B`.

`FormatTimeWithReference(t, ref)` describes `t` relative to `ref`. `FormatTime(t)` uses
`time.Now()` as the reference, `FormatUnixTime(sec)` takes Unix seconds, and
`FormatLastSeen(t)` gives the same result as `FormatTime(t)`:

| `t` relative to `ref` | Output |
| --- | --- |
| zero `time.Time` | `never` |
| within 1 second | `now` |
| 1 to 3 seconds in the past | `just now` |
| under 1 minute | `5 seconds ago`, `in 5 seconds` |
| under 1 hour | `5 minutes ago`, `in 5 minutes` |
| under 1 day | `10 hours ago`, `in 10 hours` |
| a day or more away, same year | `04-29 12:00:00` |
| a day or more away, different year | `2023-05-01 12:00:00` |

Relative units are not singularized (`1 minutes ago`, `in 1 seconds`).

## JSON

`MarshalJSON`, `UnmarshalJSON`, `MarshalJSONIndent`, `MarshalString`, `UnmarshalFromString`,
`NewJSONEncoder`, and `NewJSONDecoder` use `encoding/json/v2` with one customization:
`time.Duration` is encoded as an integer number of nanoseconds, matching `encoding/json`
v1 (`1s` becomes `1000000000`). `ValidJSON` and `ValidJSONString` check that the input is
exactly one valid JSON value; trailing data makes them return `false`.

Go's v2 defaults differ from `encoding/json` v1 in ways that affect stored data:

- Field names match case-sensitively; unknown fields are ignored; duplicate object
  member names are an error.
- Nil slices encode as `[]` and nil maps as `{}`.
- Map member order is not deterministic.
- Types that implement `MarshalJSON`/`UnmarshalJSON` (such as `Redacted`) work with both
  libraries.

`NewJSONEncoder(w).Encode(v)` writes a trailing newline and does not escape HTML unless
you call `SetEscapeHTML(true)`. `SetIndent(prefix, indent)` enables indented output;
both strings must contain only spaces or tabs. `SetIndent("", "")` restores compact output.

## Redacting secrets

`Redact(s)` masks the middle of a string using Unicode character boundaries.
For more than four characters, the first two and last two stay visible and the
rest becomes `*`. Three or four characters keep only the first and last
(`"abcd"` gives `a**d`); one or two characters are fully masked (`"ab"` gives `**`).
Empty strings stay empty.

`Redacted` is a `string` type that applies `Redact` when marshaled to JSON or YAML. It
stores the real value, so `String()` and `fmt` verbs print it unmasked:

```go
package main

import (
	"fmt"
	"time"

	strutils "github.com/yusing/goutils/strings"
)

type Creds struct {
	User string            `json:"user"`
	Pass strutils.Redacted `json:"pass"`
	Wait time.Duration     `json:"wait"`
}

func main() {
	out, err := strutils.MarshalJSON(Creds{"bob", "hunter2", time.Second})
	fmt.Println(string(out), err)

	var c Creds
	err = strutils.UnmarshalFromString(`{"user":"bob","pass":"hunter2"}`, &c)
	fmt.Println(c.Pass.String(), c.Pass.Empty(), err) // unmarshaling keeps the real value
}
```

Output:

```text
{"user":"bob","pass":"hu***r2","wait":1000000000} <nil>
hunter2 false <nil>
```

## YAML

This package does not import a YAML library. Register one once at startup with
`SetYAMLMarshaler` and `SetYAMLUnmarshaler`, then use `MarshalYAML` and `UnmarshalYAML`.
Calling either before its function is registered panics with a nil pointer dereference,
and so does a YAML library invoking the `Redacted` hooks.

`Redacted` implements the byte-slice hooks `MarshalYAML() ([]byte, error)` and
`UnmarshalYAML([]byte) error`, which `github.com/goccy/go-yaml` understands. Other YAML
libraries ignore them; with `gopkg.in/yaml.v3`, for example, a `Redacted` value is written
unmasked.

```go
import "github.com/goccy/go-yaml"

func init() {
	strutils.SetYAMLMarshaler(yaml.Marshal)
	strutils.SetYAMLUnmarshaler(func(data []byte, v any) error { return yaml.Unmarshal(data, v) })
}
```

## Generic parsing

`Parser` is `interface{ Parse(string) error }`. `Parse[T](s)` allocates a `T`, calls its
`Parse` method, and returns it; `MustParse[T]` panics with `must failed: <error>`
instead of returning the error. Use a pointer type for `T` so the method can fill in the
value:

```go
package main

import (
	"fmt"
	"strconv"

	strutils "github.com/yusing/goutils/strings"
)

type Port int

func (p *Port) Parse(s string) error {
	n, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*p = Port(n)
	return nil
}

func main() {
	p, err := strutils.Parse[*Port]("8080")
	fmt.Println(*p, err)

	_, err = strutils.Parse[*Port]("http")
	fmt.Println(err)
}
```

Output:

```text
8080 <nil>
strconv.Atoi: parsing "http": invalid syntax
```

## Path and name checks

- `SanitizeURI(uri)` makes a request path safe to join or redirect to: it adds a leading
  `/`, cleans `.` and `..` segments, collapses repeated slashes, drops a trailing slash,
  and turns anything starting with `//` or `/\` into `/`. Input that starts with
  `http://` or `https://` is returned **unchanged**, so validate absolute URLs separately.
  The whole string is cleaned as a path, so treat it as a path, not as a full request
  URI with a query.
- `IsValidFilename(name)` returns `false` when `name` contains `/`, `\`, or `..`. Nothing
  else is checked: the empty string, `.`, and names with NUL bytes or reserved Windows
  names pass.

```go
strutils.SanitizeURI("")                 // "/"
strutils.SanitizeURI("a/./b/../c/")      // "/a/c"
strutils.SanitizeURI("//evil.com/path")  // "/"
strutils.IsValidFilename("report.pdf")   // true
strutils.IsValidFilename("../secret")    // false
```

## UUIDs

`NewUUIDv7()` is deprecated. Use `uuid.NewV7().String()` from the standard library
directly. The compatibility wrapper returns an RFC 9562 version 7 UUID using Go's `uuid.NewV7`: a
millisecond timestamp plus cryptographically random bits. UUIDs are time-ordered
and suitable for collision-resistant identifiers across processes, not secrets.
The clock comes from the standard library, not `mockable.TimeNow`.

## API reference

```sh
go doc -all github.com/yusing/goutils/strings
```

For terminal color helpers, see [`ansi`](ansi/README.md).
