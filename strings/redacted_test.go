package strutils

import (
	"encoding/json"
	"strings"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

type testRedacted struct {
	Value Redacted `json:"value"`
}

func TestRedacted(t *testing.T) {
	t.Run("marshal", func(t *testing.T) {
		tests := []struct {
			name     string
			input    string
			expected string
		}{
			{name: "short", input: "test", expected: `{"value":"t**t"}`},
			{name: "medium", input: "testtest", expected: `{"value":"te****st"}`},
			{name: "long", input: "testtesttesttest", expected: `{"value":"te************st"}`},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				v := testRedacted{Value: Redacted(tt.input)}
				got, err := json.Marshal(v)
				expect.NoError(t, err)
				expect.Equal(t, string(got), tt.expected)
			})
		}
	})

	t.Run("unmarshal", func(t *testing.T) {
		var v testRedacted
		expect.NoError(t, json.Unmarshal([]byte(`{"value": "test"}`), &v))
		expect.Equal(t, v.Value.String(), "test")
	})
}

func BenchmarkRedact(b *testing.B) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "Short", input: "test"},
		{name: "Medium", input: "testtest"},
		{name: "StaticBoundary", input: strings.Repeat("a", 67)},
		{name: "LongBoundary", input: strings.Repeat("a", 68)},
		{name: "Long", input: strings.Repeat("a", 256)},
		{name: "Large", input: strings.Repeat("a", 4096)},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			for b.Loop() {
				_ = Redact(tt.input)
			}
		})
	}
}
