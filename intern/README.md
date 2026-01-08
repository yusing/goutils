# intern

Internal string handling using unique handles for memory efficiency.

## Overview

The `intern` is a type-safe wrapper around unique.Handle[string] that implements json.Unmarshaler and json.Marshaler.

## API Reference

```go
type Handle[T comparable] struct {
    v unique.Handle[T]
}

func Make[T comparable](v T) Handle[T]
func (s Handle[T]) Value() T
func (s Handle[T]) MarshalJSON() ([]byte, error)
func (s *Handle[T]) UnmarshalJSON(data []byte) error
```

## Usage

```go
// Create handles for duplicate strings
h1 := intern.Make("hello")
h2 := intern.Make("hello")

fmt.Println(h1.Value()) // "hello"
fmt.Println(h1 == h2)   // true - same memory

// JSON serialization
data, _ := json.Marshal(h1)
var h3 intern.Handle[string]
json.Unmarshal(data, &h3)
```

## Use Cases

- Configuration structs
- Logging frequently logged messages
- Cache keys with duplicates
