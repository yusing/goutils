package strutils

import (
	"bytes"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestJSONEncoderIndentRegression(t *testing.T) {
	var buf bytes.Buffer
	enc := NewJSONEncoder(&buf)
	enc.SetIndent("\t", "  ")
	if err := enc.Encode(struct {
		N int `json:"n"`
	}{N: 2}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "{\n\t  \"n\": 2\n\t}\n"; got != want {
		t.Fatalf("indented JSON = %q, want %q", got, want)
	}
	buf.Reset()
	enc.SetIndent("", "")
	if err := enc.Encode(struct {
		N int `json:"n"`
	}{N: 3}); err != nil {
		t.Fatal(err)
	}
	if got, want := buf.String(), "{\"n\":3}\n"; got != want {
		t.Fatalf("reset JSON = %q, want %q", got, want)
	}
}

func TestRedactRuneRegression(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", ""}, {"a", "*"}, {"ab", "**"}, {"abc", "a**c"}, {"abcd", "a**d"}, {"abcde", "ab*de"},
		{"你", "*"}, {"你好", "**"}, {"你好嗎", "你**嗎"}, {"你好世界", "你**界"}, {"你好世界啊", "你好*界啊"},
		{strings.Repeat("界", 69), "界界" + strings.Repeat("*", 65) + "界界"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got := Redact(tc.input)
			if !utf8.ValidString(got) || got != tc.want {
				t.Fatalf("Redact(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestFormatByteSizeRegression(t *testing.T) {
	type signed int64
	type unsigned uint64
	type integer int
	type unsignedInteger uint
	type floating float64
	for _, tc := range []struct{ name, got, want string }{
		{"float zero", FormatByteSize(0.0), "0 B"}, {"named signed", FormatByteSize(signed(-7)), "-7 B"},
		{"named unsigned", FormatByteSize(unsigned(42)), "42 B"}, {"named int", FormatByteSize(integer(12)), "12 B"},
		{"named uint", FormatByteSize(unsignedInteger(13)), "13 B"}, {"named float", FormatByteSize(floating(0)), "0 B"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %q, want %q", tc.got, tc.want)
			}
		})
	}
}
