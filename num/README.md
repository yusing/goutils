# goutils/num

Number utilities including percentage type.

## Overview

The `num` package provides a `Percentage` type for compact representation of values in [0, 100].

## API Reference

```go
type Percentage struct{ code uint8 }

func NewPercentage(f float64) Percentage
func (p Percentage) ToFloat() float64
func (p Percentage) String() string
func (p Percentage) MarshalJSON() ([]byte, error)
func (p *Percentage) UnmarshalJSON(data []byte) error
```

## Usage

```go
// Create percentage from float
p := num.NewPercentage(75.5)
fmt.Println(p.ToFloat()) // 75.5
fmt.Println(p)           // 75.5%

// JSON serialization
data, _ := json.Marshal(p)       // "75.5"
var p2 num.Percentage
json.Unmarshal([]byte(`"75.5"`), &p2)
```

## Features

- 1-byte storage per value
- ~1000 discrete levels of precision
- JSON marshal/unmarshal support
- String representation with %
