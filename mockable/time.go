package mockable

import "time"

var TimeNow = time.Now

func MockTimeNow(t time.Time) {
	TimeNow = func() time.Time {
		return t
	}
}
