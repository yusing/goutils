package num

import (
	"fmt"
	"math"

	strutils "github.com/yusing/goutils/strings"
)

// Percentage holds a value in [0, 100] with about 0.4 percentage-point spacing
// and a maximum rounding error of about 0.25 percentage points.
type Percentage struct {
	code uint8
}

// Each table holds 128 evenly spaced values.
// Combined, the tables provide about 251 distinct values after clamping to 100.
var (
	tableA [128]float64
	tableB [128]float64
)

const tolerance = 0.200001

func round1(x float64) float64 {
	return math.Round(x*10) / 10
}

func init() {
	// Sample two interleaved sets from a 1000-step grid.
	step := 100.0 / 999.0 // for 1000 values
	for i := range 128 {
		// Map first table (even indices) and second (odd offset)
		tableA[i] = min(round1(float64(i*8)*step), 100) // spacing ~0.8%
		tableB[i] = min(round1(float64(i*8+4)*step), 100)
	}
}

// NewPercentage constructs a Percentage from a float [0.0, 100.0].
func NewPercentage(f float64) Percentage {
	if f <= 0 {
		return Percentage{0}
	}
	if f >= 100 {
		return Percentage{255}
	}

	step := 100.0 / 999.0
	// Estimate nearest double-table index in 0-255
	rawIndex := f / (8 * step / 2) // half spacing, since tables are interleaved
	code := uint8(math.Round(rawIndex))
	return Percentage{code}
}

// ToFloat restores the float representation (lossless from lookup table)
func (p Percentage) ToFloat() float64 {
	index := p.code >> 1
	if p.code&1 == 0 {
		return tableA[index]
	}
	return tableB[index]
}

func (p Percentage) String() string {
	return fmt.Sprintf("%.1f%%", p.ToFloat())
}

func (p Percentage) MarshalJSON() ([]byte, error) {
	return fmt.Appendf(nil, `%.1f`, p.ToFloat()), nil
}

func (p *Percentage) UnmarshalJSON(data []byte) error {
	var f float64
	err := strutils.UnmarshalJSON(data, &f)
	if err != nil {
		return err
	}
	*p = NewPercentage(f)
	return nil
}
