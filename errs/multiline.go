package gperr

import (
	"fmt"
	"unicode"
)

type MultilineError struct {
	lastParent, currentParent Error
	baseIndent                int
	parentStack               []Error
	indentStack               []int
}

func Multiline() *MultilineError {
	return &MultilineError{}
}

func (m *MultilineError) Error() string {
	if m.currentParent == nil {
		return ""
	}
	return m.currentParent.Error()
}

func (m *MultilineError) Is(other error) bool {
	if m.currentParent == nil {
		return other == nil
	}
	return m.currentParent.Is(other)
}

func (m *MultilineError) Unwrap() error {
	return m.currentParent
}

func (m *MultilineError) Markdown() []byte {
	if m.currentParent == nil {
		return nil
	}
	return m.currentParent.Markdown()
}

func (m *MultilineError) Plain() []byte {
	if m.currentParent == nil {
		return nil
	}
	return m.currentParent.Plain()
}

func (m *MultilineError) Subject(subject string) Error {
	if m.currentParent == nil {
		m.currentParent = New(subject)
	} else {
		m.currentParent = m.currentParent.Subject(subject)
	}
	return m
}

func (m *MultilineError) Subjectf(format string, args ...any) Error {
	if m.currentParent == nil {
		m.currentParent = Errorf(format, args...)
	} else {
		m.currentParent = m.currentParent.Subjectf(format, args...)
	}
	return m
}

func (m *MultilineError) With(err error) Error {
	if m.currentParent == nil {
		m.currentParent = wrap(err)
	} else {
		m.currentParent = m.currentParent.With(err)
	}
	return m
}

func (m *MultilineError) Withf(format string, args ...any) Error {
	if m.currentParent == nil {
		m.currentParent = Errorf(format, args...)
	} else {
		m.currentParent = m.currentParent.Withf(format, args...)
	}
	return m
}

// add appends the error to the current parent
func (m *MultilineError) add(err error) {
	if err == nil {
		return
	}
	switch parent := m.currentParent.(type) {
	case *nestedError:
		parent.Extras = append(parent.Extras, err)
	case *MultilineError:
		parent.add(err)
	default:
		m.currentParent = &nestedError{Extras: []error{parent, err}}
	}
}

func (m *MultilineError) Addf(format string, args ...any) *MultilineError {
	m.Adds(fmt.Sprintf(format, args...))
	return m
}

func (m *MultilineError) Adds(s string) *MultilineError {
	indent := countIndent(s)
	// trim leading spaces, they will be added by the nestedError.Error() method
	s = s[indent:]
	newErr := New(s)

	if m.currentParent == nil {
		// First line - create the base error and wrap in nestedError as root
		m.currentParent = &nestedError{Err: nil, Extras: []error{newErr}}
		m.baseIndent = indent
		m.parentStack = []Error{newErr}
		m.indentStack = []int{indent}
		return m
	}

	if indent > m.baseIndent {
		// Indent increasing - add as child of the last added error
		lastErr := m.parentStack[len(m.parentStack)-1]
		m.ensureNested(lastErr).Extras = append(m.ensureNested(lastErr).Extras, newErr)
		m.parentStack = append(m.parentStack, newErr)
		m.indentStack = append(m.indentStack, indent)
		m.baseIndent = indent
	} else if indent < m.baseIndent {
		// Indent decreasing - find the appropriate parent from stack
		m.updateStackForIndent(indent)
		parent := m.findParentForIndent(indent)

		if parent != nil {
			m.ensureNested(parent).Extras = append(m.ensureNested(parent).Extras, newErr)
		} else {
			// At root level
			m.currentParent.(*nestedError).Extras = append(m.currentParent.(*nestedError).Extras, newErr)
		}
		m.parentStack = append(m.parentStack, newErr)
		m.indentStack = append(m.indentStack, indent)
		m.baseIndent = indent
	} else {
		// Same indent level - add as sibling
		parent := m.findParentForIndent(indent)
		if parent != nil {
			m.ensureNested(parent).Extras = append(m.ensureNested(parent).Extras, newErr)
		} else {
			// At root level
			m.currentParent.(*nestedError).Extras = append(m.currentParent.(*nestedError).Extras, newErr)
		}
		// Replace last entry in stack with new error at same level
		m.parentStack[len(m.parentStack)-1] = newErr
	}
	return m
}

// ensureNested converts an Error to a nestedError, updating the stack if needed
func (m *MultilineError) ensureNested(err Error) *nestedError {
	if nested, ok := err.(*nestedError); ok {
		return nested
	}
	// Convert baseError to nestedError
	base := err.(baseError)
	nested := &nestedError{Err: base.Err, Extras: []error{}}
	// Update in stack
	for i, stackErr := range m.parentStack {
		if stackErr == err {
			m.parentStack[i] = nested
			break
		}
	}
	// Update in parent's extras
	if m.currentParent != nil {
		if rootNested, ok := m.currentParent.(*nestedError); ok {
			for i, extra := range rootNested.Extras {
				if extra == err {
					rootNested.Extras[i] = nested
					break
				}
			}
		}
	}
	// Also need to update in any parent's extras
	for _, parent := range m.parentStack {
		if parentNested, ok := parent.(*nestedError); ok {
			for i, extra := range parentNested.Extras {
				if extra == err {
					parentNested.Extras[i] = nested
					break
				}
			}
		}
	}
	return nested
}

func (m *MultilineError) findParentForIndent(indent int) Error {
	// Find the parent with the closest smaller indent
	for i := len(m.indentStack) - 1; i >= 0; i-- {
		if m.indentStack[i] < indent {
			return m.parentStack[i]
		}
	}
	return nil
}

func (m *MultilineError) updateStackForIndent(indent int) {
	// Remove all entries with indent >= current indent
	for i := len(m.indentStack) - 1; i >= 0; i-- {
		if m.indentStack[i] >= indent {
			m.parentStack = m.parentStack[:i]
			m.indentStack = m.indentStack[:i]
		} else {
			break
		}
	}
}

func (m *MultilineError) AddStrings(lines ...string) *MultilineError {
	for _, line := range lines {
		m.Adds(line)
	}
	return m
}

func (m *MultilineError) AddLines(lines ...any) *MultilineError {
	for _, line := range lines {
		switch v := line.(type) {
		case string:
			m.Adds(v)
		case fmt.Stringer:
			m.Adds(v.String())
		case error:
			m.add(v)
		default:
			m.add(fmt.Errorf("%v", v))
		}
	}
	return m
}
func countIndent(line string) (n int) {
	for _, r := range line {
		if !unicode.IsSpace(r) {
			break
		}
		n++
	}
	return
}
