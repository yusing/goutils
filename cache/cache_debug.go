//go:build debug

package cache

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/rs/zerolog/log"
)

func logCacheExpiredEntry(key any, result any, err error) {
	log.Debug().
		Interface("key", formatResult(key)).
		Interface("value", formatResult(result)).
		AnErr("err", err).
		Msg("cache: expired entry recomputed")
}

func logCacheEvicted(maxEntries int, sizeAfter int, evicted cacheEvictedKV) {
	log.Debug().
		Int("max_entries", maxEntries).
		Int("size_after", sizeAfter).
		Interface("key", formatResult(evicted.key)).
		Interface("result", formatResult(evicted.result)).
		AnErr("error", evicted.err).
		Msg("cache: overflow eviction")
}

func logCacheHit(key any, result any, err error) {
	log.Debug().
		Interface("key", formatResult(key)).
		Interface("result", formatResult(result)).
		AnErr("err", err).
		Msg("cache: hit")
}

func logCacheMiss(key any) {
	log.Debug().
		Interface("key", formatResult(key)).
		Msg("cache: miss")
}

func logCacheUsage(size int, maxEntries int) {
	evt := log.Debug().
		Int("size", size).
		Int("max_entries", maxEntries)
	if maxEntries > 0 {
		evt = evt.Float64("ratio", float64(size)/float64(maxEntries))
	}
	evt.Msg("cache: usage")
}

const formatResultStringMax = 100
const formatResultSliceMax = 10
const formatResultMapMax = 10
const formatResultMaxDepth = 32

// formatResult returns a compact, log-friendly view of a cached key or value: long
// strings and byte buffers are truncated, large slices and maps are capped, structs
// become maps of field names to formatted values, and nesting is summarized
// recursively.
func formatResult(result any) any {
	return formatResultDepth(result, 0)
}

func formatResultDepth(result any, depth int) any {
	if depth > formatResultMaxDepth {
		return "<max depth>"
	}
	if result == nil {
		return nil
	}

	rv := reflect.ValueOf(result)
	if !rv.IsValid() {
		return nil
	}

	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Interface, reflect.Slice:
		if rv.IsNil() {
			return nil
		}
	}

	switch rv.Kind() {
	case reflect.String:
		s := rv.String()
		if len(s) <= formatResultStringMax {
			return s
		}
		return s[:formatResultStringMax] + fmt.Sprintf("...(%d more bytes)", len(s)-formatResultStringMax)
	case reflect.Slice:
		if rv.Type().Elem().Kind() == reflect.Uint8 {
			return formatBytes(rv.Bytes())
		}
		n := min(rv.Len(), formatResultSliceMax)
		elemType := rv.Type().Elem()
		out := reflect.MakeSlice(rv.Type(), n, n)
		elems := make([]any, n)
		assignable := true
		for i := range n {
			formatted := formatResultDepth(rv.Index(i).Interface(), depth+1)
			elems[i] = formatted
			fv := reflect.ValueOf(formatted)
			if !fv.IsValid() || !fv.Type().AssignableTo(elemType) {
				assignable = false
			}
		}
		if assignable {
			for i := range n {
				out.Index(i).Set(reflect.ValueOf(elems[i]))
			}
			return out.Interface()
		}
		return elems
	case reflect.Map:
		type pair struct {
			keyString string
			key       reflect.Value
			value     reflect.Value
		}
		pairs := make([]pair, 0, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			key := iter.Key()
			pairs = append(pairs, pair{keyString: fmt.Sprint(key.Interface()), key: key, value: iter.Value()})
		}
		slices.SortFunc(pairs, func(a, b pair) int {
			switch {
			case a.keyString < b.keyString:
				return -1
			case a.keyString > b.keyString:
				return 1
			default:
				return 0
			}
		})
		pairs = pairs[:min(len(pairs), formatResultMapMax)]
		out := make(map[any]any, len(pairs))
		for _, pair := range pairs {
			out[formatResultDepth(pair.key.Interface(), depth+1)] = formatResultDepth(pair.value.Interface(), depth+1)
		}
		return out
	case reflect.Struct:
		typeOfValue := rv.Type()
		out := make(map[string]any, typeOfValue.NumField())
		for i := range typeOfValue.NumField() {
			field := typeOfValue.Field(i)
			if !field.IsExported() {
				continue
			}
			out[field.Name] = formatResultDepth(rv.Field(i).Interface(), depth+1)
		}
		return out
	case reflect.Pointer:
		return formatResultDepth(rv.Elem().Interface(), depth+1)
	default:
		return rv.Interface()
	}
}

func formatBytes(b []byte) any {
	if len(b) <= formatResultStringMax {
		return b
	}
	prefix := string(b[:formatResultStringMax])
	return prefix + fmt.Sprintf("...(%d more bytes)", len(b)-formatResultStringMax)
}
