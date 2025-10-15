package intern

import (
	"bytes"
	"encoding/json"
	"unique"
)

// Handle is wrapper around unique.Handle[string]
// but it implements json.Unmarshaler and json.Marshaler.
type Handle[T comparable] struct {
	v unique.Handle[T]
}

func Make[T comparable](v T) Handle[T] {
	return Handle[T]{
		v: unique.Make(v),
	}
}

func (s Handle[T]) Value() T {
	return s.v.Value()
}

func (s Handle[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Value())
}

func (s *Handle[T]) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || bytes.Equal(data, []byte("null")) || bytes.Equal(data, []byte("{}")) {
		return nil
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	s.v = unique.Make(v)
	return nil
}
