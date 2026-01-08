# pool

Object pool for managing collections of objects with keys.

## Overview

The `pool` package provides a thread-safe pool for managing objects with string keys.

## API Reference

### Types

```go
type Pool[T Object] struct {
    m          *xsync.Map[string, T]
    name       string
    disableLog atomic.Bool
}

type Object interface {
    Key() string
    Name() string
}

type ObjectWithDisplayName interface {
    Object
    DisplayName() string
}

type Preferable interface {
    PreferOver(other any) bool
}
```

### Methods

```go
func New[T Object](name string) Pool[T]
func (p *Pool[T]) Add(obj T)
func (p *Pool[T]) AddKey(key string, obj T)
func (p *Pool[T]) AddIfNotExists(obj T) (actual T, added bool)
func (p *Pool[T]) Del(obj T)
func (p *Pool[T]) DelKey(key string)
func (p *Pool[T]) Get(key string) (T, bool)
func (p *Pool[T]) Size() int
func (p *Pool[T]) Clear()
func (p *Pool[T]) Iter(fn func(k string, v T) bool)
func (p *Pool[T]) Slice() []T
func (p *Pool[T]) ToggleLog(v bool)
```

## Usage

```go
type User struct{ id, name string }

func (u User) Key() string   { return u.id }
func (u User) Name() string  { return u.name }

pool := pool.New[User]("users")
pool.Add(User{"1", "Alice"})
pool.Add(User{"2", "Bob"})

user, ok := pool.Get("1")
pool.Del(user)
```

## Features

- Thread-safe operations via xsync.Map
- Optional preference-based replacement
- Logging of add/remove operations
- Sorted slice output
