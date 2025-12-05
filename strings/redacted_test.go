package strutils

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
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
				require.NoError(t, err)
				require.Equal(t, tt.expected, string(got))
			})
		}
	})

	t.Run("unmarshal", func(t *testing.T) {
		var v testRedacted
		require.NoError(t, json.Unmarshal([]byte(`{"value": "test"}`), &v))
		require.Equal(t, "test", v.Value.String())
	})
}
