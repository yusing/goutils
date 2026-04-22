//go:build debug

package cache

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatResult_nil(t *testing.T) {
	t.Parallel()
	assert.Nil(t, formatResult(nil))

	var p *int
	assert.Nil(t, formatResult(p))

	var s []byte
	assert.Nil(t, formatResult(s))
}

func TestFormatResult_string(t *testing.T) {
	t.Parallel()
	short := strings.Repeat("a", 50)
	assert.Equal(t, short, formatResult(short))

	exactlyMax := strings.Repeat("a", 100)
	assert.Equal(t, exactlyMax, formatResult(exactlyMax))

	over := strings.Repeat("b", 101)
	want := strings.Repeat("b", 100) + "...(1 more bytes)"
	assert.Equal(t, want, formatResult(over))
}

func TestFormatResult_byteSlice(t *testing.T) {
	t.Parallel()
	small := []byte("hello")
	assert.Equal(t, small, formatResult(small))

	big := make([]byte, 120)
	for i := range big {
		big[i] = 'z'
	}
	got, ok := formatResult(big).(string)
	require.True(t, ok)
	assert.Contains(t, got, "...(20 more bytes)")
	assert.Less(t, len(got), len(string(big)))
}

func TestFormatResult_intSlice(t *testing.T) {
	t.Parallel()
	short := []int{1, 2, 3}
	assert.Equal(t, short, formatResult(short))

	long := make([]int, 12)
	for i := range long {
		long[i] = i
	}
	got, ok := formatResult(long).([]int)
	require.True(t, ok)
	assert.Equal(t, long[:10], got)
}

func TestFormatResult_map(t *testing.T) {
	t.Parallel()
	small := map[string]int{"a": 1, "b": 2}
	gotSmall, ok := formatResult(small).(map[any]any)
	require.True(t, ok)
	assert.Len(t, gotSmall, 2)
	assert.Equal(t, 1, gotSmall["a"])
	assert.Equal(t, 2, gotSmall["b"])

	big := make(map[string]int, 11)
	for i := range 11 {
		big[string(rune('a'+i))] = i
	}
	out, ok := formatResult(big).(map[any]any)
	require.True(t, ok)
	require.Len(t, out, 10)
	_, hasK := out["k"]
	assert.False(t, hasK)
	_, hasA := out["a"]
	assert.True(t, hasA)
}

func TestFormatResult_mapLargeValuesRecursively(t *testing.T) {
	t.Parallel()
	big := make(map[string]any)
	for i := range 11 {
		big[fmt.Sprintf("k%d", i)] = string(make([]byte, 120))
	}
	out, ok := formatResult(big).(map[any]any)
	require.True(t, ok)
	require.Len(t, out, 10)
	for _, v := range out {
		s, ok := v.(string)
		require.True(t, ok)
		assert.Contains(t, s, "...(20 more bytes)")
	}
}

func TestFormatResult_defaultKinds(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 42, formatResult(42))
	assert.Equal(t, true, formatResult(true))
}

func TestFormatResult_keyLongString(t *testing.T) {
	t.Parallel()
	over := strings.Repeat("k", 101)
	want := strings.Repeat("k", 100) + "...(1 more bytes)"
	assert.Equal(t, want, formatResult(over))
}

func TestFormatResult_smallMapLargeInnerString(t *testing.T) {
	t.Parallel()
	m := map[string]string{"id": "x", "Icon": strings.Repeat("A", 200)}
	out, ok := formatResult(m).(map[any]any)
	require.True(t, ok)
	require.Len(t, out, 2)
	icon, ok := out["Icon"].(string)
	require.True(t, ok)
	assert.Contains(t, icon, "...(100 more bytes)")
}

type iconHolder struct {
	Icon string
	ID   int
}

func TestFormatResult_structLargeField(t *testing.T) {
	t.Parallel()
	h := iconHolder{Icon: strings.Repeat("B", 200), ID: 1}
	out, ok := formatResult(h).(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, out["ID"])
	icon, ok := out["Icon"].(string)
	require.True(t, ok)
	assert.Contains(t, icon, "...(100 more bytes)")
}

func TestFormatResult_pointerToStruct(t *testing.T) {
	t.Parallel()
	h := &iconHolder{Icon: strings.Repeat("C", 200), ID: 2}
	out, ok := formatResult(h).(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 2, out["ID"])
	icon, ok := out["Icon"].(string)
	require.True(t, ok)
	assert.Contains(t, icon, "...(100 more bytes)")
}
