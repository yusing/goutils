package strutils

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"time"
)

// AppendDuration appends a human-readable duration, for example "1 ns",
// "1 ms", "1 second", "1 minute and 1 second", or "1 day, 1 hour and 1 minute".
// Seconds are omitted for durations of at least one hour. Zero is "0 Seconds".
func AppendDuration(d time.Duration, buf []byte) []byte {
	if d < 0 {
		buf = append(buf, '-')
		d = -d
	}

	if d == 0 {
		return append(buf, []byte("0 Seconds")...)
	}

	switch {
	case d < time.Millisecond:
		buf = strconv.AppendInt(buf, d.Nanoseconds(), 10)
		buf = append(buf, []byte(" ns")...)
		return buf
	case d < time.Second:
		buf = strconv.AppendInt(buf, d.Milliseconds(), 10)
		buf = append(buf, []byte(" ms")...)
		return buf
	}

	// Get total seconds from duration
	totalSeconds := int64(d.Seconds())

	// Calculate days, hours, minutes, and seconds
	days := totalSeconds / (24 * 3600)
	hours := (totalSeconds % (24 * 3600)) / 3600
	minutes := (totalSeconds % 3600) / 60
	seconds := totalSeconds % 60

	idxPartBeg := 0
	if days > 0 {
		buf = strconv.AppendInt(buf, days, 10)
		buf = fmt.Appendf(buf, " day%s, ", Pluralize(days))
	}
	if hours > 0 {
		idxPartBeg = len(buf) - 2
		buf = strconv.AppendInt(buf, hours, 10)
		buf = fmt.Appendf(buf, " hour%s, ", Pluralize(hours))
	}
	if minutes > 0 {
		idxPartBeg = len(buf) - 2
		buf = strconv.AppendInt(buf, minutes, 10)
		buf = fmt.Appendf(buf, " minute%s, ", Pluralize(minutes))
	}
	if seconds > 0 && totalSeconds < 3600 {
		idxPartBeg = len(buf) - 2
		buf = strconv.AppendInt(buf, seconds, 10)
		buf = fmt.Appendf(buf, " second%s, ", Pluralize(seconds))
	}
	// remove last comma and space
	buf = buf[:len(buf)-2]
	if idxPartBeg > 0 && idxPartBeg < len(buf) {
		// replace last part ', ' with ' and ' in-place, alloc-free
		// ', ' is 2 bytes, ' and ' is 5 bytes, so we need to make room for 3 more bytes
		tailLen := len(buf) - (idxPartBeg + 2)
		buf = append(buf, "000"...)                                      // append 3 bytes for ' and '
		copy(buf[idxPartBeg+5:], buf[idxPartBeg+2:idxPartBeg+2+tailLen]) // shift tail right by 3
		copy(buf[idxPartBeg:], " and ")                                  // overwrite ', ' with ' and '
	}
	return buf
}

func FormatDuration(d time.Duration) string {
	return string(AppendDuration(d, nil))
}

func FormatLastSeen(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return FormatTime(t)
}

func appendRound(f float64, buf []byte) []byte {
	return strconv.AppendInt(buf, int64(math.Round(f)), 10)
}

func appendFloat(f float64, buf []byte) []byte {
	f = math.Round(f*100) / 100
	return strconv.AppendFloat(buf, f, 'f', -1, 64)
}

func AppendTime(t time.Time, buf []byte) []byte {
	if t.IsZero() {
		return append(buf, []byte("never")...)
	}
	return AppendTimeWithReference(t, time.Now(), buf)
}

func FormatTime(t time.Time) string {
	return string(AppendTime(t, nil))
}

func FormatUnixTime(t int64) string {
	return FormatTime(time.Unix(t, 0))
}

func FormatTimeWithReference(t, ref time.Time) string {
	return string(AppendTimeWithReference(t, ref, nil))
}

func AppendTimeWithReference(t, ref time.Time, buf []byte) []byte {
	if t.IsZero() {
		return append(buf, []byte("never")...)
	}
	diff := t.Sub(ref)
	absDiff := diff.Abs()
	switch {
	case absDiff < time.Second:
		return append(buf, []byte("now")...)
	case absDiff < 3*time.Second:
		if diff < 0 {
			return append(buf, []byte("just now")...)
		}
		fallthrough
	case absDiff < 60*time.Second:
		if diff < 0 {
			buf = appendRound(absDiff.Seconds(), buf)
			buf = append(buf, []byte(" seconds ago")...)
		} else {
			buf = append(buf, []byte("in ")...)
			buf = appendRound(absDiff.Seconds(), buf)
			buf = append(buf, []byte(" seconds")...)
		}
		return buf
	case absDiff < 60*time.Minute:
		if diff < 0 {
			buf = appendRound(absDiff.Minutes(), buf)
			buf = append(buf, []byte(" minutes ago")...)
		} else {
			buf = append(buf, []byte("in ")...)
			buf = appendRound(absDiff.Minutes(), buf)
			buf = append(buf, []byte(" minutes")...)
		}
		return buf
	case absDiff < 24*time.Hour:
		if diff < 0 {
			buf = appendRound(absDiff.Hours(), buf)
			buf = append(buf, []byte(" hours ago")...)
		} else {
			buf = append(buf, []byte("in ")...)
			buf = appendRound(absDiff.Hours(), buf)
			buf = append(buf, []byte(" hours")...)
		}
		return buf
	case t.Year() == ref.Year():
		return t.AppendFormat(buf, "01-02 15:04:05")
	default:
		return t.AppendFormat(buf, "2006-01-02 15:04:05")
	}
}

func FormatByteSize[T ~int | ~uint | ~int64 | ~uint64 | ~float64](size T) string {
	return string(AppendByteSize(size, nil))
}

func AppendByteSize[T ~int | ~uint | ~int64 | ~uint64 | ~float64](size T, buf []byte) []byte {
	const (
		_ = (1 << (10 * iota))
		kb
		mb
		gb
		tb
		pb
	)
	switch {
	case size < kb:
		switch reflect.TypeFor[T]().Kind() {
		case reflect.Int, reflect.Int64:
			buf = strconv.AppendInt(buf, int64(size), 10)
		case reflect.Uint, reflect.Uint64:
			buf = strconv.AppendUint(buf, uint64(size), 10)
		case reflect.Float64:
			buf = appendFloat(float64(size), buf)
		}
		buf = append(buf, []byte(" B")...)
	case size < mb:
		buf = appendFloat(float64(size)/kb, buf)
		buf = append(buf, []byte(" KiB")...)
	case size < gb:
		buf = appendFloat(float64(size)/mb, buf)
		buf = append(buf, []byte(" MiB")...)
	case size < tb:
		buf = appendFloat(float64(size)/gb, buf)
		buf = append(buf, []byte(" GiB")...)
	case size < pb:
		buf = appendFloat(float64(size/gb)/kb, buf)
		buf = append(buf, []byte(" TiB")...)
	default:
		buf = appendFloat(float64(size/tb)/kb, buf)
		buf = append(buf, []byte(" PiB")...)
	}
	return buf
}

func Pluralize(n int64) string {
	if n > 1 {
		return "s"
	}
	return ""
}
