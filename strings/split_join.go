package strutils

import (
	"strings"
	"unicode"
)

// CommaSeperatedList splits s on commas and Unicode whitespace, omitting empty fields.
func CommaSeperatedList(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.FieldsFunc(s, isCommaSpace)
}

func isCommaSpace(r rune) bool {
	return r == ',' || unicode.IsSpace(r)
}
