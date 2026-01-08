# strings

String utilities for manipulation, formatting, and parsing.

## Overview

The `strings` package provides various string utilities including formatting, parsing, and comparison.

## API Reference

### Basic Operations

```go
func CommaSeperatedList(s string) []string
func Title(s string) string
func ContainsFold(s, substr string) bool
func IndexFold(s, substr string) int
func ToLowerNoSnake(s string) string
func LevenshteinDistance(a, b string) int
```

### Formatting

```go
func FormatDuration(d time.Duration) string
func FormatTime(t time.Time) string
func FormatLastSeen(t time.Time) string
func FormatTimeWithReference(t, ref time.Time) string
func FormatUnixTime(t int64) string
func FormatByteSize[T ~int | ~uint | ~int64 | ~uint64 | ~float64](size T) string

func AppendDuration(d time.Duration, buf []byte) []byte
func AppendTime(t time.Time, buf []byte) []byte
func AppendByteSize[T ~int | ~uint | ~int64 | ~uint64 | ~float64](size T, buf []byte) []byte
func AppendTimeWithReference(t, ref time.Time, buf []byte) []byte
```

### Utilities

```go
func Pluralize(n int64) string
```

## Usage

```go
// Parse comma-separated list
items := strutils.CommaSeperatedList("a, b, c") // ["a", "b", "c"]

// Case-insensitive search
strutils.ContainsFold("Hello World", "world") // true

// Levenshtein distance
dist := strutils.LevenshteinDistance("kitten", "sitting") // 3

// Format duration
strutils.FormatDuration(3661000000000) // "1 hour, 1 minute and 1 second"

// Format time ago
strutils.FormatTime(time.Now().Add(-5 * time.Minute)) // "5 minutes ago"

// Format byte size
strutils.FormatByteSize(1024 * 1024) // "1 MiB"
```
