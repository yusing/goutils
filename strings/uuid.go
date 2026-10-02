package strutils

import "uuid"

// NewUUIDv7 returns an RFC 9562 UUID version 7 string with cryptographically random bits.
//
// Deprecated: Use uuid.NewV7().String() from the standard library instead.
func NewUUIDv7() string {
	return uuid.NewV7().String()
}
