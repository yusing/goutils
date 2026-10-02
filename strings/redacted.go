package strutils

import (
	"strings"
)

const numAsterisks = 64

var asterisks = strings.Repeat("*", numAsterisks)

type Redacted string

func (r Redacted) String() string {
	return string(r)
}

func (r Redacted) Empty() bool {
	return r == ""
}

func (r Redacted) MarshalJSON() ([]byte, error) {
	return MarshalJSON(Redact(string(r)))
}

func (r Redacted) MarshalYAML() ([]byte, error) {
	return MarshalYAML(Redact(string(r)))
}

func (r *Redacted) UnmarshalJSON(data []byte) error {
	var s string
	err := UnmarshalJSON(data, &s)
	if err != nil {
		return err
	}
	*r = Redacted(s)
	return nil
}

func (r *Redacted) UnmarshalYAML(data []byte) error {
	var s string
	err := UnmarshalYAML(data, &s)
	if err != nil {
		return err
	}
	*r = Redacted(s)
	return nil
}

func Redact(s string) string {
	runes := []rune(s)
	n := len(runes)
	if n == 0 {
		return ""
	}
	if n <= 2 {
		return asterisks[:n]
	}
	if n <= 4 {
		return string(runes[:1]) + "**" + string(runes[n-1:])
	}
	if n-4 <= numAsterisks {
		return string(runes[:2]) + asterisks[:n-4] + string(runes[n-2:])
	}
	return string(runes[:2]) + strings.Repeat("*", n-4) + string(runes[n-2:])
}
