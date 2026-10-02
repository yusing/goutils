# goutils/events/http

Records concurrent HTTP block diagnostics once per active remote-IP/host operation. Events are added to the history
attached to the context; without a history the helper does nothing.

## Module

`github.com/yusing/goutils/events/http` is a separate Go module. It owns the
`golang.org/x/sync/singleflight` dependency; core event history remains in the
root utility module. Import paths and event contents are unchanged.
Repository consumers use local replacements during development.
