# Errors: gperr

Import as `gperr "github.com/yusing/goutils/errs"` (root module). `gperr.Error` is an `error` with
subjects (what failed), nested sub-errors, and plain, ANSI, and Markdown renderings. Use it when a
caller or user needs to see which item failed or many failures at once, such as config validation.
Use the standard `errors` package for sentinels and simple wrapping.

```go
var ErrInvalidPort = gperr.New("invalid port") // sentinel: compare with errors.Is

func validate(routes map[string]Route) error {
	b := gperr.NewBuilder("route validation failed")
	for name, r := range routes {
		if r.Port <= 0 {
			b.Add(ErrInvalidPort.Subject(name).Withf("got %d", r.Port))
		}
	}
	return b.Error() // nil when nothing was added
}
```

The returned error's `Error()` renders a header and one indented `• ` bullet per sub-error.

## Rules

- `(*Builder).Error()` returns the aggregate, or nil when empty. A `*Builder` is not an `error`,
  and it is not safe for concurrent use. Use `gperr.NewGroup(context)` with `Go`/`Add` and
  `Wait()` (which returns `*Builder`) to collect from goroutines; it does not cancel on the first
  error.
- `New("")`, `Wrap(nil)`, `Unwrap(nil)`, an all-nil `Join`, and an empty builder all return nil.
- `err.Subject("a").Subject("b")` renders `b > a: message` (the innermost subject highlighted).
  Subjects starting with `[`, `(`, or `{` attach to the next subject, giving `items[0] > name: …`.
  `gperr.PrependSubject(err, subject)` does the same for any `error`. An empty subject on an error
  without subjects renders a stray `: `, so skip empty subjects yourself.
- `Wrap(err, "decode")` renders `decode: <err>`. `Errorf` supports `%w`; `errors.Is` and
  `errors.As` see through `Wrap`, `With`, subjects, and builders.
- Identity is by instance: `errors.Is(gperr.New("x"), gperr.New("x"))` is false. Declare sentinels
  once.
- `Error()` includes ANSI color codes. For logs, files, and API responses use `gperr.Plain(err)`
  (or `err.Plain()`); use `gperr.Markdown(err)` for Markdown. `apitypes.Error` already prefers
  `Plain()`.
- Errors marshal to JSON: a plain error is a string, an error with subjects is
  `{"subjects":[…],"err":"…"}`, and a nested error is `{"err":…,"extras":[…]}`.
- `gperr.Multiline()` builds a multi-line message (`Adds`, `Addf`, `AddLines`). `Hint`,
  `DoYouMean(s)`, and `DoYouMeanField(input, structOrSlice)` produce "did you mean" suggestions
  for unknown keys; `NearestField` panics on types other than structs, maps, and `[]string`.
- `gperr.Collect(&b, fn, arg)` calls `fn(arg)`, adds a non-nil error to the builder, and returns
  the value.
