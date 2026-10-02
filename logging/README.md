# goutils/logging

Framework-neutral diagnostics for goutils. Packages such as `task`, `pool`, and
`synk` report shutdown warnings, recovered panics, and pool activity through one
small interface that your application implements. The package imports only the
standard library and never chooses a logging framework, output, or level.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/logging"
```

## Quick start

goutils is silent until the application installs a logger. Install it once at
startup, before starting goroutines that use goutils packages:

```go
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/yusing/goutils/logging"
)

type slogLogger struct{ l *slog.Logger }

func (s slogLogger) Log(level logging.Level, message string, fields ...logging.Field) {
	attrs := make([]any, 0, len(fields))
	for _, f := range fields {
		attrs = append(attrs, slog.Any(f.Key, f.Value))
	}
	s.l.Log(context.Background(), toSlog(level), message, attrs...)
}

func toSlog(level logging.Level) slog.Level {
	switch level {
	case logging.Debug:
		return slog.LevelDebug
	case logging.Warn:
		return slog.LevelWarn
	case logging.Error:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func main() {
	logging.SetLogger(slogLogger{slog.New(slog.NewTextHandler(os.Stderr, nil))})

	logging.Log(logging.Info, "ready", logging.Field{Key: "port", Value: 8080})
	// time=... level=INFO msg=ready port=8080
}
```

The same shape adapts to zerolog, zap, or any other framework. The adapter is
application code; goutils ships none.

## API

| Symbol | Meaning |
| --- | --- |
| `type Logger interface { Log(level Level, message string, fields ...Field) }` | What the application implements. |
| `SetLogger(Logger)` | Installs the logger. `nil` removes it (the default). |
| `Log(level, message, fields...)` | Forwards to the installed logger, or does nothing. Called by goutils packages; applications may use it too. |
| `Level` (`uint8`) | `Debug`, `Info`, `Warn`, `Error`, in increasing severity. They carry no framework-specific numbers. Map them in your adapter. |
| `Field{Key string; Value any}` | A structured value. The original type is preserved, so an `error` arrives as an `error` and a counter as a number. |

## Rules for logger implementations

- `Log` runs synchronously on the goroutine that produced the diagnostic. This
  includes task callback goroutines and the `pool` add/remove paths, so keep it
  fast and do not block on slow I/O.
- It must be safe for concurrent calls.
- Filtering, formatting, destination, and serialization of `error` values belong
  to your logger. goutils never changes a framework's global level.
- `SetLogger` may be called concurrently with `Log`. A call already in flight
  can finish on the previous logger, and replacing the logger does not wait for
  it.
- `Log` never fails or reports whether a logger is installed. A nil or missing
  logger discards the message.
- Messages are plain text, never format strings. Some sources put details in the
  message (`pool`), others in `fields` (`task` panics, `synk` statistics).

In tests, install a recording logger and reset it with
`t.Cleanup(func() { logging.SetLogger(nil) })`. The logger is process-wide, so
tests that install one must not run in parallel with tests that depend on a
different one.

## What goutils reports

| Source | Level | Message and fields | Build |
| --- | --- | --- | --- |
| `task.WaitExit` | `Info` | `shutting down` | all |
| `task` | `Warn` | `<task> stucked callbacks: N, stucked children: M (<cause>)` followed by the names of the stuck children and callbacks | all |
| `task` | `Error` | `panic`, fields `error` and `callback` (callback panic that was recovered) | all |
| `task` | `Info` | `task <full name> started` and `task <full name> finished` for every subtask | `debug` |
| `pool` | `Info` | `<pool>: added \| reloaded \| removed <name>` (`<display> (<name>)` when a display name differs). `Pool.DisableLog(true)` suppresses these. | all |
| `pool` | `Warn` | `<pool>: key <key> already exists` plus a stack trace | `debug` |
| `synk` | `Info` | `bytes pool stats`, fields such as `sizeInUse`, `numReused`, `numDropped`, every 5 s | `pprof` |
| `http` module | `Debug` to `Error` | request helpers `LogDebug`, `LogInfo`, `LogWarn`, and `LogError` add `remote`, `host`, and `uri` fields | all |
| `http` module | `Error` | `write header after response has been created` | all |

See the [task](../task/README.md), [pool](../pool/README.md), and
[synk](../synk/README.md) READMEs for when each message occurs.

## Build tags

`logging` itself has none. Two tags change what other packages emit:

- `-tags debug` adds the per-subtask lifecycle messages in `task` and the
  duplicate-key warning in `pool`. In `task` it also makes a recovered callback
  panic fatal after it is logged. See the task README before using it in
  production.
- `-tags pprof` starts the byte-pool statistics reporter in `synk`.

## Dependencies

Standard library only.
