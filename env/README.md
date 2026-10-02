# goutils/env

Typed environment-variable getters with defaults and a prefix fallback chain. Each
lookup tries several names for one setting, such as `GODOXY_PORT`, then `GOPROXY_PORT`,
then `PORT`, and takes the first non-empty value.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/env"
```

The package name is `env`. It uses only the standard library and needs Go 1.27 or newer.
The `env/godoxy` subpackage is specific to the GoDoxy server (see
[GoDoxy server settings](#godoxy-server-settings)).

## Quick start

```go
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/yusing/goutils/env"
)

func main() {
	os.Setenv("GODOXY_HTTP_ADDR", "127.0.0.1:8080")
	os.Setenv("TIMEOUT", "1500ms")

	addr, host, port, url := env.GetAddrEnv("HTTP_ADDR", ":80", "http")
	fmt.Println(addr, host, port, url)
	fmt.Println(env.GetEnvDuation("TIMEOUT", time.Second))
	fmt.Println(env.GetEnvInt("WORKERS", 4))
	fmt.Println(env.GetEnvBool("DEBUG", false))
	fmt.Println(env.GetEnvCommaSep("HOSTS", "a, b"))
}
```

Output:

```text
127.0.0.1:8080 127.0.0.1 8080 http://127.0.0.1:8080
1.5s
4
false
[a b]
```

## Lookup rules

For a key such as `PORT`, the package reads these names in order and returns the first
**non-empty** value:

1. `GODOXY_PORT`
2. `GOPROXY_PORT`
3. `PORT`

A variable that is set to an empty string does not shadow a later name, and the typed
getters treat an empty value the same as an unset one by returning their default.

These default prefixes are GoDoxy's. In your own application, call `SetPrefixes` once at
startup, before any goroutine reads configuration:

```go
env.SetPrefixes("MYAPP_", "") // MYAPP_PORT first, then PORT
```

`SetPrefixes` replaces the whole list, in priority order. Include the separator in each
prefix, and use `""` to keep the bare name. Calling it with no arguments leaves only
the bare name. It is not safe for concurrent use.

`LookupEnv(key)` returns the chosen value and whether any candidate was set at all (so
`("", true)` means the variable exists but is empty everywhere). `LookupEnvSource(key)`
also returns the name that supplied the value, or an empty source when nothing non-empty
was found.

## Typed getters

| Function | Parses with | Notes |
| --- | --- | --- |
| `GetEnvString(key, def)` | none | |
| `GetEnvBool(key, def)` | `strconv.ParseBool` | Accepts `1 t T TRUE true True 0 f F FALSE false False`; anything else, such as `yes`, is an error. |
| `GetEnvInt(key, def)` | `strconv.Atoi` | |
| `GetEnvDuation(key, def)` | `time.ParseDuration` | Go duration text such as `1500ms`. The misspelling is the real name. |
| `GetEnvCommaSep(key, def)` | `strings.Split` on `,` | Trims each item and keeps empty ones; an empty value gives `[""]`, not an empty slice. |
| `GetAddrEnv(key, def, scheme)` | `net.SplitHostPort`, `strconv.Atoi` | Returns `addr, host, port, fullURL` where `fullURL` is `scheme://host:port`. All results are zero when both the variable and the default are empty. |
| `GetEnv[T](key, def, parser)` | your `func(string) (T, error)` | For any other type. |

```go
size := env.GetEnv("CACHE_SIZE", int64(1<<20), func(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
})
```

Two details of `GetAddrEnv`: a bare port such as `:8080` gives an empty host and the URL
`http://:8080`, and IPv6 hosts lose their brackets, so `[::1]:8080` gives the invalid URL
`http://::1:8080`. Build the URL yourself for those.

## Errors

A value that fails to parse is a configuration error: the getter logs
`env KEY: invalid bool value` (the key without prefix, and the Go type) with the standard
`log` package and then panics with the same message. The message never contains the
offending value, so it is safe for secrets. Read configuration at startup so a bad value
stops the process early.

## GoDoxy server settings

The `env/godoxy` package defines the GoDoxy server's environment contract on top of this
package and is not meant for other applications:

- `Definitions()` returns the registry of settings (name, type, default, description,
  whether it is sensitive, development-only, or a Compose input). The GoDoxy repository
  generates its `.env.example` and wiki environment table from it. After changing a
  definition, run `shadowtree gen-env-docs` there; `shadowtree check-env-docs` and CI
  reject drift.
- `String`, `Int`, `Duration`, `CommaSep`, `Address`, and `Bool` read a registered setting
  with the registry's default and panic for a name that is not registered. `Bool` derives
  `TEST` (also true in Go test executables), `DEBUG` (defaults to `TEST`, an explicit
  `false` wins), `TRACE` (requires `DEBUG`), and `SERVER_DEBUG` / `WEBSOCKET_DEBUG`
  (also on when `DEBUG` is explicitly `true`) at runtime.
- `Inspect()` returns `Diagnostics` for startup logging: each setting with its source and
  safe value (`[redacted]` for sensitive ones), names of unprefixed variables that
  supplied a value (`Deprecated`), and unrecognized `GODOXY_` variables (`Unknown`). It
  never includes the values of sensitive or unknown variables. Call it once after your
  logger is initialized.

These accessors use the same prefix list as `env`, so `SetPrefixes` affects them too.
The GoDoxy server still accepts unprefixed names but warns that they are deprecated.

## API reference

```sh
go doc -all github.com/yusing/goutils/env
go doc -all github.com/yusing/goutils/env/godoxy
```
