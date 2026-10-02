package strutils_test

import (
	"testing"

	strutils "github.com/yusing/goutils/strings"
	expect "github.com/yusing/goutils/testing"
)

func TestCommaSeperatedList(t *testing.T) {
	expect.Equal(t, strutils.CommaSeperatedList("a,   b,c,  d"), []string{"a", "b", "c", "d"})
}
