package strutils

import (
	"bytes"
	jsonv2 "encoding/json/v2"
	"reflect"
	"strings"
	"testing"
	"time"

	expect "github.com/yusing/goutils/testing"
)

func TestJSONDurationRoundTrip(t *testing.T) {
	type payload struct {
		D time.Duration `json:"d"`
	}
	in := payload{D: time.Second}
	b, err := MarshalJSON(in)
	expect.NoError(t, err)
	assertJSONEqual(t, `{"d":1000000000}`, string(b))

	var out payload
	expect.NoError(t, UnmarshalJSON(b, &out))
	expect.Equal(t, out.D, time.Second)

	s, err := MarshalString(in)
	expect.NoError(t, err)
	assertJSONEqual(t, `{"d":1000000000}`, s)
	expect.False(t, strings.HasSuffix(s, "\n"))

	var fromString payload
	expect.NoError(t, UnmarshalFromString(s, &fromString))
	expect.Equal(t, fromString.D, time.Second)
}

func TestJSONStringAndValid(t *testing.T) {
	s, err := MarshalString(map[string]int{"a": 1})
	expect.NoError(t, err)
	assertJSONEqual(t, `{"a":1}`, s)
	expect.True(t, ValidJSONString(s))
	expect.True(t, ValidJSON([]byte(s)))
	expect.False(t, ValidJSONString("{"))
	expect.False(t, ValidJSONString(""))

	var got map[string]int
	expect.NoError(t, UnmarshalFromString(s, &got))
	expect.Equal(t, got["a"], 1)
	expect.HasError(t, UnmarshalFromString("{", &got))
}

func TestJSONEncoderDecoder(t *testing.T) {
	var buf bytes.Buffer
	expect.NoError(t, NewJSONEncoder(&buf).Encode(map[string]int{"n": 2}))
	expect.True(t, strings.HasSuffix(buf.String(), "\n"), "streaming encode should end with a newline")
	assertJSONEqual(t, `{"n":2}`, strings.TrimSpace(buf.String()))

	var got map[string]int
	expect.NoError(t, NewJSONDecoder(&buf).Decode(&got))
	expect.Equal(t, got["n"], 2)
}

func TestJSONMarshalIndent(t *testing.T) {
	b, err := MarshalJSONIndent(map[string]int{"a": 1}, "", "  ")
	expect.NoError(t, err)
	expect.Equal(t, string(b), "{\n  \"a\": 1\n}")
}

// assertJSONEqual compares JSON values independently of object member order and whitespace.
func assertJSONEqual(t *testing.T, want, got string) {
	t.Helper()
	var wantValue, gotValue any
	expect.NoError(t, jsonv2.Unmarshal([]byte(want), &wantValue))
	expect.NoError(t, jsonv2.Unmarshal([]byte(got), &gotValue))
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON values differ: got %s, want %s", got, want)
	}
}
