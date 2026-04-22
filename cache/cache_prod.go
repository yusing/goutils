//go:build !debug

package cache

func logCacheExpiredEntry(key any, result any, err error) {}

func logCacheEvicted(maxEntries int, sizeAfter int, evicted cacheEvictedKV) {}

func logCacheHit(key any, result any, err error) {}

func logCacheMiss(key any) {}

func logCacheUsage(size int, maxEntries int) {}

func formatResult(result any) any { return result }
