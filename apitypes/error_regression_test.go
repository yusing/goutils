package apitypes

import (
	"errors"
	"testing"
)

type regressionPlainError struct{}

func (regressionPlainError) Error() string { return "formatted" }
func (regressionPlainError) Plain() []byte { return []byte("plain") }

func TestErrorNilRegression(t *testing.T) {
	for _, tc := range []struct {
		name string
		errs []error
		want string
	}{
		{"none", nil, ""}, {"nil", []error{nil}, ""}, {"all nil", []error{nil, nil}, ""},
		{"first nonnil", []error{nil, errors.New("first"), errors.New("second")}, "first"},
		{"plain", []error{nil, regressionPlainError{}, errors.New("second")}, "plain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Error("message", tc.errs...)
			if got.Message != "message" || got.Error != tc.want {
				t.Fatalf("got %+v, want error %q", got, tc.want)
			}
		})
	}
}
