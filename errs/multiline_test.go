package gperr

import (
	"net"
	"testing"

	expect "github.com/yusing/goutils/testing"
)

func TestWrapMultiline(t *testing.T) {
	multiline := Multiline()
	var wrapper error = wrap(multiline)
	_, ok := wrapper.(*MultilineError)
	if !ok {
		t.Errorf("wrapper is not a MultilineError")
	}
}

func TestPrependSubjectMultiline(t *testing.T) {
	multiline := Multiline()
	multiline.Addf("line 1 %s", "test")
	multiline.Adds("line 2")
	multiline.AddLines([]any{1, "2", 3.0, net.IPv4(127, 0, 0, 1)})
	multiline.Subject("subject")

	builder := NewBuilder()
	builder.Add(multiline)
	expect.Equal(t, len(multiline.currentParent.(*nestedError).Extras), len(builder.errs))
}

func TestFormattingMultiline(t *testing.T) {
	multiline := Multiline()
	multiline.AddStrings(
		"line 1",
		"  line 2",
		"    line 3",
		"  line 4",
		"  line 5",
		"line 6",
	)
	/*
		expected structure

		multiline: currentParent should be nestedError
		line 1: nestedError, first child of multiline.currentParent
		line 2: first child inside line1.Extra, also nestedError
		line 3: first child inside line2.Extra, also nestedError
		line 4: 2nd child inside line1.Extra, baseError
		line 5: 3nd child inside line1.Extra, baseError
		line 6: baseError, 2nd child of multiline.currentParent
	*/
	expect.Equal(t, multiline.Error(),
		`
line 1
  • line 2
    • line 3
  • line 4
  • line 5
line 6
`[1:])
}

func BenchmarkMultiline(b *testing.B) {
	for b.Loop() {
		multiline := Multiline()
		multiline.AddStrings(
			"line 1",
			"  line 2",
			"    line 3",
			"  line 4",
			"  line 5",
			"line 6",
		)
	}
}
