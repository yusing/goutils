package strutils

import (
	"strings"
	"unicode"
)

// CommaSeperatedList returns a list of strings split by commas,
// then trim spaces from each element.
func CommaSeperatedList(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.FieldsFunc(s, isCommaSpace)
}

func isCommaSpace(r rune) bool {
	return r == ',' || unicode.IsSpace(r)
}
