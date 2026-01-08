# ansi

ANSI color code utilities for terminal output formatting.

## Overview

The `ansi` package provides constants and functions for applying ANSI escape codes to terminal text. It includes predefined color combinations for common use cases (errors, warnings, success messages) and utilities for stripping ANSI codes from strings.

## Constants

### Basic Colors

```go
const (
    BrightRed    = "\x1b[91m"
    BrightGreen  = "\x1b[92m"
    BrightYellow = "\x1b[93m"
    BrightCyan   = "\x1b[96m"
    BrightWhite  = "\x1b[97m"
    Bold         = "\x1b[1m"
    Reset        = "\x1b[0m"
)
```

### Highlighted Colors (Bold + Color)

```go
const (
    HighlightRed    = BrightRed + Bold
    HighlightGreen  = BrightGreen + Bold
    HighlightYellow = BrightYellow + Bold
    HighlightCyan   = BrightCyan + Bold
    HighlightWhite  = BrightWhite + Bold
)
```

## API Reference

### Styled Output Functions

#### Error

```go
func Error(s string) string
```

Returns the string wrapped in HighlightRed (bright red + bold).

#### Success

```go
func Success(s string) string
```

Returns the string wrapped in HighlightGreen (bright green + bold).

#### Warning

```go
func Warning(s string) string
```

Returns the string wrapped in HighlightYellow (bright yellow + bold).

#### Info

```go
func Info(s string) string
```

Returns the string wrapped in HighlightCyan (bright cyan + bold).

### Low-Level Functions

#### WithANSI

```go
func WithANSI(s string, ansi string) string
```

Wraps a string with arbitrary ANSI codes. The ANSI code is prepended and Reset is appended.

#### StripANSI

```go
func StripANSI(s string) string
```

Removes all ANSI escape codes from a string, returning plain text.

## Usage Examples

### Basic Styling

```go
package main

import (
    "fmt"
    "github.com/yusing/goutils/strings/ansi"
)

func main() {
    fmt.Println(ansi.Error("This is an error message"))
    fmt.Println(ansi.Success("Operation completed successfully"))
    fmt.Println(ansi.Warning("This is a warning"))
    fmt.Println(ansi.Info("Here is some information"))
}
```

Output:

```
[1;91mThis is an error message[0m
[1;92mOperation completed successfully[0m
[1;93mThis is a warning[0m
[1;96mHere is some information[0m
```

### Custom ANSI Codes

```go
// Use basic color without bold
fmt.Println(ansi.WithANSI("Blue text", ansi.BrightBlue))

// Combine multiple styles
customStyle := ansi.BrightWhite + ansi.Bold
fmt.Println(ansi.WithANSI("Bold white", customStyle))
```

### Stripping ANSI Codes

```go
coloredText := ansi.Error("Error message")
plainText := ansi.StripANSI(coloredText)
// plainText == "Error message"
```

### Logging with Colors

```go
import "github.com/rs/zerolog"

func logMessage(level string, message string) {
    switch level {
    case "error":
        zerolog.Error().Msg(ansi.Error(message))
    case "warn":
        zerolog.Warn().Msg(ansi.Warning(message))
    case "info":
        zerolog.Info().Msg(ansi.Info(message))
    case "success":
        zerolog.Info().Msg(ansi.Success(message))
    }
}
```

### Predefined Constants Reference

| Constant          | ANSI Code         | Description          |
| ----------------- | ----------------- | -------------------- |
| `BrightRed`       | `\x1b[91m`        | Bright red text      |
| `BrightGreen`     | `\x1b[92m`        | Bright green text    |
| `BrightYellow`    | `\x1b[93m`        | Bright yellow text   |
| `BrightCyan`      | `\x1b[96m`        | Bright cyan text     |
| `BrightWhite`     | `\x1b[97m`        | Bright white text    |
| `Bold`            | `\x1b[1m`         | Bold formatting      |
| `Reset`           | `\x1b[0m`         | Reset all formatting |
| `HighlightRed`    | `\x1b[91m\x1b[1m` | Bold bright red      |
| `HighlightGreen`  | `\x1b[92m\x1b[1m` | Bold bright green    |
| `HighlightYellow` | `\x1b[93m\x1b[1m` | Bold bright yellow   |
| `HighlightCyan`   | `\x1b[96m\x1b[1m` | Bold bright cyan     |
| `HighlightWhite`  | `\x1b[97m\x1b[1m` | Bold bright white    |

## Implementation Details

The package uses:

- `regexp.MustCompile` to create a pattern for stripping ANSI codes: `\x1b\[[0-9;]*m`
- String concatenation to combine ANSI codes with text
- The Reset code (`\x1b[0m`) to restore default terminal formatting after styled text

## Compatibility

ANSI escape codes are supported by:

- Most terminal emulators (iTerm2, Terminal.app, GNOME Terminal, etc.)
- Console2, Windows Terminal
- Some IDE terminals
- Not supported in standard Windows cmd.exe without ANSI.sys

For cross-platform compatibility, consider using a library like `github.com/mattn/go-colorable` or `github.com/mattn/go-isatty`.
