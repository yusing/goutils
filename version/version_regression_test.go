package version

import "testing"

func TestStrictOlderRegression(t *testing.T) {
	v := New(2, 3, 4)
	for _, tc := range []struct {
		other             Version
		older, olderMajor bool
	}{
		{New(2, 3, 4), false, false}, {New(2, 3, 5), true, false}, {New(2, 3, 3), false, false},
		{New(2, 4, 0), true, true}, {New(2, 2, 99), false, false}, {New(3, 0, 0), true, true}, {New(1, 99, 99), false, false},
	} {
		if got := v.IsOlderThan(tc.other); got != tc.older {
			t.Errorf("%v.IsOlderThan(%v) = %v", v, tc.other, got)
		}
		if got := v.IsOlderMajorThan(tc.other); got != tc.olderMajor {
			t.Errorf("%v.IsOlderMajorThan(%v) = %v", v, tc.other, got)
		}
	}
}
