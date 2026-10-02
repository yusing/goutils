# Utility packages

Covers `strings` (`strutils`), `strings/ansi`, `io` (`ioutils`), `env`, `fs`, `num`, `intern`,
`version`, `mockable`, `apitypes`, and the separate `cache` module. Check the package README for
the full API.

## strutils (`github.com/yusing/goutils/strings`)

- JSON helpers wrap `encoding/json/v2`: `MarshalJSON`, `UnmarshalJSON`, `MarshalJSONIndent`,
  `MarshalString`, `UnmarshalFromString`, `NewJSONEncoder`/`NewJSONDecoder`, `ValidJSON`. Expect
  v2 semantics: case-sensitive field names, a nil slice encodes as `[]` and a nil map as `{}`,
  duplicate names are an error, and `time.Duration` is nanoseconds. Calling `SetIndent` on
  `NewJSONEncoder` makes `Encode` fail; use `MarshalJSONIndent` instead.
- `MarshalYAML`/`UnmarshalYAML` panic until the application installs a YAML library with
  `SetYAMLMarshaler` and `SetYAMLUnmarshaler`.
- Human-readable formatting: `FormatDuration` (drops seconds at one hour or more),
  `FormatByteSize` (binary units, two decimals), `FormatTime`/`FormatLastSeen`/
  `FormatTimeWithReference` (relative times are not singularized: "1 minutes ago"), and `Append*`
  variants that write into a caller buffer.
- Case-insensitive search: `ContainsFold`, `IndexFold`, `HasPrefixFold`, `HasSuffixFold`.
  `Title` uses x/text casing.
- `CommaSeperatedList` (the misspelling is the real name) splits on commas and on whitespace.
- `Redacted` is a string type for secrets in config: it masks in JSON and YAML output (YAML masking
  needs a library that honors `MarshalYAML() ([]byte, error)`, such as goccy/go-yaml), while
  `String()` and `fmt` print the real value. `Redact` reveals values of four bytes or fewer.
- `SanitizeURI` cleans relative redirect targets (returns http(s) URLs unchanged);
  `IsValidFilename` rejects only `/`, `\`, and `..`, so do not use it alone for path safety.
- `NewUUIDv7` has no random bits; use a random UUID library where collision resistance across
  processes matters.
- `Parse[T]`/`MustParse[T]` call a `Parse(string) error` method; instantiate with a pointer type
  (`Parse[*Port]`).

## ansi (`github.com/yusing/goutils/strings/ansi`)

`Error`, `Warning`, `Success`, `Info` wrap text in bold bright colors; `WithANSI` takes a code;
`StripANSI` removes SGR sequences. Nothing detects terminals or `NO_COLOR`; decide at the caller.

## ioutils (`github.com/yusing/goutils/io`)

- `CopyClose(dst, src, sizeHint)`: dst first. It copies with a pooled buffer (hint ≤ 0 means
  32 KiB) and flushes each write when dst is a flushable `http.ResponseWriter` serving a streaming
  response (no Content-Length, or SSE/gRPC). Despite its name, it closes nothing.
- `CopyCloseWithContext(ctx, dst, src, sizeHint)` closes src and then dst (when they implement
  `io.Closer`) only if ctx is canceled, to unblock the copy. Close them yourself after normal
  completion.
- `NewBufferedWriter(w, size)` is a pooled `bufio.Writer`. Its `Close` flushes, returns the buffer
  to the pool, and closes w if w is an `io.Closer`.
- `NewPipe(ctx, r, w).Start()` and `NewBidirectionalPipe(ctx, a, b).Start()` copy until done and
  treat closed-pipe, reset, and canceled errors as success. EOF in one direction does not close
  the other, so cancel ctx to end a bidirectional pipe.
- `NewContextReader`/`NewContextWriter` check ctx between calls but do not interrupt a blocked
  call. `NewHookReadCloser(rc, hook)` runs hook on every `Close`.

## env (`github.com/yusing/goutils/env`)

- Lookups try the prefixes `GODOXY_`, `GOPROXY_`, then the bare key; the first non-empty value
  wins. Call `env.SetPrefixes("MYAPP_", "")` once at startup, before any lookup and before other
  goroutines start, for your own application prefix.
- Typed getters: `GetEnvString`, `GetEnvBool`, `GetEnvInt`, `GetEnvDuation` (misspelled),
  `GetEnvCommaSep`, `GetAddrEnv`, and generic `GetEnv[T](key, def, parser)`. An unparsable value
  panics with a message that names the key but not the value.
- `env/godoxy` is GoDoxy's registry of server settings (also read by the `server` and
  `http/websocket` modules). Use it only when integrating with GoDoxy's environment contract.

## Small types

- `fs.ListFiles(dir, maxDepth, hideHidden...)`: depth 0 lists only the top level. Results are
  `dir`-joined paths, and `hideHidden` applies only to the top level.
- `num.Percentage`: a 1-byte value in [0, 100] with about 0.4 precision. It marshals to a JSON
  number with one decimal.
- `intern.Make(v)` returns a comparable `Handle[T]` that deduplicates repeated values (hostnames,
  names). The zero handle panics on `Value()`. `intern.MakeValue(v)` returns the interned value.
- `version.Parse("v1.2.3")`: the format is `v<gen>.<major>.<minor>` with an optional `-suffix`.
  It never errors, and anything else parses to v0.0.0. `version.Get()` reports the build version
  set with `-ldflags "-X github.com/yusing/goutils/version.version=v1.2.3"`. `IsOlderThan` is
  `!IsNewerThan`, so it is true for equal versions; compare with `IsEqual` first.
- `mockable.TimeNow` is a replaceable `time.Now` (used by `NewUUIDv7`). See `testing.md`.

## apitypes (`github.com/yusing/goutils/apitypes`)

JSON response shapes for gin-style APIs: `apitypes.Error(message, err)` (uses `Plain()` for
gperr errors; passing a nil error panics, so omit it instead), `apitypes.Success(message,
details)`, and `QueryOptions`/`QueryResponse` for paginated lists (`form`/`binding` tags, limit
1-20). `InternalServerError(err, message)` takes the error first and is meant for `c.Error(...)`
with middleware that replies with a generic 500.

## cache (module `github.com/yusing/goutils/cache`)

Memoizes context-aware functions. This module does not depend on the root module.

```go
var getUser = cache.NewKeyFunc(func(ctx context.Context, id string) (User, error) {
	return db.LoadUser(ctx, id)
}).WithTTL(30 * time.Second).WithMaxEntries(1024).Build() // build once, at package or component scope

u, err := getUser(ctx, "42")
```

- TTL 0 (the default) caches forever, and a negative TTL recomputes on every call.
- Errors are cached like values (forever with TTL 0). The exception is an error matching the
  refreshing caller's context cause. Pair error-prone functions with a TTL or retries.
- Refreshes are single-flight per key, and waiting callers do not observe their own context
  deadline.
- `WithRetries*(n)` allows up to `n+1` calls; exponential backoff has no overall time limit.
- Keyed caches grow without bound unless `WithMaxEntries` is set. A process can build at most 32
  bounded keyed caches; the 33rd `Build()` panics, so never build them per request.
- `-tags debug` logs hits and misses, including summarized cached values, through zerolog.
