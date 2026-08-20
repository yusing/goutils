package strutils

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestJSONDurationRoundTrip(t *testing.T) {
	type payload struct {
		D time.Duration `json:"d"`
	}
	in := payload{D: time.Second}
	b, err := MarshalJSON(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"d":1000000000}`, string(b))

	var out payload
	require.NoError(t, UnmarshalJSON(b, &out))
	require.Equal(t, time.Second, out.D)

	s, err := MarshalString(in)
	require.NoError(t, err)
	require.JSONEq(t, `{"d":1000000000}`, s)
	require.False(t, strings.HasSuffix(s, "\n"))

	var fromString payload
	require.NoError(t, UnmarshalFromString(s, &fromString))
	require.Equal(t, time.Second, fromString.D)
}

func TestJSONStringAndValid(t *testing.T) {
	s, err := MarshalString(map[string]int{"a": 1})
	require.NoError(t, err)
	require.JSONEq(t, `{"a":1}`, s)
	require.True(t, ValidJSONString(s))
	require.True(t, ValidJSON([]byte(s)))
	require.False(t, ValidJSONString("{"))
	require.False(t, ValidJSONString(""))

	var got map[string]int
	require.NoError(t, UnmarshalFromString(s, &got))
	require.Equal(t, 1, got["a"])
	require.Error(t, UnmarshalFromString("{", &got))
}

func TestJSONEncoderDecoder(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, NewJSONEncoder(&buf).Encode(map[string]int{"n": 2}))
	require.True(t, strings.HasSuffix(buf.String(), "\n"), "streaming encode should end with a newline")
	require.JSONEq(t, `{"n":2}`, strings.TrimSpace(buf.String()))

	var got map[string]int
	require.NoError(t, NewJSONDecoder(&buf).Decode(&got))
	require.Equal(t, 2, got["n"])
}

func TestJSONMarshalIndent(t *testing.T) {
	b, err := MarshalJSONIndent(map[string]int{"a": 1}, "", "  ")
	require.NoError(t, err)
	require.Equal(t, "{\n  \"a\": 1\n}", string(b))
}
