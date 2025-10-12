package httputils

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"testing"
)

func BenchmarkReadAllBody(b *testing.B) {
	small := make([]byte, 1024)         // 1 KB
	small4K := make([]byte, 4*1024)     // 4 KB
	small16K := make([]byte, 16*1024)   // 16 KB
	small64K := make([]byte, 64*1024)   // 64 KB
	small256K := make([]byte, 256*1024) // 256 KB
	medium := make([]byte, 1024*1024)   // 1 MB
	large := make([]byte, 4*1024*1024)  // 4 MB

	io.CopyN(bytes.NewBuffer(small), rand.Reader, int64(len(small)))
	io.CopyN(bytes.NewBuffer(small4K), rand.Reader, int64(len(small4K)))
	io.CopyN(bytes.NewBuffer(medium), rand.Reader, int64(len(medium)))
	io.CopyN(bytes.NewBuffer(large), rand.Reader, int64(len(large)))

	newReader := func(b []byte) io.ReadCloser {
		return io.NopCloser(bytes.NewReader(b))
	}

	b.ResetTimer()

	benchmarks := []struct {
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

	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			var resp http.Response
			for b.Loop() {
				resp.Body = newReader(benchmark.body)
				buf, release, err := ReadAllBody(&resp)
				if err != nil {
					b.Fatal(err)
				}
				release(buf)
			}
		})
		b.Run(benchmark.name+"_with_content_length", func(b *testing.B) {
			var resp http.Response
			for b.Loop() {
				resp.Body = newReader(benchmark.body)
				resp.ContentLength = int64(len(benchmark.body))
				buf, release, err := ReadAllBody(&resp)
				if err != nil {
					b.Fatal(err)
				}
				release(buf)
			}
		})
		b.Run(benchmark.name+"_std", func(b *testing.B) {
			var resp http.Response
			for b.Loop() {
				resp.Body = newReader(benchmark.body)
				_, err := io.ReadAll(resp.Body)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
