# goutils/strings/ansi

ANSI escape-code helpers: a few color constants, wrappers for error, success, warning,
and info text, and a function that strips the codes again.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/strings/ansi"
```

The package name is `ansi`. It uses only the standard library and needs Go 1.27 or newer.

## Quick start

```go
package main

import (
	"fmt"

	"github.com/yusing/goutils/strings/ansi"
)

func main() {
	msg := ansi.Error("disk full")
	fmt.Printf("%q\n", msg)
	fmt.Println(ansi.StripANSI(msg))
	fmt.Printf("%q\n", ansi.WithANSI("ok", ansi.BrightGreen))
}
```

Output:

```text
"\x1b[91m\x1b[1mdisk full\x1b[0m"
disk full
"\x1b[92mok\x1b[0m"
```

## Functions

| Function | Result |
| --- | --- |
| `Error(s)` | `s` in bold bright red (`HighlightRed`) |
| `Success(s)` | `s` in bold bright green (`HighlightGreen`) |
| `Warning(s)` | `s` in bold bright yellow (`HighlightYellow`) |
| `Info(s)` | `s` in bold bright cyan (`HighlightCyan`) |
| `WithANSI(s, code)` | `code + s + Reset` for any escape sequence you pass |
| `StripANSI(s)` | `s` without color and style sequences |

Every wrapper ends with `Reset`, which clears all attributes. Wrapping text that
already contains styled text therefore ends the outer style at the first inner `Reset`.

## Constants

| Constant | Value |
| --- | --- |
| `BrightRed`, `BrightGreen`, `BrightYellow`, `BrightCyan`, `BrightWhite` | `\x1b[91m`, `\x1b[92m`, `\x1b[93m`, `\x1b[96m`, `\x1b[97m` |
| `Bold` | `\x1b[1m` |
| `Reset` | `\x1b[0m` |
| `HighlightRed`, `HighlightGreen`, `HighlightYellow`, `HighlightCyan`, `HighlightWhite` | The matching bright color followed by `Bold`, for example `\x1b[91m\x1b[1m` |

There are no constants for other colors. Pass the code you need to `WithANSI`, for
example `ansi.WithANSI("note", "\x1b[94m")` for bright blue.

## Behavior and limits

- The package always emits the codes. It does not check whether the output is a
  terminal or honor `NO_COLOR`, so decide that in your program before calling it.
- `StripANSI` removes only sequences matching `\x1b\[[0-9;]*m` (colors and styles).
  Cursor movement, screen clearing, and other control sequences are left in place.
- Other `goutils` packages emit these codes too. For instance, the text of an `errs`
  error with a subject contains them; use its `Plain` form or `StripANSI` for logs,
  files, and HTTP responses.
