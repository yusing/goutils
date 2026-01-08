# mockable

Mockable interface utilities for testing.

## Overview

The `mockable` package provides mockable implementations of system functions.

## API Reference

```go
var TimeNow func() time.Time

func MockTimeNow(t time.Time)
```

## Usage

```go
package main

import (
    "time"
    "github.com/yusing/goutils/mockable"
)

func main() {
    // Mock time for testing
    mockable.MockTimeNow(time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC))

    // Now TimeNow returns the mocked time
    now := mockable.TimeNow()
    fmt.Println(now) // 2024-01-01 12:00:00
}
```

## Use Cases

- Time-based testing without sleep
- Reproducible timing scenarios
- Testing timeouts and deadlines
