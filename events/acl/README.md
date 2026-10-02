# goutils/events/acl

Records concurrent ACL block diagnostics once per active IP operation. Events are added to the history
attached to the context; without a history the helper does nothing.

## Module

`github.com/yusing/goutils/events/acl` is a separate Go module. It owns the
`golang.org/x/sync/singleflight` dependency; core event history remains in the
root utility module. Import paths and event contents are unchanged.
Repository consumers use local replacements during development.
