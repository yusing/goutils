package env

import (
	"errors"
	"os"
	"testing"
	"time"

	expect "github.com/yusing/goutils/testing"
)

var originalPrefixes = envPrefixes

func cleanup() {
	envPrefixes = originalPrefixes
	for _, env := range os.Environ() {
		os.Unsetenv(env)
	}
}

func TestSetPrefixes(t *testing.T) {
	t.Cleanup(cleanup)

	// Test setting prefixes
	SetPrefixes("TEST_", "APP_")
	expect.Equal(t, envPrefixes, []string{"TEST_", "APP_"})

	// Test empty prefixes
	SetPrefixes()
	expect.Equal(t, envPrefixes, []string{""})
}

func TestGetEnvString(t *testing.T) {
	t.Cleanup(cleanup)

	// Test without prefixes
	SetPrefixes()
	key := "TEST_STRING_VAR"

	// Test default value when env var not set
	result := GetEnvString(key, "default")
	expect.Equal(t, result, "default")

	// Test getting set value
	os.Setenv(key, "test_value")
	result = GetEnvString(key, "default")
	expect.Equal(t, result, "test_value")

	// Test empty string value (should return default)
	os.Setenv(key, "")
	result = GetEnvString(key, "default")
	expect.Equal(t, result, "default")
}

func TestGetEnvStringWithPrefixes(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes("TEST_", "APP_")

	key := "STRING_VAR"
	testKey1 := "TEST_STRING_VAR"
	testKey2 := "APP_STRING_VAR"
	defer func() {
		os.Unsetenv(testKey1)
		os.Unsetenv(testKey2)
	}()

	// Test first prefix match
	os.Setenv(testKey1, "first_prefix_value")
	result := GetEnvString(key, "default")
	expect.Equal(t, result, "first_prefix_value")

	// Clean up and test second prefix
	os.Unsetenv(testKey1)
	os.Setenv(testKey2, "second_prefix_value")
	result = GetEnvString(key, "default")
	expect.Equal(t, result, "second_prefix_value")

	// Test default when no prefixes match
	os.Unsetenv(testKey2)
	result = GetEnvString(key, "default")
	expect.Equal(t, result, "default")
}

func TestGetEnvBool(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes()
	key := "TEST_BOOL_VAR"

	// Test default value when env var not set
	result := GetEnvBool(key, true)
	expect.True(t, result)

	result = GetEnvBool(key, false)
	expect.False(t, result)

	// Test true values
	trueValues := []string{"true", "TRUE", "True", "1", "t", "T"}
	for _, val := range trueValues {
		os.Setenv(key, val)
		result := GetEnvBool(key, false)
		expect.True(t, result, "Expected true for value: %s", val)
	}

	// Test false values
	falseValues := []string{"false", "FALSE", "False", "0", "f", "F"}
	for _, val := range falseValues {
		os.Setenv(key, val)
		result := GetEnvBool(key, true)
		expect.False(t, result, "Expected false for value: %s", val)
	}

	// Test invalid value (should panic)
	os.Setenv(key, "invalid")
	expect.Panics(t, func() {
		GetEnvBool(key, false)
	})
}

func TestGetEnvInt(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes()
	key := "TEST_INT_VAR"

	// Test default value when env var not set
	result := GetEnvInt(key, 42)
	expect.Equal(t, result, 42)

	// Test valid integer values
	testCases := []struct {
		input    string
		expected int
	}{
		{"0", 0},
		{"123", 123},
		{"-456", -456},
		{"2147483647", 2147483647},   // max int32
		{"-2147483648", -2147483648}, // min int32
	}

	for _, tc := range testCases {
		os.Setenv(key, tc.input)
		result := GetEnvInt(key, 0)
		expect.Equal(t, result, tc.expected, "Failed for input: %s", tc.input)
	}

	// Test invalid value (should panic)
	os.Setenv(key, "not_a_number")
	expect.Panics(t, func() {
		GetEnvInt(key, 0)
	})

	// Test float value (should panic)
	os.Setenv(key, "3.14")
	expect.Panics(t, func() {
		GetEnvInt(key, 0)
	})
}

func TestGetAddrEnv(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes()
	key := "TEST_ADDR_VAR"

	// Test default/empty value
	addr, host, portInt, fullURL := GetAddrEnv(key, "", "http")
	expect.Equal(t, addr, "")
	expect.Equal(t, host, "")
	expect.Equal(t, portInt, 0)
	expect.Equal(t, fullURL, "")

	// Test valid address with host and port
	os.Setenv(key, "example.com:8080")
	addr, host, portInt, fullURL = GetAddrEnv(key, "", "https")
	expect.Equal(t, addr, "example.com:8080")
	expect.Equal(t, host, "example.com")
	expect.Equal(t, portInt, 8080)
	expect.Equal(t, fullURL, "https://example.com:8080")

	// Test empty host
	os.Setenv(key, ":3000")
	addr, host, portInt, fullURL = GetAddrEnv(key, "", "http")
	expect.Equal(t, addr, ":3000")
	expect.Empty(t, host)
	expect.Equal(t, portInt, 3000)
	expect.Equal(t, fullURL, "http://:3000")

	// Test IPv6 address
	os.Setenv(key, "[::1]:8080")
	addr, host, portInt, fullURL = GetAddrEnv(key, "", "http")
	expect.Equal(t, addr, "[::1]:8080")
	expect.Equal(t, host, "::1")
	expect.Equal(t, portInt, 8080)
	expect.Equal(t, fullURL, "http://::1:8080")

	// Test invalid address (should panic)
	os.Setenv(key, "invalid_address")
	expect.Panics(t, func() {
		GetAddrEnv(key, "", "http")
	})

	// Test address without port (should panic)
	os.Setenv(key, "example.com")
	expect.Panics(t, func() {
		GetAddrEnv(key, "", "http")
	})

	// Test invalid port (should panic)
	os.Setenv(key, "example.com:abc")
	expect.Panics(t, func() {
		GetAddrEnv(key, "", "http")
	})
}

func TestGetEnvDuation(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes()
	key := "TEST_DURATION_VAR"

	// Test default value when env var not set
	result := GetEnvDuation(key, time.Minute)
	expect.Equal(t, result, time.Minute)

	// Test valid duration values
	testCases := []struct {
		input    string
		expected time.Duration
	}{
		{"1s", time.Second},
		{"5m", 5 * time.Minute},
		{"2h30m", 2*time.Hour + 30*time.Minute},
		{"100ms", 100 * time.Millisecond},
		{"1h", time.Hour},
	}

	for _, tc := range testCases {
		os.Setenv(key, tc.input)
		result := GetEnvDuation(key, 0)
		expect.Equal(t, result, tc.expected, "Failed for input: %s", tc.input)
	}

	// Test invalid duration (should panic)
	os.Setenv(key, "invalid_duration")
	expect.Panics(t, func() {
		GetEnvDuation(key, time.Second)
	})
}

func TestGetEnvCommaSep(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes()
	key := "TEST_COMMA_VAR"

	// Test default value when env var not set
	result := GetEnvCommaSep(key, "a,b,c")
	expected := []string{"a", "b", "c"}
	expect.Equal(t, result, expected)

	// Test single value
	os.Setenv(key, "single")
	result = GetEnvCommaSep(key, "")
	expect.Equal(t, result, []string{"single"})

	// Test multiple values with spaces
	os.Setenv(key, "  val1  ,  val2  , val3 ")
	result = GetEnvCommaSep(key, "")
	expect.Equal(t, result, []string{"val1", "val2", "val3"})

	// Test empty values
	os.Setenv(key, "val1,,val3,")
	result = GetEnvCommaSep(key, "")
	expect.Equal(t, result, []string{"val1", "", "val3", ""})

	// Test empty string
	os.Setenv(key, "")
	result = GetEnvCommaSep(key, "default1,default2")
	expect.Equal(t, result, []string{"default1", "default2"})
}

func TestGetEnvGeneric(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes()
	key := "TEST_GENERIC_VAR"

	// Test with custom parser
	customParser := func(s string) (int, error) {
		if s == "double" {
			return 42, nil
		}
		if s == "invalid" {
			return 0, errors.New("invalid")
		}
		return 0, nil
	}

	// Test default value
	result := GetEnv(key, 100, customParser)
	expect.Equal(t, result, 100)

	// Test parsed value
	os.Setenv(key, "double")
	result = GetEnv(key, 100, customParser)
	expect.Equal(t, result, 42)

	// Test parser error (should panic)
	os.Setenv(key, "invalid")
	expect.Panics(t, func() {
		GetEnv(key, 100, customParser)
	})
}

// Test with prefixes for GetEnv
func TestGetEnvWithPrefixes(t *testing.T) {
	t.Cleanup(cleanup)

	SetPrefixes("TEST_", "APP_")

	key := "GENERIC_VAR"
	testKey1 := "TEST_GENERIC_VAR"
	testKey2 := "APP_GENERIC_VAR"
	defer func() {
		os.Unsetenv(testKey1)
		os.Unsetenv(testKey2)
	}()

	customParser := func(s string) (string, error) {
		return "[" + s + "]", nil
	}

	// Test first prefix match
	os.Setenv(testKey1, "first")
	result := GetEnv(key, "default", customParser)
	expect.Equal(t, result, "[first]")

	// Clean up and test second prefix
	os.Unsetenv(testKey1)
	os.Setenv(testKey2, "second")
	result = GetEnv(key, "default", customParser)
	expect.Equal(t, result, "[second]")

	// Test default when no prefixes match
	os.Unsetenv(testKey2)
	result = GetEnv(key, "default", customParser)
	expect.Equal(t, result, "default")
}
