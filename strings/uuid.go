package strutils

import (
	"sync/atomic"

	"github.com/yusing/goutils/mockable"
)

var uuidV7Seq atomic.Uint64

// NewUUIDv7 returns a RFC 9562 UUID version 7 string.
func NewUUIDv7() string {
	var b [16]byte

	ts := uint64(mockable.TimeNow().UnixMilli())
	b[0] = byte(ts >> 40)
	b[1] = byte(ts >> 32)
	b[2] = byte(ts >> 24)
	b[3] = byte(ts >> 16)
	b[4] = byte(ts >> 8)
	b[5] = byte(ts)

	seq := uuidV7Seq.Add(1)
	b[6] = byte(seq >> 8)
	b[7] = byte(seq)
	b[8] = 0
	b[9] = 0
	b[10] = 0
	b[11] = 0
	b[12] = 0
	b[13] = 0
	b[14] = 0
	b[15] = 0

	b[6] = (b[6] & 0x0f) | 0x70
	b[8] = (b[8] & 0x3f) | 0x80

	return formatUUID(b)
}

func formatUUID(b [16]byte) string {
	const hex = "0123456789abcdef"

	var out [36]byte
	pos := 0
	for i := range len(b) {
		if pos == 8 || pos == 13 || pos == 18 || pos == 23 {
			out[pos] = '-'
			pos++
		}
		out[pos] = hex[b[i]>>4]
		out[pos+1] = hex[b[i]&0x0f]
		pos += 2
	}

	return string(out[:])
}
