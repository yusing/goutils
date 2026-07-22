package gperr

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	strutils "github.com/yusing/goutils/strings"
)

var (
	_ json.Marshaler = baseError{}
	_ json.Marshaler = (*nestedError)(nil)
	_ json.Marshaler = (*MultilineError)(nil)
	_ json.Marshaler = (*withSubject)(nil)
)

type structuredJSONError struct {
	Kind string `json:"kind"`
}

func (err structuredJSONError) Error() string { return "structured error" }

func (err structuredJSONError) MarshalJSON() ([]byte, error) {
	type wireError structuredJSONError
	return json.Marshal(wireError(err))
}

type malformedJSONError struct{ err error }

func (err malformedJSONError) Error() string { return "malformed error" }

func (err malformedJSONError) MarshalJSON() ([]byte, error) { return nil, err.err }

type futureJSONError struct{}

func (futureJSONError) Error() string { return "future error" }

// legacyError proves that implementing Error does not require opting into a
// serialization interface. Package wrappers still provide readable JSON at
// the presentation boundary.
type legacyError struct{}

func (legacyError) Error() string                     { return "legacy error" }
func (legacyError) Is(error) bool                     { return false }
func (err legacyError) With(error) Error              { return err }
func (err legacyError) Withf(string, ...any) Error    { return err }
func (err legacyError) Subject(string) Error          { return err }
func (err legacyError) Subjectf(string, ...any) Error { return err }
func (legacyError) Plain() []byte                     { return []byte("legacy error") }
func (legacyError) Markdown() []byte                  { return []byte("legacy error") }

var _ Error = legacyError{}

func TestErrorJSONContract(t *testing.T) {
	t.Run("nil remains nil", func(t *testing.T) {
		require.Nil(t, Wrap(nil))
	})

	t.Run("plain error is readable and retains identity", func(t *testing.T) {
		sentinel := errors.New("plain error")
		err := Wrap(sentinel)

		encoded, marshalErr := strutils.MarshalJSON(err)
		require.NoError(t, marshalErr)
		require.JSONEq(t, `"plain error"`, string(encoded))
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("underlying structured JSON is preserved", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(Wrap(structuredJSONError{Kind: "structured"}))
		require.NoError(t, err)
		require.JSONEq(t, `{"kind":"structured"}`, string(encoded))
	})

	t.Run("malformed underlying JSON is propagated", func(t *testing.T) {
		sentinel := errors.New("marshal failed")
		_, err := strutils.MarshalJSON(Wrap(malformedJSONError{err: sentinel}))
		require.ErrorIs(t, err, sentinel)
	})

	t.Run("unknown future error falls back to readable text", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(Wrap(futureJSONError{}))
		require.NoError(t, err)
		require.JSONEq(t, `"future error"`, string(encoded))
	})

	t.Run("legacy Error implementations need no JSON method", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(Wrap(legacyError{}))
		require.NoError(t, err)
		require.JSONEq(t, `"legacy error"`, string(encoded))
	})

	t.Run("joined errors retain every identity and readable diagnostic", func(t *testing.T) {
		first := errors.New("same message")
		unrelated := errors.New("same message")
		joined := Join(first, errors.New("second error"))

		encoded, err := strutils.MarshalJSON(joined)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "same message")
		require.Contains(t, string(encoded), "second error")
		require.ErrorIs(t, joined, first)
		require.NotErrorIs(t, joined, unrelated)
	})

	t.Run("multiline errors delegate JSON to their owned error tree", func(t *testing.T) {
		multiline := Multiline().AddStrings("first", "  second")

		encoded, err := strutils.MarshalJSON(multiline)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "first")
		require.Contains(t, string(encoded), "second")
	})
}
