package strutils

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"io"
	"strings"
)

type (
	MarshalerFunc   func(value any) ([]byte, error)
	UnmarshalerFunc func(data []byte, value any) error
	Encoder         interface {
		Encode(v any) error
		SetEscapeHTML(bool)
		SetIndent(prefix, indent string)
	}
	Decoder interface{ Decode(v any) error }
)

// json/v2 has no default encoding for time.Duration. Keep v1's nanosecond
// numbers so existing API and stored JSON (health, HTTP timeouts, ACL) still round-trip.
var jsonOpts = jsonv2.JoinOptions(jsonv1.FormatDurationAsNano(true))

var (
	yamlMarshaler   MarshalerFunc
	yamlUnmarshaler UnmarshalerFunc
)

type jsonEncoder struct {
	w          io.Writer
	prefix     string
	indent     string
	escapeHTML bool
}

func (e *jsonEncoder) Encode(v any) error {
	opts := []jsonv2.Options{jsonOpts}
	if e.prefix != "" || e.indent != "" {
		opts = append(opts, jsontext.WithIndentPrefix(e.prefix), jsontext.WithIndent(e.indent))
	}
	if e.escapeHTML {
		opts = append(opts, jsontext.EscapeForHTML(true))
	}
	return jsonv2.MarshalEncode(jsontext.NewEncoder(e.w), v, opts...)
}

func (e *jsonEncoder) SetEscapeHTML(escape bool) {
	e.escapeHTML = escape
}

func (e *jsonEncoder) SetIndent(prefix, indent string) {
	e.prefix = prefix
	e.indent = indent
}

type jsonDecoder struct {
	dec *jsontext.Decoder
}

func (d *jsonDecoder) Decode(v any) error {
	return jsonv2.UnmarshalDecode(d.dec, v, jsonOpts)
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

// ValidJSON reports whether b is syntactically valid JSON.
func ValidJSON(b []byte) bool {
	return jsontext.Value(b).IsValid()
}

// ValidJSONString reports whether s is syntactically valid JSON.
func ValidJSONString(s string) bool {
	d := jsontext.NewDecoder(strings.NewReader(s))
	_, errVal := d.ReadValue()
	_, errEOF := d.ReadToken()
	return errVal == nil && errEOF == io.EOF
}

// MarshalJSON marshals the value to JSON using encoding/json/v2.
func MarshalJSON(value any) ([]byte, error) {
	return jsonv2.Marshal(value, jsonOpts)
}

// MarshalJSONIndent marshals value to indented JSON using encoding/json/v2.
func MarshalJSONIndent(value any, prefix string, indent string) ([]byte, error) {
	return jsonv2.Marshal(value, jsonOpts, jsontext.WithIndentPrefix(prefix), jsontext.WithIndent(indent))
}

// NewJSONEncoder returns a streaming JSON encoder writing to w.
func NewJSONEncoder(w io.Writer) Encoder {
	return &jsonEncoder{w: w}
}

// NewJSONDecoder returns a streaming JSON decoder reading from r.
func NewJSONDecoder(r io.Reader) Decoder {
	return &jsonDecoder{dec: jsontext.NewDecoder(r)}
}

// MarshalYAML marshals the value to YAML with the configured marshaler, it must be set before using this function.
func MarshalYAML(value any) ([]byte, error) {
	return yamlMarshaler(value)
}

// UnmarshalJSON unmarshals data into value using encoding/json/v2.
func UnmarshalJSON(data []byte, value any) error {
	return jsonv2.Unmarshal(data, value, jsonOpts)
}

// MarshalString marshals value to a JSON document as a string.
func MarshalString(value any) (string, error) {
	var b strings.Builder
	if err := jsonv2.MarshalWrite(&b, value, jsonOpts); err != nil {
		return "", err
	}
	return b.String(), nil
}

// UnmarshalFromString unmarshals JSON text from data into value.
func UnmarshalFromString(data string, value any) error {
	return jsonv2.UnmarshalRead(strings.NewReader(data), value, jsonOpts)
}

// UnmarshalYAML unmarshals the data to the value with the configured unmarshaler, it must be set before using this function.
func UnmarshalYAML(data []byte, value any) error {
	return yamlUnmarshaler(data, value)
}
