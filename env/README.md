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
`("", true)` means the variable exists but is empty everywhere).

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

A bare port such as `:8080` gives an empty host and the URL `http://:8080`. IPv6
URLs keep the required brackets: `[::1]:8080` gives `http://[::1]:8080`.

## Errors

A value that fails to parse is a configuration error: the getter logs
`env KEY: invalid bool value: VALUE` (the key without prefix, the Go type, and the
supplied value) with the standard `log` package, then panics with the same message.
Do not put secrets in malformed typed values: the diagnostic includes them.
Invalid booleans are intentionally rejected, including `DEBUG=app:*` in importers
of `server` or `http/websocket`, which read their flags during package initialization.

## API reference

```sh
go doc -all github.com/yusing/goutils/env
```
