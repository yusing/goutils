package expect

import (
	"cmp"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

var isTest = strings.HasSuffix(os.Args[0], ".test")

func init() {
	if isTest {
		os.Args = append([]string{os.Args[0], "-test.v"}, os.Args[1:]...)
	}
}

func Must[Result any](r Result, err error) Result {
	if err != nil {
		panic(err)
	}
	return r
}

func check(t testing.TB, ok bool, message string, msgAndArgs []any) {
	t.Helper()
	if ok {
		return
	}
	if len(msgAndArgs) != 0 {
		if format, ok := msgAndArgs[0].(string); ok && len(msgAndArgs) > 1 {
			message += ": " + fmt.Sprintf(format, msgAndArgs[1:]...)
		} else {
			message += ": " + fmt.Sprint(msgAndArgs...)
		}
	}
	t.Fatal(message)
}

func NoError(t testing.TB, err error, msgAndArgs ...any) {
	t.Helper()
	check(t, err == nil, fmt.Sprintf("unexpected error: %v", err), msgAndArgs)
}
func HasError(t testing.TB, err error, msgAndArgs ...any) {
	t.Helper()
	check(t, err != nil, "expected an error", msgAndArgs)
}
func True(t testing.TB, value bool, msgAndArgs ...any) {
	t.Helper()
	check(t, value, "expected true", msgAndArgs)
}
func False(t testing.TB, value bool, msgAndArgs ...any) {
	t.Helper()
	check(t, !value, "expected false", msgAndArgs)
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
func Nil(t testing.TB, value any, msgAndArgs ...any) {
	t.Helper()
	check(t, isNil(value), fmt.Sprintf("expected nil, got %#v", value), msgAndArgs)
}
func NotNil(t testing.TB, value any, msgAndArgs ...any) {
	t.Helper()
	check(t, !isNil(value), "expected a non-nil value", msgAndArgs)
}
func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Pointer:
		return v.IsNil() || isEmpty(v.Elem().Interface())
	default:
		return v.IsZero()
	}
}
func Empty(t testing.TB, value any, msgAndArgs ...any) {
	t.Helper()
	check(t, isEmpty(value), fmt.Sprintf("expected empty, got %#v", value), msgAndArgs)
}
func NotEmpty(t testing.TB, value any, msgAndArgs ...any) {
	t.Helper()
	check(t, !isEmpty(value), "expected a non-empty value", msgAndArgs)
}
func ErrorContains(t testing.TB, err error, substring string, msgAndArgs ...any) {
	t.Helper()
	check(t, err != nil && strings.Contains(err.Error(), substring), fmt.Sprintf("expected error containing %q, got %v", substring, err), msgAndArgs)
}
func Panics(t testing.TB, fn func(), msgAndArgs ...any) {
	t.Helper()
	panicked := false
	func() {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		fn()
	}()
	check(t, panicked, "expected a panic", msgAndArgs)
}
func Greater[T cmp.Ordered](t testing.TB, got, bound T, msgAndArgs ...any) {
	t.Helper()
	check(t, got > bound, fmt.Sprintf("expected %v > %v", got, bound), msgAndArgs)
}
func Less[T cmp.Ordered](t testing.TB, got, bound T, msgAndArgs ...any) {
	t.Helper()
	check(t, got < bound, fmt.Sprintf("expected %v < %v", got, bound), msgAndArgs)
}
func GreaterOrEqual[T cmp.Ordered](t testing.TB, got, bound T, msgAndArgs ...any) {
	t.Helper()
	check(t, got >= bound, fmt.Sprintf("expected %v >= %v", got, bound), msgAndArgs)
}
func LessOrEqual[T cmp.Ordered](t testing.TB, got, bound T, msgAndArgs ...any) {
	t.Helper()
	check(t, got <= bound, fmt.Sprintf("expected %v <= %v", got, bound), msgAndArgs)
}
func ErrorIs(t *testing.T, expected error, err error, msgAndArgs ...any) {
	t.Helper()
	check(t, errors.Is(err, expected), fmt.Sprintf("expected error matching %v, got %v", expected, err), msgAndArgs)
}
func ErrorT[T error](t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	_, ok := errors.AsType[T](err)
	check(t, ok, fmt.Sprintf("expected error type %v, got %T", reflect.TypeFor[T](), err), msgAndArgs)
}

func equalValues(got, want any) bool {
	if reflect.DeepEqual(got, want) {
		return true
	}
	if got == nil || want == nil {
		return false
	}
	g, w := reflect.ValueOf(got), reflect.ValueOf(want)
	gt, wt := g.Type(), w.Type()
	if !wt.ConvertibleTo(gt) {
		return false
	}
	isNumeric := func(k reflect.Kind) bool { return k >= reflect.Int && k <= reflect.Complex128 }
	if !isNumeric(g.Kind()) || !isNumeric(w.Kind()) {
		return reflect.DeepEqual(got, w.Convert(gt).Interface())
	}
	if gt.Size() > wt.Size() {
		return reflect.DeepEqual(got, w.Convert(gt).Interface())
	}
	return reflect.DeepEqual(g.Convert(wt).Interface(), want)
}
func Equal[T any](t *testing.T, got T, want T, msgAndArgs ...any) {
	t.Helper()
	check(t, equalValues(got, want), fmt.Sprintf("got %#v, want %#v", got, want), msgAndArgs)
}
func NotEqual[T any](t *testing.T, got T, want T, msgAndArgs ...any) {
	t.Helper()
	check(t, !reflect.DeepEqual(got, want), fmt.Sprintf("expected values to differ: %#v", got), msgAndArgs)
}
func Contains[T any](t *testing.T, got T, wants []T, msgAndArgs ...any) {
	t.Helper()
	ok := slices.ContainsFunc(wants, func(want T) bool { return reflect.DeepEqual(got, want) })
	check(t, ok, fmt.Sprintf("expected %#v in %#v", got, wants), msgAndArgs)
}
func StringsContain(t *testing.T, got string, want string, msgAndArgs ...any) {
	t.Helper()
	check(t, strings.Contains(got, want), fmt.Sprintf("expected %q to contain %q", got, want), msgAndArgs)
}
func Type[T any](t *testing.T, got any, msgAndArgs ...any) T {
	t.Helper()
	value, ok := got.(T)
	check(t, ok, fmt.Sprintf("expected type %v, got %T", reflect.TypeFor[T](), got), msgAndArgs)
	return value
}
