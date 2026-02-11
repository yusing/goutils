package httputils

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	mathrand "math/rand/v2"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/yusing/goutils/synk/workerpool"
)

/*
BenchmarkReadAllBody
BenchmarkReadAllBody/unknown_size
BenchmarkReadAllBody/unknown_size-10                3535            835974 ns/op           59990 B/op          8 allocs/op
BenchmarkReadAllBody/known_size
BenchmarkReadAllBody/known_size-10                  7149            764773 ns/op           44278 B/op          8 allocs/op
BenchmarkReadAllBody/mixed
BenchmarkReadAllBody/mixed-10                       4302            801842 ns/op           85021 B/op          8 allocs/op
BenchmarkReadAllBody/stdlib
BenchmarkReadAllBody/stdlib-10                      1606           1199845 ns/op          292991 B/op         17 allocs/op
*/

var (
	small     = make([]byte, 1024)        // 1 KB
	small4K   = make([]byte, 4*1024)      // 4 KB
	small16K  = make([]byte, 16*1024)     // 16 KB
	small64K  = make([]byte, 64*1024)     // 64 KB
	small256K = make([]byte, 256*1024)    // 256 KB
	medium    = make([]byte, 1024*1024)   // 1 MB
	large     = make([]byte, 4*1024*1024) // 4 MB

	benchmarks = []struct {
		name string
		body []byte
	}{
		{"small", small},
		{"small4K", small4K},
		{"small16K", small16K},
		{"small64K", small64K},
		{"small256K", small256K},
		{"medium", medium},
		{"large", large},
	}

	choices [10000]int
)

func TestMain(m *testing.M) {
	// Fill slices in-place.
	_, _ = rand.Read(small)
	_, _ = rand.Read(small4K)
	_, _ = rand.Read(small16K)
	_, _ = rand.Read(small64K)
	_, _ = rand.Read(small256K)
	_, _ = rand.Read(medium)
	_, _ = rand.Read(large)

	// Skewed, heavy-tail-ish distribution: mostly small responses, rare large ones.
	// (Weights roughly approximate typical web/proxy traffic.)
	weights := [...]int{
		30, // 1KB
		30, // 4KB
		15, // 16KB
		10, // 64KB
		8,  // 256KB
		6,  // 1MB
		1,  // 4MB
	}
	totalWeight := 0
	for _, w := range weights {
		totalWeight += w
	}

	for i := range choices {
		r := mathrand.IntN(totalWeight)
		acc := 0
		for j, w := range weights {
			acc += w
			if r < acc {
				choices[i] = j
				break
			}
		}
	}

	os.Exit(m.Run())
}

func newReader(i int) io.ReadCloser {
	b := benchmarks[choices[i%len(choices)]].body
	return io.NopCloser(bytes.NewReader(b))
}

type chunkedReadCloser struct {
	r        io.Reader
	rng      *mathrand.Rand
	maxChunk int
	maxDelay time.Duration
}

func (r *chunkedReadCloser) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	x := r.rng.Int64()
	limit := 1 + int(x%int64(r.maxChunk))
	if limit < len(p) {
		p = p[:limit]
	}

	if r.maxDelay > 0 {
		// Deterministic jitter to simulate TCP pacing / scheduling delay.
		// Keep it small so the benchmark still finishes in reasonable time.
		d := time.Duration(x % int64(r.maxDelay+1))
		time.Sleep(d)
	}
	return r.r.Read(p)
}

func (*chunkedReadCloser) Close() error { return nil }

func newChunkedReader(i int, maxChunk int, maxDelay time.Duration) io.ReadCloser {
	b := benchmarks[choices[i%len(choices)]].body
	seed := uint64(i) + 1
	return &chunkedReadCloser{
		r:        bytes.NewReader(b),
		rng:      mathrand.New(mathrand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		maxChunk: max(1, maxChunk),
		maxDelay: maxDelay,
	}
}

func sizeOf(i int) int {
	return len(benchmarks[choices[i%len(choices)]].body)
}

func BenchmarkReadAllBody(b *testing.B) {
	b.Run("unknown_size", func(b *testing.B) {
		pool := workerpool.New(b.Context())
		for b.Loop() {
			pool.Go(func(_ context.Context, i int) {
				// Unknown length is common in real traffic (chunked, compressed, streamed).
				resp := http.Response{
					Body:          newChunkedReader(i, 32*1024, 200*time.Microsecond),
					ContentLength: -1,
				}
				buf, release, err := ReadAllBody(&resp)
				if err != nil {
					b.Fatal(err)
				}
				release(buf)
			})
		}
		pool.Wait()
	})
	b.Run("known_size", func(b *testing.B) {
		pool := workerpool.New(b.Context())
		for b.Loop() {
			pool.Go(func(_ context.Context, i int) {
				resp := http.Response{
					Body:          newChunkedReader(i, 32*1024, 200*time.Microsecond),
					ContentLength: int64(sizeOf(i)),
				}
				buf, release, err := ReadAllBody(&resp)
				if err != nil {
					b.Fatal(err)
				}
				release(buf)
			})
		}
		pool.Wait()
	})
	b.Run("mixed", func(b *testing.B) {
		// 33% known size, 66% unknown size
		pool := workerpool.New(b.Context())
		for b.Loop() {
			pool.Go(func(_ context.Context, i int) {
				ctxLen := -1
				if i%3 == 1 {
					ctxLen = sizeOf(i)
				}
				resp := http.Response{
					Body:          newChunkedReader(i, 32*1024, 200*time.Microsecond),
					ContentLength: int64(ctxLen),
				}
				buf, release, err := ReadAllBody(&resp)
				if err != nil {
					b.Fatal(err)
				}
				release(buf)
			})
		}
		pool.Wait()
	})
	b.Run("stdlib", func(b *testing.B) {
		pool := workerpool.New(b.Context())
		for b.Loop() {
			pool.Go(func(_ context.Context, i int) {
				resp := http.Response{Body: newChunkedReader(i, 32*1024, 200*time.Microsecond)}
				_, err := io.ReadAll(resp.Body)
				if err != nil {
					b.Fatal(err)
				}
			})
		}
		pool.Wait()
	})
}
