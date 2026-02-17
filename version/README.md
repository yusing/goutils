# goutils/version

Version utilities for semantic versioning with parsing and comparison.

## Overview

The `version` package provides a `Version` type for semantic versioning (generation.major.minor) with parsing and comparison.

## API Reference

```go
type Version struct{ Generation, Major, Minor int }

func New(gen, major, minor int) Version
func Get() Version
func Parse(v string) Version
```

### Comparison Methods

```go
func (v Version) IsNewerThan(other Version) bool
func (v Version) IsNewerThanMajor(other Version) bool
func (v Version) IsOlderThan(other Version) bool
func (v Version) IsOlderThanMajor(other Version) bool
func (v Version) IsOlderMajorThan(other Version) bool
func (v Version) IsEqual(other Version) bool
```

### String Methods

```go
func (v Version) String() string
func (v Version) MarshalText() ([]byte, error)
func (v *Version) UnmarshalText(text []byte) error
```

## Usage

```go
// Parse version string
v := version.Parse("v1.2.3")
fmt.Println(v) // "v1.2.3"

// Create version
v := version.New(1, 2, 3)

// Comparison
v1 := version.Parse("v1.2.3")
v2 := version.Parse("v1.2.4")
v1.IsNewerThan(v2)       // false
v2.IsNewerThan(v1)       // true
v2.IsNewerThanMajor(v1)  // false (same generation)

// JSON serialization
data, _ := json.Marshal(v) // "\"v1.2.3\""
var v2 version.Version
json.Unmarshal(data, &v2)
```

## Format

Versions follow the format: `v<generation>.<major>.<minor>`

Examples: `v1.0.0`, `v2.3.15`, `v3.1.0-beta`
