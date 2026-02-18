package strutils_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	strutils "github.com/yusing/goutils/strings"
)

func TestCommaSeperatedList(t *testing.T) {
	require.Equal(t, []string{"a", "b", "c", "d"}, strutils.CommaSeperatedList("a,   b,c,  d"))
}
