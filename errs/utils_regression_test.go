package gperr

import "testing"

type regressionNilUnwrapper struct{}

func (regressionNilUnwrapper) Error() string { return "outer" }
func (regressionNilUnwrapper) Unwrap() error { return nil }

func TestUnwrapNilRegression(t *testing.T) {
	for _, err := range []error{nil, regressionNilUnwrapper{}} {
		if got := Unwrap(err); got != nil {
			t.Fatalf("Unwrap(%T) returned nonnil %T", err, got)
		}
	}
}

func TestDoYouMeanFieldEmptyRegression(t *testing.T) {
	for _, fields := range [][]string{nil, {}} {
		if got := DoYouMeanField("missing", fields); got != nil {
			t.Fatalf("empty candidates returned %v", got)
		}
	}
}
