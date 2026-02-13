package strutils_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yusing/goutils/mockable"
	. "github.com/yusing/goutils/strings"
)

func TestNewUUIDv7(t *testing.T) {
	mockable.TimeNow = func() time.Time {
		t, _ := time.Parse(time.RFC3339Nano, "2016-06-02T01:02:03.456000000Z")
		return t
	}

	id := NewUUIDv7()
	require.Equal(t, "01550ea1-a8c0-7001-8000-000000000000", id)
}
