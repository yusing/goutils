package websocket

import (
	"fmt"
	"reflect"

	_ "unsafe"

	"github.com/yusing/gointernals"
)

// DeepEqual reports whether x and y are deeply equal.
// It supports numerics, strings, maps, slices, arrays, and structs (exported fields only).
// It's optimized for performance by avoiding reflection for common types and
// adaptively choosing between BFS and DFS traversal strategies.
func DeepEqual(x, y any) bool {
	if x == nil || y == nil {
		return x == y
	}

	v1 := reflect.ValueOf(x)
	v2 := reflect.ValueOf(y)

	if v1.Type() != v2.Type() {
		return false
	}

	return deepEqual(v1, v2, make(map[visit]bool), 0)
}

// visit represents a visit to a pair of values during comparison
type visit struct {
	a1, a2 uintptr
	typ    reflect.Type
	l1, c1 int
	l2, c2 int
}

func markVisited(v1, v2 reflect.Value, visited map[visit]bool) bool {
	switch v1.Kind() {
	case reflect.Pointer:
		if v1.IsNil() || v2.IsNil() {
			return false
		}
		a1, a2 := uintptr(v1.Pointer()), uintptr(v2.Pointer())
		if a1 > a2 {
			a1, a2 = a2, a1
		}
		v := visit{a1: a1, a2: a2, typ: v1.Type()}
		if visited[v] {
			return true
		}
		visited[v] = true
	case reflect.Map:
		if v1.IsNil() || v2.IsNil() {
			return false
		}
		a1, a2 := uintptr(v1.Pointer()), uintptr(v2.Pointer())
		l1, l2 := v1.Len(), v2.Len()
		if a1 > a2 {
			a1, a2 = a2, a1
			l1, l2 = l2, l1
		}
		v := visit{a1: a1, a2: a2, typ: v1.Type(), l1: l1, l2: l2}
		if visited[v] {
			return true
		}
		visited[v] = true
	case reflect.Slice:
		if v1.IsNil() || v2.IsNil() {
			return false
		}
		a1, a2 := uintptr(v1.Pointer()), uintptr(v2.Pointer())
		l1, c1 := v1.Len(), v1.Cap()
		l2, c2 := v2.Len(), v2.Cap()
		if a1 > a2 {
			a1, a2 = a2, a1
			l1, l2 = l2, l1
			c1, c2 = c2, c1
		}
		v := visit{a1: a1, a2: a2, typ: v1.Type(), l1: l1, c1: c1, l2: l2, c2: c2}
		if visited[v] {
			return true
		}
		visited[v] = true
	}
	return false
}

func isInt(kind reflect.Kind) bool {
	return kind >= reflect.Int && kind <= reflect.Uintptr
}

// deepEqual performs the actual deep comparison with cycle detection
func deepEqual(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}

	v1Type, v2Type := v1.Type(), v2.Type()
	if v1Type != v2Type {
		return false
	}

	v1Kind := v1.Kind()
	if isInt(v1Kind) {
		// fast compare ints, don't care about sign difference
		switch v1Type.Bits() {
		case 64:
			return gointernals.ReflectValueAs[int64](v1) == gointernals.ReflectValueAs[int64](v2)
		case 32:
			return gointernals.ReflectValueAs[int32](v1) == gointernals.ReflectValueAs[int32](v2)
		case 16:
			return gointernals.ReflectValueAs[int16](v1) == gointernals.ReflectValueAs[int16](v2)
		case 8:
			return gointernals.ReflectValueAs[int8](v1) == gointernals.ReflectValueAs[int8](v2)
		default:
			panic(fmt.Sprintf("invalid bits: %d", v1Type.Bits()))
		}
	}

	switch v1Kind {
	case reflect.Bool:
		return gointernals.ReflectValueAs[bool](v1) == gointernals.ReflectValueAs[bool](v2)
	case reflect.Float32, reflect.Float64:
		return floatEqual(v1.Float(), v2.Float())
	case reflect.Complex64, reflect.Complex128:
		c1, c2 := v1.Complex(), v2.Complex()
		return floatEqual(real(c1), real(c2)) && floatEqual(imag(c1), imag(c2))
	case reflect.String:
		return v1.String() == v2.String()
	case reflect.Array:
		return deepEqualArray(v1, v2, visited, depth)
	case reflect.Slice:
		if markVisited(v1, v2, visited) {
			return true
		}
		return deepEqualSlice(v1, v2, visited, depth)
	case reflect.Map:
		if markVisited(v1, v2, visited) {
			return true
		}
		return deepEqualMap(v1, v2, visited, depth)
	case reflect.Struct:
		return deepEqualStruct(v1, v2, visited, depth)
	case reflect.Pointer:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.IsNil() {
			return true
		}
		if markVisited(v1, v2, visited) {
			return true
		}
		return deepEqual(v1.Elem(), v2.Elem(), visited, depth+1)
	case reflect.Interface:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.IsNil() {
			return true
		}
		return deepEqual(v1.Elem(), v2.Elem(), visited, depth+1)
	case reflect.Func:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return false
	default:
		if !v1Type.Comparable() {
			return false
		}
		return v1.Interface() == v2.Interface()
	}
}

// floatEqual handles NaN cases properly
func floatEqual(f1, f2 float64) bool {
	return f1 == f2 || (f1 != f1 && f2 != f2) // NaN == NaN
}

// deepEqualArray compares arrays using DFS (since arrays have fixed size)
func deepEqualArray(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	for i := range v1.Len() {
		if !deepEqual(v1.Index(i), v2.Index(i), visited, depth+1) {
			return false
		}
	}
	return true
}

// deepEqualSlice compares slices, choosing strategy based on size and depth
func deepEqualSlice(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	if v1.IsNil() != v2.IsNil() {
		return false
	}
	if v1.Len() != v2.Len() {
		return false
	}
	if v1.IsNil() {
		return true
	}

	// Use BFS for large slices at shallow depth to improve cache locality
	// Use DFS for small slices or deep nesting to reduce memory overhead
	if shouldUseBFS(v1.Len(), depth) {
		return deepEqualSliceBFS(v1, v2, visited, depth)
	}
	return deepEqualSliceDFS(v1, v2, visited, depth)
}

// deepEqualSliceDFS uses depth-first traversal
func deepEqualSliceDFS(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	for i := range v1.Len() {
		if !deepEqual(v1.Index(i), v2.Index(i), visited, depth+1) {
			return false
		}
	}
	return true
}

// deepEqualSliceBFS uses breadth-first traversal for better cache locality
func deepEqualSliceBFS(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	length := v1.Len()

	// First, check all direct elements
	for i := range length {
		elem1, elem2 := v1.Index(i), v2.Index(i)

		// For simple types, compare directly
		if isSimpleType(elem1.Kind()) {
			if !deepEqual(elem1, elem2, visited, depth+1) {
				return false
			}
		}
	}

	// Then, recursively check complex elements
	for i := range length {
		elem1, elem2 := v1.Index(i), v2.Index(i)

		if !isSimpleType(elem1.Kind()) {
			if !deepEqual(elem1, elem2, visited, depth+1) {
				return false
			}
		}
	}

	return true
}

// deepEqualMap compares maps
func deepEqualMap(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	if v1.IsNil() != v2.IsNil() {
		return false
	}
	if v1.Len() != v2.Len() {
		return false
	}
	if v1.IsNil() {
		return true
	}

	// Check all keys and values
	for _, key := range v1.MapKeys() {
		val1 := v1.MapIndex(key)
		val2 := v2.MapIndex(key)

		if !val2.IsValid() {
			return false // key doesn't exist in v2
		}

		if !deepEqual(val1, val2, visited, depth+1) {
			return false
		}
	}

	return true
}

// deepEqualStruct compares structs (exported fields only)
func deepEqualStruct(v1, v2 reflect.Value, visited map[visit]bool, depth int) bool {
	typ := v1.Type()

	for i := range typ.NumField() {
		field := typ.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		if !deepEqual(v1.Field(i), v2.Field(i), visited, depth+1) {
			return false
		}
	}

	return true
}

// shouldUseBFS determines whether to use BFS or DFS based on slice size and depth
func shouldUseBFS(length, depth int) bool {
	// Use BFS for large slices at shallow depth (better cache locality)
	// Use DFS for small slices or deep nesting (lower memory overhead)
	return length > 100 && depth < 3
}

// isSimpleType checks if a type can be compared without deep recursion
func isSimpleType(kind reflect.Kind) bool {
	if kind >= reflect.Bool && kind <= reflect.Complex128 {
		return true
	}
	return kind == reflect.String
}
