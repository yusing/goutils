# Testing with goutils

## expect assertions

Import as `expect "github.com/yusing/goutils/testing"`. Assertions are fail-fast (`t.Fatal`), so
call them from the test goroutine.

```go
func TestLoad(t *testing.T) {
	cfg, err := Load("testdata/ok.yml")
	expect.NoError(t, err)
	expect.Equal(t, cfg.Port, 8080) // got, then want
	_, err = Load("testdata/bad.yml")
	expect.ErrorIs(t, ErrInvalidPort, err) // expected, then err: reverse of errors.Is
}
```

- `Equal`, `NotEqual`, `ErrorIs`, `ErrorT`, `Contains`, `StringsContain`, and `Type` take
  `*testing.T`. The rest take `testing.TB`, so only those work in benchmarks.
- `Equal` compares deeply and accepts numeric types only when conversions in both directions
  are lossless. A nil slice does not equal an empty slice.
- `expect.Must(v, err)` panics on error, which suits test setup.
- An optional trailing message is a plain string or a format string with arguments.
- Importing the package turns on verbose test output when the test binary is run directly or by
  `go test` in directory mode.

## Tasks in tests

`task.GetTestTask(t)` returns a task bound to `t.Context()`, canceled when the test ends. Use it as
the parent for the code under test. Finish the test task directly to cancel everything under
it, or finish a scoped subtask to stop only one part of the test.

```go
func TestPoller(t *testing.T) {
	scope := task.GetTestTask(t).Subtask("poller-test", true)
	defer scope.FinishAndWait(nil)
	expect.NoError(t, startPoller(scope))
}
```

Code that calls `task.WaitExit` or signals the process does not belong in unit tests.

## Time

`mockable.TimeNow` is a package variable that defaults to `time.Now`, and only goutils code that
reads it is affected. `strutils.NewUUIDv7` uses the standard-library clock instead. Make your own time-dependent code read
`mockable.TimeNow()` if tests need to control it. The variable is global and unsynchronized: do not
mock it in parallel tests, and restore it afterwards:

```go
orig := mockable.TimeNow
mockable.MockTimeNow(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
t.Cleanup(func() { mockable.TimeNow = orig })
```

## Logging in tests

goutils logs nothing until a logger is installed. To see task, pool, or HTTP diagnostics, install
a `logging.Logger` that writes with `t.Log`, and reset it with `t.Cleanup(func() {
logging.SetLogger(nil) })`.
