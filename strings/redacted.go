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
	n := len(s)
	if n == 0 {
		return ""
	}
	if n <= 4 {
		return s[:1] + "**" + s[n-1:]
	}
	if n-4 <= numAsterisks {
		return s[:2] + asterisks[:n-4] + s[n-2:]
	}
	return redactLong(s, n)
}

func redactLong(s string, n int) string {
	var b strings.Builder
	b.Grow(n)
	b.WriteString(s[:2])
	for remaining := n - 4; remaining > 0; {
		chunk := min(remaining, numAsterisks)
		b.WriteString(asterisks[:chunk])
		remaining -= chunk
	}
	b.WriteString(s[n-2:])
	return b.String()
}
