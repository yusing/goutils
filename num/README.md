# goutils/num

`Percentage`: a percentage between 0 and 100 stored in a single byte, for large
collections of samples (such as metric histories) where memory matters more than
precision.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/num"
```

The package name is `num`. It needs Go 1.27 or newer and uses `goutils/strings` for JSON
decoding.

## Quick start

```go
package main

import (
	"encoding/json"
	"fmt"

	"github.com/yusing/goutils/num"
)

func main() {
	p := num.NewPercentage(75.5)
	fmt.Println(p.ToFloat())
	fmt.Println(p)

	data, _ := json.Marshal(p)
	fmt.Println(string(data))

	var back num.Percentage
	if err := json.Unmarshal(data, &back); err != nil {
		panic(err)
	}
	fmt.Println(back == p)
}
```

Output:

```text
75.7
75.7%
75.7
true
```

`75.5` comes back as `75.7` because the value is quantized, as described next.

## Precision

A `Percentage` holds one of 256 codes that map to about 250 distinct values spaced roughly
0.4 percentage points apart, so a stored value can differ from the input by up to about
0.25 points (`75.5` becomes `75.7`, `1` becomes `0.8`, `99.8` becomes `99.7`). Do not use
it where exact values or tight tolerances matter.

- `NewPercentage(f)` clamps: anything at or below 0 becomes 0 and anything at or above 100
  becomes 100. The result for `NaN` is unspecified.
- The zero value is `0.0%`.
- `Percentage` is a comparable value type, so `==` compares the stored codes. Two inputs
  that quantize to the same value are equal.
- `ToFloat()` returns the quantized value; `NewPercentage(p.ToFloat())` reproduces it.
- `String()` formats it as `%.1f%%`, for example `75.7%`.

## JSON

`Percentage` marshals to a JSON **number** with one decimal (`75.7`), not a string. It
unmarshals from a number and quantizes it with `NewPercentage`. A quoted value such as
`"75.7"` is an error.
