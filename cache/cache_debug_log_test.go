//go:build debug

package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeLogLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(buf.String())
	if trimmed == "" {
		return nil
	}

	lines := strings.Split(trimmed, "\n")
	entries := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &entry))
		entries = append(entries, entry)
	}
	return entries
}

func withCapturedDebugLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := log.Logger
	log.Logger = zerolog.New(buf).Level(zerolog.DebugLevel)
	t.Cleanup(func() {
		log.Logger = prev
	})
	return buf
}

func TestCacheDebugLogPayloads_CachedFunc(t *testing.T) {
	buf := withCapturedDebugLogger(t)

	var calls int
	cached := NewFunc(func(ctx context.Context) (string, error) {
		calls++
		return strings.Repeat("x", 120), errors.New("boom")
	}).WithTTL(20 * time.Millisecond).Build()

	result, err := cached(context.Background())
	require.Error(t, err)
	require.Len(t, result, 120)

	result, err = cached(context.Background())
	require.Error(t, err)
	require.Len(t, result, 120)

	time.Sleep(30 * time.Millisecond)
	_, _ = cached(context.Background())

	entries := decodeLogLines(t, buf)
	require.Len(t, entries, 4)

	assert.Equal(t, "cache: miss", entries[0]["message"])
	assert.Equal(t, "<func>", entries[0]["key"])

	assert.Equal(t, "cache: usage", entries[1]["message"])
	assert.EqualValues(t, 1, entries[1]["size"])
	assert.EqualValues(t, 1, entries[1]["max_entries"])
	assert.EqualValues(t, 1, entries[1]["ratio"])

	assert.Equal(t, "cache: hit", entries[2]["message"])
	assert.Equal(t, "<func>", entries[2]["key"])
	resultSummary, ok := entries[2]["result"].(string)
	require.True(t, ok)
	assert.Contains(t, resultSummary, "...(20 more bytes)")
	assert.Equal(t, "boom", entries[2]["err"])

	assert.Equal(t, "cache: expired entry recomputed", entries[3]["message"])
	assert.Equal(t, "<func>", entries[3]["key"])
	valueSummary, ok := entries[3]["value"].(string)
	require.True(t, ok)
	assert.Contains(t, valueSummary, "...(20 more bytes)")
	assert.Equal(t, "boom", entries[3]["err"])
	assert.Equal(t, 2, calls)
}

func TestCacheDebugLogPayloads_KeyedEvictionIncludesSummarizedSnapshot(t *testing.T) {
	buf := withCapturedDebugLogger(t)

	state := &CachedContextKeyFuncState[string, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[string, int]{
			maxEntries: 1,
		},
		entries:   xsync.NewMap[int, *CacheEntry[string]](),
		accessLog: make([]cleanupCandidate[int], 0, 2),
	}

	first := &CacheEntry[string]{}
	first.cached.Store(&cachedValue[string]{result: strings.Repeat("y", 120), err: errors.New("evicted")})
	first.accessSeq.Store(1)
	first.queuedSeq.Store(1)
	state.entries.Store(1, first)
	state.accessLog = append(state.accessLog, cleanupCandidate[int]{key: 1, seq: 1})

	second := &CacheEntry[string]{}
	second.cached.Store(&cachedValue[string]{result: "survivor"})
	second.accessSeq.Store(2)
	second.queuedSeq.Store(2)
	state.entries.Store(2, second)
	state.accessLog = append(state.accessLog, cleanupCandidate[int]{key: 2, seq: 2})

	state.Cleanup()

	entries := decodeLogLines(t, buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "cache: overflow eviction", entries[0]["message"])
	assert.EqualValues(t, 1, entries[0]["max_entries"])
	assert.EqualValues(t, 1, entries[0]["size_after"])
	assert.EqualValues(t, 1, entries[0]["key"])
	resultSummary, ok := entries[0]["result"].(string)
	require.True(t, ok)
	assert.Contains(t, resultSummary, "...(20 more bytes)")
	assert.Equal(t, "evicted", entries[0]["error"])
}

func TestCacheDebugLogPayloads_KeyedEvictionWithoutCachedSnapshotStillLogs(t *testing.T) {
	buf := withCapturedDebugLogger(t)

	state := &CachedContextKeyFuncState[string, int]{
		CachedKeyFuncBuilder: CachedKeyFuncBuilder[string, int]{
			maxEntries: 1,
		},
		entries:   xsync.NewMap[int, *CacheEntry[string]](),
		accessLog: make([]cleanupCandidate[int], 0, 2),
	}

	first := &CacheEntry[string]{}
	first.accessSeq.Store(1)
	first.queuedSeq.Store(1)
	state.entries.Store(1, first)
	state.accessLog = append(state.accessLog, cleanupCandidate[int]{key: 1, seq: 1})

	second := &CacheEntry[string]{}
	second.cached.Store(&cachedValue[string]{result: "survivor"})
	second.accessSeq.Store(2)
	second.queuedSeq.Store(2)
	state.entries.Store(2, second)
	state.accessLog = append(state.accessLog, cleanupCandidate[int]{key: 2, seq: 2})

	state.Cleanup()

	entries := decodeLogLines(t, buf)
	require.Len(t, entries, 1)
	assert.Equal(t, "cache: overflow eviction", entries[0]["message"])
	assert.EqualValues(t, 1, entries[0]["max_entries"])
	assert.EqualValues(t, 1, entries[0]["size_after"])
	assert.EqualValues(t, 1, entries[0]["key"])
	_, hasResult := entries[0]["result"]
	assert.True(t, hasResult)
	assert.Nil(t, entries[0]["result"])
}
