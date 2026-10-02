# goutils/mockable

A replaceable clock for tests. Code under test calls `mockable.TimeNow()` instead of
`time.Now()`, and a test swaps in a fixed time.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/mockable"
```

The package name is `mockable`. It uses only the standard library and needs Go 1.27 or
newer.

## API

```go
var TimeNow = time.Now           // call this instead of time.Now
func MockTimeNow(t time.Time)    // make TimeNow always return t
```

## Quick start

Use `mockable.TimeNow()` in the code you want to control, and fix the time in the test:

```go
package stamp

import (
	"testing"
	"time"

	"github.com/yusing/goutils/mockable"
)

func Stamp() string {
	return mockable.TimeNow().UTC().Format(time.DateOnly)
}

func TestStamp(t *testing.T) {
	original := mockable.TimeNow
	t.Cleanup(func() { mockable.TimeNow = original })

	mockable.MockTimeNow(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))
	if got := Stamp(); got != "2024-01-01" {
		t.Fatalf("Stamp() = %q", got)
	}
}
```

## Behavior and limits

- `TimeNow` is a plain package variable. `MockTimeNow(t)` replaces it with a function that
  always returns `t`, so the clock stands still. To advance time, assign your own function
  to `mockable.TimeNow`.
- Nothing restores the real clock for you. Save the original and put it back, as in the
  example, or later tests will see the fixed time.
- The variable is global and unsynchronized. Do not change it from parallel tests or while
  other goroutines read it.
- Only code that calls `mockable.TimeNow()` is affected. The standard library, timers, and
  other packages still use the real clock, including `strings.NewUUIDv7`.
- The mocked value carries no monotonic clock reading, unlike `time.Now()`.
