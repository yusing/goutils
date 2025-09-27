package strutils_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	. "github.com/yusing/goutils/strings"
)

func mustParseTime(t *testing.T, layout, value string) time.Time {
	t.Helper()
	time, err := time.Parse(layout, value)
	if err != nil {
		t.Fatalf("failed to parse time: %s", err)
	}
	return time
}

func TestFormatTime(t *testing.T) {
	now := mustParseTime(t, time.RFC3339, "2021-06-15T12:30:30Z")

	tests := []struct {
		name           string
		time           time.Time
		expected       string
		expectedLength int
	}{
		{
			name:     "now",
			time:     now.Add(100 * time.Millisecond),
			expected: "now",
		},
		{
			name:     "just now (past within 3 seconds)",
			time:     now.Add(-1 * time.Second),
			expected: "just now",
		},
		{
			name:     "seconds ago",
			time:     now.Add(-10 * time.Second),
			expected: "10 seconds ago",
		},
		{
			name:     "in seconds",
			time:     now.Add(10 * time.Second),
			expected: "in 10 seconds",
		},
		{
			name:     "minutes ago",
			time:     now.Add(-10 * time.Minute),
			expected: "10 minutes ago",
		},
		{
			name:     "in minutes",
			time:     now.Add(10 * time.Minute),
			expected: "in 10 minutes",
		},
		{
			name:     "hours ago",
			time:     now.Add(-10 * time.Hour),
			expected: "10 hours ago",
		},
		{
			name:     "in hours",
			time:     now.Add(10 * time.Hour),
			expected: "in 10 hours",
		},
		{
			name:           "different day",
			time:           now.Add(-25 * time.Hour),
			expectedLength: len("01-01 15:04:05"),
		},
		{
			name:           "same year but different month",
			time:           now.Add(-30 * 24 * time.Hour),
			expectedLength: len("01-01 15:04:05"),
		},
		{
			name:     "different year",
			time:     time.Date(now.Year()-1, 1, 1, 10, 20, 30, 0, now.Location()),
			expected: time.Date(now.Year()-1, 1, 1, 10, 20, 30, 0, now.Location()).Format("2006-01-02 15:04:05"),
		},
		{
			name:     "zero time",
			time:     time.Time{},
			expected: "never",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatTimeWithReference(tt.time, now)

			if tt.expectedLength > 0 {
				require.Len(t, result, tt.expectedLength)
			} else {
				require.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		duration time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			duration: 0,
			expected: "0 Seconds",
		},
		{
			name:     "seconds only",
			duration: 45 * time.Second,
			expected: "45 seconds",
		},
		{
			name:     "one second",
			duration: 1 * time.Second,
			expected: "1 second",
		},
		{
			name:     "minutes only",
			duration: 5 * time.Minute,
			expected: "5 minutes",
		},
		{
			name:     "one minute",
			duration: 1 * time.Minute,
			expected: "1 minute",
		},
		{
			name:     "hours only",
			duration: 3 * time.Hour,
			expected: "3 hours",
		},
		{
			name:     "one hour",
			duration: 1 * time.Hour,
			expected: "1 hour",
		},
		{
			name:     "days only",
			duration: 2 * 24 * time.Hour,
			expected: "2 days",
		},
		{
			name:     "one day",
			duration: 24 * time.Hour,
			expected: "1 day",
		},
		{
			name:     "complex duration",
			duration: 2*24*time.Hour + 3*time.Hour + 45*time.Minute + 15*time.Second,
			expected: "2 days, 3 hours and 45 minutes",
		},
		{
			name:     "hours and minutes",
			duration: 2*time.Hour + 30*time.Minute,
			expected: "2 hours and 30 minutes",
		},
		{
			name:     "days and hours",
			duration: 1*24*time.Hour + 12*time.Hour,
			expected: "1 day and 12 hours",
		},
		{
			name:     "days and hours and minutes",
			duration: 1*24*time.Hour + 12*time.Hour + 30*time.Minute,
			expected: "1 day, 12 hours and 30 minutes",
		},
		{
			name:     "days and hours and minutes and seconds (ignore seconds)",
			duration: 1*24*time.Hour + 12*time.Hour + 30*time.Minute + 15*time.Second,
			expected: "1 day, 12 hours and 30 minutes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatDuration(tt.duration)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatLastSeen(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "zero time",
			time:     time.Time{},
			expected: "never",
		},
		{
			name: "non-zero time",
			time: now.Add(-10 * time.Minute),
			// The actual result will be handled by FormatTime, which is tested separately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatLastSeen(tt.time)

			if tt.name == "zero time" {
				require.Equal(t, tt.expected, result)
			} else if result == "never" { // Just make sure it's not "never", the actual formatting is tested in TestFormatTime
				t.Errorf("Expected non-zero time to not return 'never', got %s", result)
			}
		})
	}
}

func TestFormatByteSize(t *testing.T) {
	tests := []struct {
		name     string
		size     int64
		expected string
	}{
		{
			name:     "zero size",
			size:     0,
			expected: "0 B",
		},
		{
			name:     "one byte",
			size:     1,
			expected: "1 B",
		},
		{
			name:     "bytes (less than 1 KiB)",
			size:     1023,
			expected: "1023 B",
		},
		{
			name:     "1 KiB",
			size:     1024,
			expected: "1 KiB",
		},
		{
			name:     "KiB (less than 1 MiB)",
			size:     1024 * 1023,
			expected: "1023 KiB",
		},
		{
			name:     "1 MiB",
			size:     1024 * 1024,
			expected: "1 MiB",
		},
		{
			name:     "MiB (less than 1 GiB)",
			size:     1024 * 1024 * 1023,
			expected: "1023 MiB",
		},
		{
			name:     "1 GiB",
			size:     1024 * 1024 * 1024,
			expected: "1 GiB",
		},
		{
			name:     "GiB (less than 1 TiB)",
			size:     1024 * 1024 * 1024 * 1023,
			expected: "1023 GiB",
		},
		{
			name:     "1 TiB",
			size:     1024 * 1024 * 1024 * 1024,
			expected: "1 TiB",
		},
		{
			name:     "TiB (less than 1 PiB)",
			size:     1024 * 1024 * 1024 * 1024 * 1023,
			expected: "1023 TiB",
		},
		{
			name:     "1 PiB",
			size:     1024 * 1024 * 1024 * 1024 * 1024,
			expected: "1 PiB",
		},
		{
			name:     "PiB (large number)",
			size:     1024 * 1024 * 1024 * 1024 * 1024 * 1023,
			expected: "1023 PiB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatByteSize(tt.size)
			require.Equal(t, tt.expected, result)
		})
	}
}
