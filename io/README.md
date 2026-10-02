# goutils/io

Building blocks for proxies and streaming servers: a copy loop that flushes HTTP
responses and stops on context cancellation, a pooled buffered writer,
context-aware readers and writers, byte pipes between two connections, and a read
closer that runs a hook when closed.

The import path ends in `io`, but the package name is `ioutils`:

```go
import ioutils "github.com/yusing/goutils/io"
```

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

The module needs Go 1.27 or newer. Buffers come from the `goutils/synk` pool.

## Quick start

Copy an upstream body to an HTTP response, flushing as data arrives:

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"

	ioutils "github.com/yusing/goutils/io"
)

func main() {
	rec := httptest.NewRecorder() // stands in for the http.ResponseWriter
	body := io.NopCloser(strings.NewReader("hello"))
	defer body.Close() // you close the source; see below

	err := ioutils.CopyCloseWithContext(context.Background(), rec, body, 0)
	fmt.Println(rec.Body.String(), err, rec.Flushed)
}
```

Output:

```text
hello <nil> true
```

## Copying

```go
func CopyClose(dst io.Writer, src io.Reader, sizeHint int) error
func CopyCloseWithContext(ctx context.Context, dst io.Writer, src io.Reader, sizeHint int) error
```

The destination comes first, as in `io.Copy`. Both return `nil` when the source
reaches EOF and otherwise the first read or write error. Unlike `io.Copy`, they do not
return the byte count and do not use `WriterTo`/`ReaderFrom` fast paths.

**Closing.** Despite the name, `CopyClose` never closes anything. `CopyCloseWithContext`
closes the source and then the destination, whichever implement `io.Closer`, only when
`ctx` is cancelled while the copy is running. Neither closes after a normal EOF or an
error, so close both yourself when the function returns. After a cancellation, the
returned error comes from the closed reader or writer (for example `io: read/write on
closed pipe`), not from `ctx.Err()`. If neither side implements `io.Closer`, cancellation
does not interrupt the copy; wrap the endpoints with `NewContextReader` or
`NewContextWriter` to stop between reads.

**Buffer size.** `sizeHint <= 0` selects 32 KiB, hints above 32 KiB are capped at 32 KiB,
and when `src` is an `*io.LimitedReader` the buffer is sized to its remaining bytes (up to
32 KiB) and the hint is ignored.

**HTTP flushing.** When `dst` is an `http.ResponseWriter` (or unwraps to one through an
`Unwrap() http.ResponseWriter` method) that supports flushing, the copy flushes after
every write unless the response has a `Content-Length` header. Responses with a
`text/event-stream` or `application/grpc*` content type are flushed even with a
`Content-Length`. The headers are inspected once when the copy starts, so set them first.
Flush errors that mean "not supported" are ignored; any other flush error ends the copy.

## Buffered writer

`BufferedWriter` is a fork of `bufio.Writer` whose buffer comes from a pool.

```go
package main

import (
	"bytes"
	"fmt"

	ioutils "github.com/yusing/goutils/io"
)

func main() {
	var out bytes.Buffer
	w := ioutils.NewBufferedWriter(&out, 16)

	w.WriteString("hello")
	fmt.Println(out.Len(), w.Buffered())

	if err := w.Flush(); err != nil {
		panic(err)
	}
	fmt.Println(out.String())

	if err := w.Close(); err != nil {
		panic(err)
	}
}
```

Output:

```text
0 5
hello
```

- `NewBufferedWriter(w, size)` uses 4096 bytes when `size <= 0`. If `w` is already a
  `*BufferedWriter` with a buffer at least `size` bytes, that writer is returned.
- Data reaches the destination only on `Flush`, `Close`, or when the buffer fills. A
  failed write is sticky: later writes and flushes return that error.
- `Resize(size)` flushes, then changes the buffer size.
- `Close` flushes, returns the buffer to the pool, and closes the destination if it
  implements `io.Closer`, joining the flush and close errors. Afterwards writes and a
  second `Close` return `io.ErrClosedPipe`, and `Size()` is 0.
- `AvailableBuffer`, `Available`, `Buffered`, `WriteByte`, `WriteRune`, and `WriteString`
  behave as in `bufio.Writer`. A `BufferedWriter` is not safe for concurrent use.

## Context-aware reader and writer

`NewContextReader(ctx, r)` and `NewContextWriter(ctx, w)` return wrappers that check
`ctx` before every `Read` or `Write` and return `ctx.Err()` once it is done. They do not
interrupt a call that is already blocked. `Close` closes the wrapped value when it
implements `io.Closer`, and otherwise does nothing.

## Pipes

`NewPipe(ctx, r, w)` copies `r` to `w` when you call `Start()`, blocking until EOF, an
error, or cancellation of `ctx` (which closes both ends). `Start` returns `nil` for
cancellation and for closed-connection errors (`EPIPE`, `ECONNRESET`,
`io.ErrClosedPipe`, `net.ErrClosed`, `context.Canceled`), and any other error as is.

`NewBidirectionalPipe(ctx, a, b)` runs both directions at once and returns when both have
finished, with their errors joined. Reaching EOF in one direction does not close or
half-close anything, so the call keeps running until the other direction also ends or
`ctx` is cancelled.

```go
package main

import (
	"context"
	"fmt"
	"io"
	"net"

	ioutils "github.com/yusing/goutils/io"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())

	client, proxyFront := net.Pipe()
	proxyBack, backend := net.Pipe()

	done := make(chan error, 1)
	go func() { done <- ioutils.NewBidirectionalPipe(ctx, proxyFront, proxyBack).Start() }()

	go client.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(backend, buf); err != nil {
		panic(err)
	}
	fmt.Println(string(buf))

	cancel()
	fmt.Println(<-done)
}
```

Output:

```text
ping
<nil>
```

## Hook on close

`NewHookReadCloser(rc, hook)` wraps a `io.ReadCloser`. `Close` closes the wrapped reader
first and then always calls `hook`, even when the close failed. The hook runs on every
`Close` call, not once, and must not be `nil`. It suits releasing a semaphore or finishing
a metric when a response body is closed.

```go
package main

import (
	"fmt"
	"io"
	"strings"

	ioutils "github.com/yusing/goutils/io"
)

func main() {
	body := ioutils.NewHookReadCloser(io.NopCloser(strings.NewReader("data")), func() {
		fmt.Println("body closed")
	})
	io.Copy(io.Discard, body)
	body.Close()
}
```

Output:

```text
body closed
```

## When to use it

Use `CopyClose*` and the pipes for reverse proxies, SSE, gRPC streaming, and tunnels,
where flushing and cancellation matter. For ordinary file or buffer copies, `io.Copy`
is simpler and can use zero-copy paths that these functions skip.

## API reference

```sh
go doc -all github.com/yusing/goutils/io
```
