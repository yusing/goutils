package strutils

import "encoding/json"

type MarshalerFunc func(value any) ([]byte, error)
type UnmarshalerFunc func(data []byte, value any) error

var (
	jsonMarshaler   MarshalerFunc   = json.Marshal
	jsonUnmarshaler UnmarshalerFunc = json.Unmarshal
	yamlMarshaler   MarshalerFunc
	yamlUnmarshaler UnmarshalerFunc
)

// SetJSONMarshaler sets the JSON marshaler to the given function which will be used by MarshalJSON.
func SetJSONMarshaler(marshaler MarshalerFunc) {
	jsonMarshaler = marshaler
}

// SetJSONUnmarshaler sets the JSON unmarshaler to the given function which will be used by UnmarshalJSON.
func SetJSONUnmarshaler(unmarshaler UnmarshalerFunc) {
	jsonUnmarshaler = unmarshaler
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

// MarshalJSON marshals the value to JSON with the configured marshaler, which is by default json.Marshal.
func MarshalJSON(value any) ([]byte, error) {
	return jsonMarshaler(value)
}

// MarshalYAML marshals the value to YAML with the configured marshaler, it must be set before using this function.
func MarshalYAML(value any) ([]byte, error) {
	return yamlMarshaler(value)
}

// UnmarshalJSON unmarshals the data to the value with the configured unmarshaler, which is by default json.Unmarshal.
func UnmarshalJSON(data []byte, value any) error {
	return jsonUnmarshaler(data, value)
}

// UnmarshalYAML unmarshals the data to the value with the configured unmarshaler, it must be set before using this function.
func UnmarshalYAML(data []byte, value any) error {
	return yamlUnmarshaler(data, value)
}
