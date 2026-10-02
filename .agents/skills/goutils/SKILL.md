---
name: goutils
description: Use github.com/yusing/goutils packages in a Go project, including choosing the package and module, wiring task lifetimes, errors, buffers, logging, and HTTP servers or proxies.
disable-model-invocation: false
---

# Using goutils

goutils is a set of Go 1.27 utility packages split across several modules. The root module
(`github.com/yusing/goutils`) depends only on xsync and x/text; HTTP, server, cache, and
blocked-event helpers are separate modules that bring their own dependencies. There is no package
at the module root: import the package you need.

## Choosing a package

The import path's last element is not always the package name. Use the package name shown here
when referring to identifiers.

| Import path (`github.com/yusing/goutils/...`) | Package | Module to require | Reference |
| --- | --- | --- | --- |
| `task` | `task` | root | `references/lifecycle.md` |
| `eventqueue` | `eventqueue` | root | `references/lifecycle.md` |
| `pool` | `pool` | root | `references/lifecycle.md` |
| `events` | `events` | root | `references/lifecycle.md` |
| `events/acl`, `events/http` | `aclevents`, `httpevents` | own module each | `references/lifecycle.md` |
| `synk`, `synk/workerpool` | `synk`, `workerpool` | root | `references/lifecycle.md` |
| `logging` | `logging` | root | this file |
| `errs` | `gperr` | root | `references/errors.md` |
| `strings`, `strings/ansi` | `strutils`, `ansi` | root | `references/utilities.md` |
| `io` | `ioutils` | root | `references/utilities.md` |
| `env`, `fs`, `num`, `intern`, `version`, `mockable` | same | root | `references/utilities.md` |
| `apitypes` | `apitypes` | root | `references/utilities.md` |
| `testing` | `expect` | root | `references/testing.md` |
| `cache` | `cache` | `cache` | `references/utilities.md` |
| `http`, `http/httpheaders`, `http/accesslog` | `httputils`, `httpheaders`, `accesslog` | `http` | `references/http.md` |
| `http/reverseproxy` | `reverseproxy` | `http/reverseproxy` | `references/http.md` |
| `http/websocket` | `websocket` | `http/websocket` | `references/http.md` |
| `server` | `server` | `server` | `references/http.md` |

Load only the references for the packages the current change touches. Each package directory
also has a README with its full API; `go doc` on the package is authoritative when they differ.

## Adding the dependency

All modules are released together at one version (for example `v0.8.0`). Require the root module
and each nested module whose packages you import, at the same version:

```sh
go get github.com/yusing/goutils@v0.8.0
go get github.com/yusing/goutils/http@v0.8.0   # only if importing http, httpheaders, or accesslog
```

Nested module paths match their import paths (`.../server`, `.../http/websocket`, ...). Mixing
versions across goutils modules is unsupported. `http/reverseproxy/integrationtest` is a test-only
module, never an application dependency.

Binaries and test binaries that link `goutils/http` (directly or through `server` or
`http/reverseproxy`) or `http/websocket` fail to link unless built with
`-ldflags=-checklinkname=0`, because those packages use `go:linkname` into standard library and
dependency internals. Add the flag wherever the project builds or tests (Makefile, Dockerfile,
CI, `GOFLAGS=-ldflags=-checklinkname=0`). `go vet` and compile-only checks pass without it.

## Logging

goutils diagnostics (task shutdown warnings, pool add/remove, HTTP request logs, profiling) go
through `logging` and are silent until the application installs a logger once at startup:

```go
type appLogger struct{ l *slog.Logger }

func (a appLogger) Log(level logging.Level, msg string, fields ...logging.Field) {
	attrs := make([]any, 0, 2*len(fields))
	for _, f := range fields {
		attrs = append(attrs, f.Key, f.Value)
	}
	a.l.Log(context.Background(), slogLevel(level), msg, attrs...)
}

func slogLevel(l logging.Level) slog.Level {
	switch l {
	case logging.Debug:
		return slog.LevelDebug
	case logging.Warn:
		return slog.LevelWarn
	case logging.Error:
		return slog.LevelError
	}
	return slog.LevelInfo
}

func main() {
	logging.SetLogger(appLogger{slog.Default()})
}
```

The logger is called synchronously from many goroutines and must be concurrency-safe.
`SetLogger(nil)` silences diagnostics again. The `server`, `http/reverseproxy`, `http/websocket`,
and `cache` modules still log through zerolog directly; configure zerolog as well when using them.

## Build tags

- `debug`: extra lifecycle logging and stack traces in `task`, `pool`, and `cache`.
- `pprof`: periodic `synk` buffer-pool statistics through `logging`.

Neither is needed for production builds.
