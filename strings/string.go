package strutils

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func Title(s string) string {
	return cases.Title(language.AmericanEnglish).String(s)
}

func ContainsFold(s, substr string) bool {
	return IndexFold(s, substr) >= 0
}

func IndexFold(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	if isASCII(s) && isASCII(substr) {
		if len(substr) > len(s) {
			return -1
		}
		return indexFoldASCII(s, substr)
	}
	return indexFoldUnicode(s, substr)
}

func HasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

func HasSuffixFold(s, suffix string) bool {
	return len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix)
}

func ToLowerNoSnake(s string) string {
	var buf strings.Builder
	for _, r := range s {
		if r == '_' {
			continue
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

func indexFoldASCII(s, substr string) int {
	first := lowerASCII(substr[0])
	last := len(s) - len(substr)

next:
	for i := 0; i <= last; i++ {
		if lowerASCII(s[i]) != first {
			continue
		}
		for j := 1; j < len(substr); j++ {
			if lowerASCII(s[i+j]) != lowerASCII(substr[j]) {
				continue next
			}
		}
		return i
	}
	return -1
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func indexFoldUnicode(s, substr string) int {
	lowerS, index := lowerWithByteIndex(s)
	i := strings.Index(lowerS, strings.ToLower(substr))
	if i < 0 {
		return -1
	}
	return index[i]
}

func lowerWithByteIndex(s string) (string, []int) {
	var b strings.Builder
	b.Grow(len(s))
	index := make([]int, 0, len(s))
	for i, r := range s {
		lower := strings.ToLower(string(r))
		b.WriteString(lower)
		for range len(lower) {
			index = append(index, i)
		}
	}
	return b.String(), index
}

//nolint:intrange
func LevenshteinDistance(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	v0 := make([]int, len(b)+1)
	v1 := make([]int, len(b)+1)

	for i := 0; i <= len(b); i++ {
		v0[i] = i
	}

	for i := 0; i < len(a); i++ {
		v1[0] = i + 1

		for j := 0; j < len(b); j++ {
			cost := 0
			if a[i] != b[j] {
				cost = 1
			}

			v1[j+1] = min(v1[j]+1, v0[j+1]+1, v0[j]+cost)
		}

		for j := 0; j <= len(b); j++ {
			v0[j] = v1[j]
		}
	}

	return v1[len(b)]
}
