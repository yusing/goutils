package strutils_test

import (
	"testing"
	"uuid"

	. "github.com/yusing/goutils/strings"
)

func TestNewUUIDv7(t *testing.T) {
	seen := make(map[string]bool)
	hasRandomTail := false
	for range 128 {
		id := NewUUIDv7()
		parsed, err := uuid.Parse(id)
		if err != nil {
			t.Fatalf("invalid UUID %q: %v", id, err)
		}
		for _, b := range parsed[9:] {
			hasRandomTail = hasRandomTail || b != 0
		}
		if len(id) != 36 || id[14] != '7' || (id[19] != '8' && id[19] != '9' && id[19] != 'a' && id[19] != 'b') {
			t.Fatalf("not an RFC 9562 version 7 UUID: %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate UUID: %s", id)
		}
		seen[id] = true
	}
	if !hasRandomTail {
		t.Fatal("all UUID random tails are zero, as in the old deterministic generator")
	}
}
