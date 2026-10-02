# goutils/errs

Errors that know what they are about. `errs` adds a *subject* ("which route, file, or
field failed"), nested sub-errors, and plain/Markdown/ANSI renderings on top of the
standard `error` interface, while still working with `errors.Is`, `errors.As`, and
`errors.Unwrap`.

The import path ends in `errs`, but the package name is `gperr`:

```go
import gperr "github.com/yusing/goutils/errs"
```

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

The module needs Go 1.27 or newer. Besides other `goutils` packages (`strings` and
`strings/ansi`), the only third-party code it pulls in is `golang.org/x/text`, through
`goutils/strings`.

## Quick start

Collect every failure instead of stopping at the first one, and tag each with a subject:

```go
package main

import (
	"fmt"
	"strconv"

	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/strings/ansi"
)

func parsePorts(values []string) ([]int, error) {
	errs := gperr.NewBuilder("invalid ports")
	var ports []int
	for i, v := range values {
		port, err := strconv.Atoi(v)
		if err != nil {
			errs.AddSubjectf(err, "ports[%d]", i)
			continue
		}
		ports = append(ports, port)
	}
	return ports, errs.Error() // nil error when nothing was added
}

func main() {
	_, err := parsePorts([]string{"80", "http", ""})
	fmt.Print(ansi.StripANSI(err.Error()))

	_, err = parsePorts([]string{"80", "443"})
	fmt.Println(err == nil)
}
```

Output:

```text
invalid ports
  • ports[1]: strconv.Atoi: parsing "http": invalid syntax
  • ports[2]: strconv.Atoi: parsing "": invalid syntax
true
```

`err.Error()` contains ANSI escape sequences wherever a subject is highlighted (see
[Rendering](#rendering)), so the example strips them for stable output.

## Creating errors

| Function | Result |
| --- | --- |
| `New(msg)` | A new `Error`. `New("")` returns `nil`. |
| `Errorf(format, args...)` | Like `fmt.Errorf`; `%w` operands still match `errors.Is`/`errors.As`. |
| `Wrap(err, msg...)` | `"msg: err"`; with no (or an empty) message it only converts `err` to an `Error`. `Wrap(nil)` is `nil`. |
| `Unwrap(err)` | One `Unwrap` step of a standard-library wrapper as an `Error`; the members of a multi-error become sub-errors. A nil input or nil unwrapped error returns nil. |
| `Join(errs...)` | All non-nil errors, one per line, with no header. `nil` when all are `nil`. |
| `JoinLines(main, lines...)` | `main` followed by one bulleted line per non-empty string. |

`Error` extends `error` with `Is`, `With`, `Withf`, `Subject`, `Subjectf`, `Plain`,
and `Markdown`. Errors are immutable: every method returns a new value and leaves
the receiver unchanged. `Error` is an interface, so a `nil` result converts to a
`nil` `error` and `if err != nil` works as usual.

## Subjects

A subject names the thing that failed. `Subject` and `PrependSubject` add an outer
subject; subjects are joined with ` > ` from outermost to innermost, and the
innermost one is highlighted:

```go
package main

import (
	"errors"
	"fmt"
	"io"

	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/strings/ansi"
)

var ErrNotFound = gperr.New("not found")

func main() {
	err := ErrNotFound.Subject("route api").Subject("provider docker")
	fmt.Println(ansi.StripANSI(err.Error()))
	fmt.Println(errors.Is(err, ErrNotFound))

	// PrependSubject works on any error, including nil (returns nil).
	fmt.Println(ansi.StripANSI(gperr.PrependSubject(io.EOF, "reading").Error()))

	fmt.Printf("%q\n", gperr.New("boom").Subject("db").Error())
}
```

Output:

```text
provider docker > route api: not found
true
reading: EOF
"\x1b[91m\x1b[1mdb\x1b[0m: boom"
```

- Subjects keep the error's identity, so `errors.Is(err, ErrNotFound)` still matches.
  Identity is by value: `errors.Is(gperr.New("x"), gperr.New("x"))` is `false`.
  Declare sentinels once, as above.
- `Subjectf` formats the subject with `fmt.Sprintf`; `PrependSubject` accepts any
  `~string` type.
- A subject that begins with `[`, `(`, or `{` is treated as an index and joined to
  the next subject added instead of standing alone: wrapping an error that already
  has the subject `name` first in `[0]` and then in `items` renders as
  `items[0] > name: bad`.
- Do not pass an empty subject. The `Error` interface says empty subjects are
  ignored, but on an error without a subject `Subject("")` renders an empty
  highlight followed by `: message`.
- On an error created by `Wrap(err, msg)`, a subject is placed inside the message:
  `Wrap(io.EOF, "decode").Subject("a.json")` renders as `decode: a.json: EOF`.

## Nested errors

`With` and `Withf` attach sub-errors. The text form is a tree with `•` bullets and
two spaces of indent per level, and a trailing newline:

```go
package main

import (
	"fmt"

	gperr "github.com/yusing/goutils/errs"
)

func main() {
	err := gperr.New("deploy failed").
		With(gperr.New("timeout").Subject("web")).
		With(gperr.New("rollback failed").With(gperr.New("disk full"))).
		Withf("%d services affected", 2)
	fmt.Print(string(gperr.Plain(err)))
}
```

Output:

```text
deploy failed
  • web: timeout
  • rollback failed
    • disk full
  • 2 services affected
```

`errors.Is` and `errors.As` search the main error and every sub-error. `With(nil)`
returns the receiver unchanged.

## Collecting several errors

### Builder

`NewBuilder(context)` accumulates errors:

- `Add`, `AddRange`, `AddSubject`, and `AddSubjectf` ignore `nil` errors. `Adds(string)`
  and `Addf(format, args...)` always add one.
- `AddFrom(other, flatten)` merges another builder, either as one nested entry or
  flattened into this builder.
- `HasError()`, `ForEach(fn)`, `About()`, and `String()` inspect the contents.
- `Error()` returns `nil` when empty. With a context it renders the context line
  followed by the bulleted errors. With an empty context and one error it returns that
  error alone, and with several it renders one error per line, with no header or
  bullets.
- `*Builder` is **not** an `error` (its `Error()` returns an `Error`), so return
  `builder.Error()`, not the builder. Builders are not safe for concurrent use.

`Collect(&builder, fn, arg)` calls `fn(arg)`, records its error in the builder, and
returns the result:

```go
n := gperr.Collect(&errs, strconv.Atoi, "12") // 12; failures land in errs
```

### Group

`Group` is a concurrency-safe `Builder` with goroutine helpers. Unlike
`golang.org/x/sync/errgroup`, it runs every function to completion and reports all
errors, with no cancellation on the first one. Error order follows completion order.

```go
package main

import (
	"errors"
	"fmt"

	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/strings/ansi"
)

func main() {
	g := gperr.NewGroup("checks failed")
	for _, name := range []string{"db", "cache", "queue"} {
		g.Go(func() error {
			if name == "cache" {
				return errors.New("connection refused")
			}
			return nil
		})
	}
	err := g.Wait().Error() // Wait returns the *Builder
	fmt.Print(ansi.StripANSI(err.Error()))
}
```

Output:

```text
checks failed
  • connection refused
```

`Group.Add` and `Group.Addf` may be called from the goroutines. Read the result only
after `Wait`.

### Multiline

`Multiline()` builds an error from indented text lines; indentation sets the nesting
depth. Use `Adds`, `Addf`, `AddStrings`, or `AddLines` (which also accepts errors,
`fmt.Stringer` values, and anything else via `%v`). It renders as an empty string
until a line is added.

```go
m := gperr.Multiline()
m.AddStrings("line 1", "  line 2", "    line 3", "  line 4", "line 5")
fmt.Print(m.Error())
// line 1
//   • line 2
//     • line 3
//   • line 4
// line 5
```

## Hints

`DoYouMean(s)` returns a hint error (`Do you mean s?`) to attach with `With`. It
returns `nil` for an empty string. `NearestField(input, candidates)` finds the closest
candidate by Levenshtein distance, and `DoYouMeanField(input, candidates)` combines the
two, returning nil if no candidate is available. `candidates` may be a `[]string`, a map with string keys, or a struct (or pointer
to one):

```go
package main

import (
	"fmt"

	gperr "github.com/yusing/goutils/errs"
	"github.com/yusing/goutils/strings/ansi"
)

type Config struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

func main() {
	err := gperr.New("unknown field").
		Subject("hots").
		With(gperr.DoYouMeanField("hots", Config{}))
	fmt.Print(ansi.StripANSI(err.Error()))
}
```

Output:

```text
hots: unknown field
  • Do you mean host?
```

For structs the candidate is the `json` tag value, or the field name when there is no
tag. The tag is used verbatim, so `json:"port,omitempty"` yields `port,omitempty`,
`json:"-"` yields `-`, and unexported fields are included. Use a plain tag name or pass a
`[]string` when that matters. Any other candidate type panics.

## Rendering

Every `Error` renders three ways:

| Call | Subject highlight | Intended for |
| --- | --- | --- |
| `err.Error()` / `Normal(err)` | ANSI bright red and bold | Terminals |
| `err.Plain()` / `Plain(err)` | none | Logs, JSON, HTTP bodies, files |
| `err.Markdown()` / `Markdown(err)` | `**bold**`, bullets as hyphen-space list items | Chat, issue trackers |

`Plain`, `Markdown`, and `Normal` also accept ordinary errors and return `nil` for a
`nil` error. For an error with no `Plain` method, `Plain` falls back to `Error()`.
`Markdown` renders a multi-error (`Unwrap() []error`) as a bulleted list.

Errors implement `json.Marshaler`. A plain error is a string; an error with subjects is
`{"subjects":["outer","inner"],"err":"message"}` (outermost first); an error with
sub-errors is `{"err":"main","extras":[...]}`.

## When to use it

Use `errs` for errors shown to people, such as CLI output, API error responses, and
configuration validation, where you want many failures with context in one message.
For errors that only flow through code, plain `fmt.Errorf("...: %w", err)` and
`errors.Join` from the standard library are simpler and add no dependency.

## API reference

```sh
go doc -all github.com/yusing/goutils/errs
```
