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
	if len(s) == 0 {
		return ""
	}
	if len(s) <= 4 {
		return s[:1] + "**" + s[len(s)-1:]
	}
	if len(s)-4 < numAsterisks {
		return s[:2] + asterisks[:len(s)-4] + s[len(s)-2:]
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}
