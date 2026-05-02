package strutils

import (
	"encoding/json"
	"io"
)

type (
	MarshalerFunc     func(value any) ([]byte, error)
	MarshalIndentFunc func(v any, prefix, indent string) ([]byte, error)
	Encoder           interface {
		Encode(v any) error
		SetEscapeHTML(bool)
		SetIndent(prefix, indent string)
	}
	Decoder         interface{ Decode(v any) error }
	NewEncoderFunc  func(w io.Writer) Encoder
	NewDecoderFunc  func(r io.Reader) Decoder
	UnmarshalerFunc func(data []byte, value any) error
	// JSONValidFunc reports whether a byte slice is syntactically valid JSON.
	JSONValidFunc func([]byte) bool
	// MarshalStringFunc marshals a value to a JSON string (not a quoted JSON string value).
	MarshalStringFunc func(value any) (string, error)
	// UnmarshalStringFunc unmarshals JSON text from a string into value.
	UnmarshalStringFunc func(data string, value any) error
	// JSONValidStringFunc reports whether s is syntactically valid JSON.
	JSONValidStringFunc func(s string) bool
)

var (
	jsonMarshaler     MarshalerFunc     = json.Marshal
	jsonMarshalIndent MarshalIndentFunc = json.MarshalIndent
	jsonNewEncoder    NewEncoderFunc    = func(w io.Writer) Encoder {
		return json.NewEncoder(w)
	}
	jsonNewDecoder NewDecoderFunc = func(r io.Reader) Decoder {
		return json.NewDecoder(r)
	}
	jsonValid         JSONValidFunc     = json.Valid
	jsonUnmarshaler   UnmarshalerFunc   = json.Unmarshal
	jsonMarshalString MarshalStringFunc = func(v any) (string, error) {
		b, err := jsonMarshaler(v)
		return string(b), err
	}
	jsonUnmarshalString UnmarshalStringFunc = func(data string, v any) error {
		return jsonUnmarshaler([]byte(data), v)
	}
	jsonValidString JSONValidStringFunc = func(s string) bool {
		return jsonValid([]byte(s))
	}

	yamlMarshaler   MarshalerFunc
	yamlUnmarshaler UnmarshalerFunc
)

// SetJSONMarshaler sets the JSON marshaler used by MarshalJSON, which defaults to encoding/json.Marshal.
func SetJSONMarshaler(marshaler MarshalerFunc) {
	jsonMarshaler = marshaler
}

// SetJSONUnmarshaler sets the JSON unmarshaler used by UnmarshalJSON, which defaults to encoding/json.Unmarshal.
func SetJSONUnmarshaler(unmarshaler UnmarshalerFunc) {
	jsonUnmarshaler = unmarshaler
}

// SetJSONMarshalIndent sets the JSON indented marshaler used by MarshalJSONIndent, which defaults to encoding/json.MarshalIndent.
func SetJSONMarshalIndent(marshalIndent MarshalIndentFunc) {
	jsonMarshalIndent = marshalIndent
}

// SetJSONNewEncoder sets the factory used by NewJSONEncoder, which defaults to encoding/json.NewEncoder.
func SetJSONNewEncoder(newEncoder NewEncoderFunc) {
	jsonNewEncoder = newEncoder
}

// SetJSONNewDecoder sets the factory used by NewJSONDecoder, which defaults to encoding/json.NewDecoder.
func SetJSONNewDecoder(newDecoder NewDecoderFunc) {
	jsonNewDecoder = newDecoder
}

// SetJSONValid sets the predicate used by ValidJSON, which defaults to encoding/json.Valid.
func SetJSONValid(valid JSONValidFunc) {
	jsonValid = valid
}

// SetJSONMarshalString sets the string marshaler used by MarshalJSONString.
// The default wraps the configured byte marshaler (MarshalJSON).
func SetJSONMarshalString(marshalString MarshalStringFunc) {
	jsonMarshalString = marshalString
}

// SetJSONUnmarshalString sets the string unmarshaler used by UnmarshalJSONString.
// The default wraps the configured byte unmarshaler (UnmarshalJSON).
func SetJSONUnmarshalString(unmarshalString UnmarshalStringFunc) {
	jsonUnmarshalString = unmarshalString
}

// SetJSONValidString sets the predicate used by ValidJSONString.
// The default wraps the configured byte predicate (ValidJSON).
func SetJSONValidString(valid JSONValidStringFunc) {
	jsonValidString = valid
}

// SetYAMLMarshaler sets the YAML marshaler to the given function which will be used by MarshalYAML.
//
// This function must be called before using MarshalYAML.
func SetYAMLMarshaler(marshaler MarshalerFunc) {
	yamlMarshaler = marshaler
}

// SetYAMLUnmarshaler sets the YAML unmarshaler to the given function which will be used by UnmarshalYAML.
//
// This function must be called before using UnmarshalYAML.
func SetYAMLUnmarshaler(unmarshaler UnmarshalerFunc) {
	yamlUnmarshaler = unmarshaler
}

// ValidJSON reports whether b is syntactically valid JSON using the configured predicate (by default encoding/json.Valid).
func ValidJSON(b []byte) bool {
	return jsonValid(b)
}

// ValidJSONString reports whether s is syntactically valid JSON using the configured predicate (by default encoding/json.Valid on the UTF-8 bytes of s).
func ValidJSONString(s string) bool {
	return jsonValidString(s)
}

// MarshalJSON marshals the value to JSON with the configured marshaler, which defaults to encoding/json.Marshal.
func MarshalJSON(value any) ([]byte, error) {
	return jsonMarshaler(value)
}

// MarshalJSONIndent marshals value to indented JSON with the configured marshaler, which defaults to encoding/json.MarshalIndent.
func MarshalJSONIndent(value any, prefix string, indent string) ([]byte, error) {
	return jsonMarshalIndent(value, prefix, indent)
}

// NewJSONEncoder returns a streaming JSON encoder from the configured factory, which defaults to encoding/json.NewEncoder.
func NewJSONEncoder(w io.Writer) Encoder {
	return jsonNewEncoder(w)
}

// NewJSONDecoder returns a streaming JSON decoder from the configured factory, which defaults to encoding/json.NewDecoder.
func NewJSONDecoder(r io.Reader) Decoder {
	return jsonNewDecoder(r)
}

// MarshalYAML marshals the value to YAML with the configured marshaler, it must be set before using this function.
func MarshalYAML(value any) ([]byte, error) {
	return yamlMarshaler(value)
}

// UnmarshalJSON unmarshals data into value with the configured unmarshaler, which defaults to encoding/json.Unmarshal.
func UnmarshalJSON(data []byte, value any) error {
	return jsonUnmarshaler(data, value)
}

// MarshalJSONString marshals value to a JSON string (document text) with the configured marshaler, by default by wrapping MarshalJSON.
func MarshalJSONString(value any) (string, error) {
	return jsonMarshalString(value)
}

// UnmarshalJSONString unmarshals JSON text from data into value with the configured unmarshaler, by default by wrapping UnmarshalJSON.
func UnmarshalJSONString(data string, value any) error {
	return jsonUnmarshalString(data, value)
}

// UnmarshalYAML unmarshals the data to the value with the configured unmarshaler, it must be set before using this function.
func UnmarshalYAML(data []byte, value any) error {
	return yamlUnmarshaler(data, value)
}
