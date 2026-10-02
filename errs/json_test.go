package gperr

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"reflect"
	"testing"

	strutils "github.com/yusing/goutils/strings"
	expect "github.com/yusing/goutils/testing"
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

type textMarshalingError struct {
	text string
	err  error
}

func (err textMarshalingError) Error() string { return "text marshaling error" }

func (err textMarshalingError) MarshalText() ([]byte, error) {
	return []byte(err.text), err.err
}

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
		expect.Nil(t, Wrap(nil))
	})

	t.Run("plain error is readable and retains identity", func(t *testing.T) {
		sentinel := errors.New("plain error")
		err := Wrap(sentinel)

		encoded, marshalErr := strutils.MarshalJSON(err)
		expect.NoError(t, marshalErr)
		assertJSONEqual(t, `"plain error"`, string(encoded))
		expect.ErrorIs(t, sentinel, err)
	})

	t.Run("underlying structured JSON is preserved", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(Wrap(structuredJSONError{Kind: "structured"}))
		expect.NoError(t, err)
		assertJSONEqual(t, `{"kind":"structured"}`, string(encoded))
	})

	t.Run("malformed underlying JSON is propagated", func(t *testing.T) {
		sentinel := errors.New("marshal failed")
		_, err := strutils.MarshalJSON(Wrap(malformedJSONError{err: sentinel}))
		expect.ErrorIs(t, sentinel, err)
	})

	t.Run("unknown future error falls back to readable text", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(Wrap(futureJSONError{}))
		expect.NoError(t, err)
		assertJSONEqual(t, `"future error"`, string(encoded))
	})

	t.Run("legacy Error implementations need no JSON method", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(Wrap(legacyError{}))
		expect.NoError(t, err)
		assertJSONEqual(t, `"legacy error"`, string(encoded))
	})

	t.Run("text marshaling errors are encoded as JSON strings", func(t *testing.T) {
		errs := NewBuilder("middleware errors")
		errs.Add(New("unknown field").With(DoYouMean("Header")))

		encoded, err := strutils.MarshalJSON(errs.Error())
		expect.NoError(t, err)
		assertJSONEqual(t, `{
			"err": "middleware errors",
			"extras": [{
				"err": "unknown field",
				"extras": ["Do you mean Header?"]
			}]
		}`, string(encoded))
	})

	t.Run("JSON-looking marshaled text remains text", func(t *testing.T) {
		encoded, err := strutils.MarshalJSON(Wrap(textMarshalingError{text: "null"}))
		expect.NoError(t, err)
		assertJSONEqual(t, `"null"`, string(encoded))
	})

	t.Run("text marshaling errors are propagated", func(t *testing.T) {
		sentinel := errors.New("text marshal failed")
		_, err := strutils.MarshalJSON(Wrap(textMarshalingError{err: sentinel}))
		expect.ErrorIs(t, sentinel, err)
	})

	t.Run("joined errors retain every identity and readable diagnostic", func(t *testing.T) {
		first := errors.New("same message")
		unrelated := errors.New("same message")
		joined := Join(first, errors.New("second error"))

		encoded, err := strutils.MarshalJSON(joined)
		expect.NoError(t, err)
		expect.StringsContain(t, string(encoded), "same message")
		expect.StringsContain(t, string(encoded), "second error")
		expect.ErrorIs(t, first, joined)
		if errors.Is(joined, unrelated) {
			t.Fatalf("error %v unexpectedly matches %v", joined, unrelated)
		}
	})

	t.Run("multiline errors delegate JSON to their owned error tree", func(t *testing.T) {
		multiline := Multiline().AddStrings("first", "  second")

		encoded, err := strutils.MarshalJSON(multiline)
		expect.NoError(t, err)
		expect.StringsContain(t, string(encoded), "first")
		expect.StringsContain(t, string(encoded), "second")
	})
}

// assertJSONEqual compares JSON values independently of object member order and whitespace.
func assertJSONEqual(t *testing.T, want, got string) {
	t.Helper()
	var wantValue, gotValue any
	expect.NoError(t, jsonv2.Unmarshal([]byte(want), &wantValue))
	expect.NoError(t, jsonv2.Unmarshal([]byte(got), &gotValue))
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("JSON values differ: got %s, want %s", got, want)
	}
}
