package num

import (
	"math"
	"testing"
)

func TestEncodingPrecision(t *testing.T) {
	totalDiffs := 0.0
	for i := 0.0; i <= 100.0; i += 0.1 {
		p := NewPercentage(i)
		got := p.ToFloat()
		diff := math.Abs(got - i)
		if diff > tolerance {
			t.Errorf("Encoding mismatch: %.1f%% encoded=0x%02X decoded=%.3f diff=%.3f",
				i, p.code, got, diff)
		}
		totalDiffs += diff
	}
	t.Logf("Avg diffs: %.2f", totalDiffs/1000)
}

func TestBoundaries(t *testing.T) {
	tests := []float64{0, 50, 99.9, 100, -5, 120}
	for _, val := range tests {
		p := NewPercentage(val)
		got := p.ToFloat()
		if got < 0 || got > 100 {
			t.Errorf("Out of range: input=%.1f decoded=%.2f", val, got)
		}
	}
}

func TestStringFormat(t *testing.T) {
	p := NewPercentage(42.8)
	if s := p.String(); s[len(s)-1] != '%' {
		t.Errorf("Percentage.String() = %q, missing %% sign", s)
	}
}

func BenchmarkNewPercentage(b *testing.B) {
	for b.Loop() {
		_ = NewPercentage(42.8)
	}
}

func BenchmarkToFloat(b *testing.B) {
	p := NewPercentage(42.8)
	for b.Loop() {
		_ = p.ToFloat()
	}
}
