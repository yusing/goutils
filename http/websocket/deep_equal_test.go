package websocket

import (
	"math"
	"testing"
)

type deepEqualBenchPayload struct {
	Name   string
	Active bool
	Count  int
	Values []int
}

var (
	deepEqualBenchStringA any = "websocket payload"
	deepEqualBenchStringB any = "websocket payload"
	deepEqualBenchIntA        = 42
	deepEqualBenchIntB        = 42
	deepEqualBenchStructA     = deepEqualBenchPayload{Name: "alpha", Active: true, Count: 42, Values: []int{1, 2, 3}}
	deepEqualBenchStructB     = deepEqualBenchPayload{Name: "alpha", Active: true, Count: 42, Values: []int{1, 2, 3}}
)

func TestDeepEqualScalars(t *testing.T) {
	tests := []struct {
		name string
		x    any
		y    any
		want bool
	}{
		{name: "equal strings", x: "same", y: "same", want: true},
		{name: "different strings", x: "same", y: "different", want: false},
		{name: "equal ints", x: 42, y: 42, want: true},
		{name: "different ints", x: 42, y: 43, want: false},
		{name: "different numeric types", x: int64(42), y: uint64(42), want: false},
		{name: "nan equals nan", x: math.NaN(), y: math.NaN(), want: true},
		{name: "nil equals nil", x: nil, y: nil, want: true},
		{name: "nil differs from zero", x: nil, y: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeepEqual(tt.x, tt.y); got != tt.want {
				t.Fatalf("DeepEqual(%v, %v) = %v, want %v", tt.x, tt.y, got, tt.want)
			}
		})
	}
}

func TestDeepEqualComposite(t *testing.T) {
	type sample struct {
		Name   string
		Values []int
		hidden string
	}

	a := sample{Name: "alpha", Values: []int{1, 2, 3}, hidden: "left"}
	b := sample{Name: "alpha", Values: []int{1, 2, 3}, hidden: "right"}
	c := sample{Name: "alpha", Values: []int{1, 2, 4}, hidden: "left"}

	if !DeepEqual(a, b) {
		t.Fatal("DeepEqual should ignore unexported struct fields")
	}
	if DeepEqual(a, c) {
		t.Fatal("DeepEqual should compare exported slice contents")
	}
}

func BenchmarkDeepEqualStringEqual(b *testing.B) {
	for b.Loop() {
		_ = DeepEqual(deepEqualBenchStringA, deepEqualBenchStringB)
	}
}

func BenchmarkDeepEqualIntEqual(b *testing.B) {
	for b.Loop() {
		_ = DeepEqual(deepEqualBenchIntA, deepEqualBenchIntB)
	}
}

func BenchmarkDeepEqualStructEqual(b *testing.B) {
	for b.Loop() {
		_ = DeepEqual(deepEqualBenchStructA, deepEqualBenchStructB)
	}
}
