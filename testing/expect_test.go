package expect_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	expect "github.com/yusing/goutils/testing"
)

type sampleError struct{ message string }

func (e *sampleError) Error() string { return e.message }

type sampleString string

func TestEqualAndNotEqual(t *testing.T) {
	for _, tc := range []struct {
		name      string
		got, want any
	}{
		{"signed widths", int8(7), int64(7)},
		{"unsigned widths", uint8(7), uint64(7)},
		{"signed and unsigned", int16(7), uint32(7)},
		{"float widths", float32(1.5), float64(1.5)},
		{"integer and float", int(7), float64(7)},
		{"complex widths", complex64(1 + 2i), complex128(1 + 2i)},
		{"named string", sampleString("hello"), "hello"},
		{"integer to expected float", int64(1), float64(1)},
		{"expected float to larger integer", int64(1), float32(1)},
		{"expected integer to string", "A", 65},
		{"nil", nil, nil},
		{"typed nil", (*int)(nil), (*int)(nil)},
		{"nested values", map[string][]int{"a": {1, 2}}, map[string][]int{"a": {1, 2}}},
	} {
		t.Run(tc.name, func(t *testing.T) { expect.Equal(t, tc.got, tc.want) })
	}
	expect.NotEqual[any](t, int8(7), int64(7)) // Unlike Equal, NotEqual compares types strictly.
	expect.NotEqual[any](t, nil, (*int)(nil))
	expect.NotEqual(t, []int(nil), []int{})
	expect.NotEqual(t, map[string][]int{"a": {1}}, map[string][]int{"a": {2}})
}

func TestNilAndEmpty(t *testing.T) {
	var pointer *int
	var slice []int
	var mapping map[string]int
	var channel chan int
	var function func()
	for _, value := range []any{nil, pointer, slice, mapping, channel, function} {
		expect.Nil(t, value)
		expect.Empty(t, value)
	}
	zero := 0
	for _, value := range []any{0, false, "", []int{}, map[string]int{}, [0]int{}, [1]int{}, [2]string{}, &[1]int{}, &zero, struct{}{}, make(chan int, 1)} {
		expect.Empty(t, value)
	}
	// Arrays are empty when all elements have their type's zero value.
	expect.NotEmpty(t, [1]int{1})
	expect.NotEmpty(t, [2]string{"", "value"})
	buffered := make(chan int, 1)
	buffered <- 1
	one := 1
	for _, value := range []any{1, true, "a", []int{0}, map[string]int{"a": 0}, &one, buffered, func() {}} {
		expect.NotNil(t, value)
		expect.NotEmpty(t, value)
	}
	for _, value := range []any{0, false, "", []int{}, map[string]int{}, make(chan int), &zero} {
		expect.NotNil(t, value)
	}
}

func TestErrorsAndBooleans(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := fmt.Errorf("outer: %w", sentinel)
	expect.NoError(t, nil)
	expect.HasError(t, wrapped)
	expect.ErrorIs(t, sentinel, wrapped)
	expect.ErrorIs(t, nil, nil)
	expect.ErrorT[*sampleError](t, fmt.Errorf("outer: %w", &sampleError{"typed"}))
	expect.ErrorT[error](t, wrapped)
	expect.ErrorContains(t, wrapped, "sentinel")
	expect.True(t, true)
	expect.False(t, false)
}

func TestContainment(t *testing.T) {
	expect.StringsContain(t, "hello world", "world")
	expect.StringsContain(t, "hello", "")
	expect.Contains(t, "b", []string{"a", "b", "c"})
	expect.Contains(t, []int{1, 2}, [][]int{{0}, {1, 2}})
	expect.Contains(t, map[string]int{"a": 1}, []map[string]int{{"a": 1}})
	expect.Contains(t, (*int)(nil), []*int{nil})
}

func TestOrderedComparisons(t *testing.T) {
	expect.Greater(t, 2*time.Second, time.Second)
	expect.Less(t, time.Second, 2*time.Second)
	expect.GreaterOrEqual(t, time.Second, time.Second)
	expect.LessOrEqual(t, time.Second, time.Second)
	expect.Greater(t, "b", "a")
	expect.Less(t, sampleString("a"), sampleString("b"))
	expect.GreaterOrEqual(t, "b", "a")
	expect.LessOrEqual(t, "a", "b")
	expect.Greater(t, 1.5, 1.0)
	expect.Less(t, uint8(1), uint8(2))
}

func TestPanics(t *testing.T) {
	expect.Panics(t, func() { panic("boom") })
	expect.Panics(t, func() { panic(errors.New("boom")) })
	expect.Panics(t, func() { panic(nil) })
}

func TestTypeAndMust(t *testing.T) {
	if got := expect.Type[string](t, "hello"); got != "hello" {
		t.Fatalf("Type returned %q", got)
	}
	var pointer *int
	if got := expect.Type[*int](t, pointer); got != nil {
		t.Fatalf("Type did not preserve typed nil: %v", got)
	}
	err := &sampleError{"typed"}
	if got := expect.Type[error](t, err); got != err {
		t.Fatal("Type did not preserve the value when asserting an interface")
	}
	if got := expect.Must([]int{1, 2}, nil); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("Must returned %v", got)
	}
	sentinel := errors.New("must fail")
	func() {
		defer func() {
			if got := recover(); got != sentinel {
				t.Errorf("Must panic = %v, want original error %v", got, sentinel)
			}
		}()
		expect.Must(42, sentinel)
		t.Error("Must returned after an error")
	}()
}

// A real testing.T must be used: testing.TB has private methods and cannot be mocked.
// Each subprocess must fail, execute cleanup, and never reach the post-assertion marker.
func TestAssertionFailures(t *testing.T) {
	for name := range failureCases() {
		t.Run(name, func(t *testing.T) {
			cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestAssertionFailureProcess$", "-test.timeout=10s")
			cmd.Env = append(os.Environ(), "GOUTILS_EXPECT_FAILURE="+name)
			output, err := cmd.CombinedOutput()
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
				t.Fatalf("expected test failure exit 1, got %v\n%s", err, output)
			}
			text := string(output)
			if !strings.Contains(text, "expect-cleanup-reached") {
				t.Fatalf("failure did not run test cleanup:\n%s", text)
			}
			if strings.Contains(text, "expect-after-assertion") || strings.Contains(text, "\npanic:") {
				t.Fatalf("assertion did not fail fast with t.Fatal:\n%s", text)
			}
			message := "expect-context 42"
			if name == "plain-message" {
				message = "plain-context"
			} else if name == "non-string-message" {
				message = "123 456"
			}
			if !strings.Contains(text, message) {
				t.Fatalf("optional message %q missing:\n%s", message, text)
			}
		})
	}
}

func TestAssertionFailureProcess(t *testing.T) {
	name := os.Getenv("GOUTILS_EXPECT_FAILURE")
	if name == "" {
		return
	}
	assertion, ok := failureCases()[name]
	if !ok {
		t.Fatalf("unknown failure case %q", name)
	}
	t.Cleanup(func() { fmt.Println("expect-cleanup-reached") })
	assertion(t, []any{"expect-context %d", 42})
	fmt.Println("expect-after-assertion")
}

func failureCases() map[string]func(*testing.T, []any) {
	return map[string]func(*testing.T, []any){
		"NoError":                func(t *testing.T, msg []any) { expect.NoError(t, errors.New("unexpected"), msg...) },
		"HasError":               func(t *testing.T, msg []any) { expect.HasError(t, nil, msg...) },
		"True":                   func(t *testing.T, msg []any) { expect.True(t, false, msg...) },
		"False":                  func(t *testing.T, msg []any) { expect.False(t, true, msg...) },
		"Nil":                    func(t *testing.T, msg []any) { expect.Nil(t, []int{}, msg...) },
		"NotNil":                 func(t *testing.T, msg []any) { expect.NotNil(t, (*int)(nil), msg...) },
		"Empty":                  func(t *testing.T, msg []any) { expect.Empty(t, []int{0}, msg...) },
		"NotEmpty":               func(t *testing.T, msg []any) { expect.NotEmpty(t, new(int), msg...) },
		"NotEmpty-zero-array":    func(t *testing.T, msg []any) { expect.NotEmpty(t, [1]int{}, msg...) },
		"Empty-nonzero-array":    func(t *testing.T, msg []any) { expect.Empty(t, [1]int{1}, msg...) },
		"ErrorContains-nil":      func(t *testing.T, msg []any) { expect.ErrorContains(t, nil, "", msg...) },
		"ErrorContains-mismatch": func(t *testing.T, msg []any) { expect.ErrorContains(t, errors.New("hello"), "bye", msg...) },
		"Panics":                 func(t *testing.T, msg []any) { expect.Panics(t, func() {}, msg...) },
		"Greater":                func(t *testing.T, msg []any) { expect.Greater(t, time.Second, time.Second, msg...) },
		"Less":                   func(t *testing.T, msg []any) { expect.Less(t, "a", "a", msg...) },
		"GreaterOrEqual":         func(t *testing.T, msg []any) { expect.GreaterOrEqual(t, 1, 2, msg...) },
		"LessOrEqual":            func(t *testing.T, msg []any) { expect.LessOrEqual(t, 2, 1, msg...) },
		"ErrorIs": func(t *testing.T, msg []any) {
			expect.ErrorIs(t, errors.New("same text"), errors.New("same text"), msg...)
		},
		"ErrorT":                    func(t *testing.T, msg []any) { expect.ErrorT[*sampleError](t, errors.New("other type"), msg...) },
		"Equal":                     func(t *testing.T, msg []any) { expect.Equal(t, []int{1}, []int{2}, msg...) },
		"Equal-numeric":             func(t *testing.T, msg []any) { expect.Equal[any](t, int8(1), int64(257), msg...) },
		"Equal-fractional-expected": func(t *testing.T, msg []any) { expect.Equal[any](t, int64(1), float64(1.5), msg...) },
		"Equal-fractional-actual":   func(t *testing.T, msg []any) { expect.Equal[any](t, float64(1.5), int32(1), msg...) },
		"Equal-string-expected":     func(t *testing.T, msg []any) { expect.Equal[any](t, 65, "A", msg...) },
		"Equal-nil":                 func(t *testing.T, msg []any) { expect.Equal[any](t, nil, (*int)(nil), msg...) },
		"Equal-nil-slice":           func(t *testing.T, msg []any) { expect.Equal(t, []int(nil), []int{}, msg...) },
		"NotEqual": func(t *testing.T, msg []any) {
			expect.NotEqual(t, map[string]int{"a": 1}, map[string]int{"a": 1}, msg...)
		},
		"Contains":           func(t *testing.T, msg []any) { expect.Contains(t, "missing", []string{"other"}, msg...) },
		"Contains-empty":     func(t *testing.T, msg []any) { expect.Contains(t, 1, []int(nil), msg...) },
		"Contains-strict":    func(t *testing.T, msg []any) { expect.Contains[any](t, int8(1), []any{int64(1)}, msg...) },
		"StringsContain":     func(t *testing.T, msg []any) { expect.StringsContain(t, "hello", "bye", msg...) },
		"Type":               func(t *testing.T, msg []any) { expect.Type[string](t, 42, msg...) },
		"Type-nil":           func(t *testing.T, msg []any) { expect.Type[*int](t, nil, msg...) },
		"plain-message":      func(t *testing.T, _ []any) { expect.True(t, false, "plain-context") },
		"non-string-message": func(t *testing.T, _ []any) { expect.True(t, false, 123, 456) },
	}
}
