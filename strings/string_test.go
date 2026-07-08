package strutils

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexFold(t *testing.T) {
	tests := []struct {
		name         string
		s, substr    string
		wantIndex    int
		wantContains bool
		wantPrefix   bool
		wantSuffix   bool
	}{
		{name: "exact", s: "application/grpc+proto", substr: "grpc", wantIndex: 12, wantContains: true},
		{name: "folded", s: "Content-Type", substr: "type", wantIndex: 8, wantContains: true, wantSuffix: true},
		{name: "missing", s: "Content-Type", substr: "accept", wantIndex: -1},
		{name: "empty_substr", s: "abc", substr: "", wantIndex: 0, wantContains: true, wantPrefix: true, wantSuffix: true},
		{name: "unicode", s: "Straße", substr: "ße", wantIndex: 4, wantContains: true, wantSuffix: true},
		{name: "unicode_folded", s: "Äpfel", substr: "ä", wantIndex: 0, wantContains: true, wantPrefix: true},
		{name: "unicode_lowercase_byte_length_change", s: "k", substr: "\u212a", wantIndex: 0, wantContains: true},
		{name: "unicode_lowercase_byte_length_change_before_match", s: "\u212aX", substr: "x", wantIndex: len("\u212a"), wantContains: true, wantSuffix: true},
		{name: "prefix_folded", s: "Application/Grpc", substr: "application", wantIndex: 0, wantContains: true, wantPrefix: true},
		{name: "suffix_folded", s: "Application/Grpc", substr: "grpc", wantIndex: 12, wantContains: true, wantSuffix: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.wantIndex, IndexFold(tt.s, tt.substr))
			require.Equal(t, tt.wantContains, ContainsFold(tt.s, tt.substr))
			require.Equal(t, tt.wantPrefix, HasPrefixFold(tt.s, tt.substr))
			require.Equal(t, tt.wantSuffix, HasSuffixFold(tt.s, tt.substr))
		})
	}
}

func BenchmarkIndexFold(b *testing.B) {
	tests := [...]struct {
		name      string
		s, substr string
	}{
		{name: "exact", s: "application/grpc+proto", substr: "grpc"},
		{name: "folded", s: "Application/Grpc+Proto", substr: "grpc"},
		{name: "exact_long", s: "text/event-stream; charset=utf-8", substr: "event-stream"},
		{name: "missing", s: "text/event-stream; charset=utf-8", substr: "application/grpc"},
		{name: "long_ascii_late", s: strings.Repeat("a", 256) + "grpc", substr: "grpc"},
		{name: "long_mixed_ascii_late", s: strings.Repeat("aA", 128) + "Grpc", substr: "grpc"},
		{name: "long_repeated_prefix_miss", s: strings.Repeat("a", 256), substr: "aaaaab"},
		{name: "non_ascii_early_match", s: "\u00e9grpc", substr: "grpc"},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = IndexFold(tt.s, tt.substr)
			}
		})
	}

	b.Run("mixed", func(b *testing.B) {
		idx := 0
		b.ReportAllocs()
		for b.Loop() {
			tt := tests[idx]
			_ = IndexFold(tt.s, tt.substr)
			idx++
			if idx == len(tests) {
				idx = 0
			}
		}
	})
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{a: "", b: "", want: 0},
		{a: "", b: "abc", want: 3},
		{a: "abc", b: "", want: 3},
		{a: "kitten", b: "sitting", want: 3},
		{a: "aba", b: "bab", want: 2},
		{a: "book", b: "back", want: 2},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%q_%q", tt.a, tt.b), func(t *testing.T) {
			require.Equal(t, tt.want, LevenshteinDistance(tt.a, tt.b))
		})
	}
}

func BenchmarkLevenshteinDistance(b *testing.B) {
	tests := [...]struct {
		a, b string
	}{
		{a: "aba", b: "bab"},
		{a: "kitten", b: "sitting"},
		{a: "authorization", b: "authentication"},
		{a: "reverse_proxy_route", b: "reverse_proxy_rule"},
	}
	idx := 0

	b.ReportAllocs()
	for b.Loop() {
		tt := tests[idx]
		_ = LevenshteinDistance(tt.a, tt.b)
		idx++
		if idx == len(tests) {
			idx = 0
		}
	}
}
