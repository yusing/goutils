# goutils/logging

Framework-neutral diagnostics for goutils. The root module has no logging-framework
dependency.

## Usage

Applications implement `Logger` and install it with `SetLogger`:

```go
package main

import (
    "fmt"
    "github.com/yusing/goutils/logging"
)

type appLogger struct{}

func (appLogger) Log(level logging.Level, message string, fields ...logging.Field) {
    fmt.Println(level, message, fields)
}

func main() {
    logging.SetLogger(appLogger{})
    logging.Log(logging.Info, "ready")
}
```

## Configuration

The default is silent. `SetLogger(nil)` disables diagnostics. Applications own
filtering, formatting, destinations, and error serialization; goutils never
changes a framework's global level. Implementations must handle concurrent calls.
Delivery is synchronous, and replacing a logger does not wait for in-flight calls
to the previous logger to complete.

`Debug`, `Info`, `Warn`, and `Error` describe severity without framework-specific
numeric values. Each `Field` preserves its key and original typed value, including
errors and numeric counters. GoDoxy supplies a zerolog adapter in its own logging
package, so existing application output stays on zerolog.

Task, pool, HTTP, and profiling diagnostics use this interface. Nested Go modules
such as http, server, cache, reverseproxy, and websocket remain independently managed
and may still depend directly on zerolog.

## Dependencies

The root module retains only xsync for concurrent maps and x/text for Unicode
title casing. HTTP/2 dependencies are isolated in the HTTP modules, and x/sync
is isolated in the blocked-event helper modules. Testing helpers use only the
standard library.
