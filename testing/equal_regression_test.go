package expect

import (
	"math"
	"testing"
)

func TestEqualValuesNumericRegression(t *testing.T) {
	type namedInt int64
	for _, tc := range []struct {
		name string
		a, b any
		want bool
	}{
		{"integer types", int8(42), uint64(42), true}, {"named integer", namedInt(42), float64(42), true}, {"float types", float32(1.5), float64(1.5), true},
		{"fractional", int64(1), float64(1.5), false}, {"signed unsigned", int64(-1), uint64(math.MaxUint64), false},
		{"large integer rounded", int64(1<<53 + 1), float64(1 << 53), false}, {"large unsigned rounded", uint64(math.MaxUint64), float64(math.MaxUint64), false},
		{"large integer exact", int64(1 << 53), float64(1 << 53), true}, {"float32 rounding", int32(1<<24 + 1), float32(1 << 24), false},
		{"float64 rounding", float64(1.00000001), float32(1), false}, {"complex rounding", complex128(1.00000001 + 2i), complex64(1 + 2i), false},
		{"complex exact", complex128(1 + 2i), complex64(1 + 2i), true}, {"nan", math.NaN(), math.NaN(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := equalValues(tc.a, tc.b); got != tc.want {
				t.Errorf("equalValues(%#v, %#v) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
			if got := equalValues(tc.b, tc.a); got != tc.want {
				t.Errorf("reverse equalValues(%#v, %#v) = %v, want %v", tc.b, tc.a, got, tc.want)
			}
		})
	}
}
