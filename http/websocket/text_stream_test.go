package websocket

import (
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	gorilla "github.com/gorilla/websocket"
)

func TestManagerCopyTextLinesFramesSplitLines(t *testing.T) {
	conn, result := openCopiedStream(
		t,
		iotest.OneByteReader(strings.NewReader("first line\nsecond line\nlast line")),
		(*Manager).CopyTextLines,
	)

	for _, want := range []string{"first line\n", "second line\n", "last line"} {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("reading WebSocket message: %v", err)
		}
		if messageType != gorilla.TextMessage {
			t.Fatalf("message type = %d, want %d", messageType, gorilla.TextMessage)
		}
		if got := string(data); got != want {
			t.Fatalf("message = %q, want %q", got, want)
		}
	}

	if err := awaitStreamResult(t, result); err != nil {
		t.Fatalf("CopyTextLines() error = %v", err)
	}
}

func TestManagerCopyTextLinesStreamsLongRecordAsOneMessage(t *testing.T) {
	want := strings.Repeat("x", 128*1024) + "\n"
	conn, result := openCopiedStream(t, strings.NewReader(want), (*Manager).CopyTextLines)

	messageType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading WebSocket message: %v", err)
	}
	if messageType != gorilla.TextMessage {
		t.Fatalf("message type = %d, want %d", messageType, gorilla.TextMessage)
	}
	if got := string(data); got != want {
		t.Fatalf("message length = %d, want %d", len(got), len(want))
	}

	if err := awaitStreamResult(t, result); err != nil {
		t.Fatalf("CopyTextLines() error = %v", err)
	}
}

func TestManagerCopyTextLinesKeepsHeartbeatResponsiveWhileLongRecordIsIncomplete(t *testing.T) {
	input := newGatedLongLineReader(strings.Repeat("x", 4096), "tail\n")
	conn, result := openCopiedStream(t, input, (*Manager).CopyTextLines)
	defer input.unblock()

	select {
	case <-input.waiting:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for incomplete record")
	}

	if err := conn.WriteMessage(gorilla.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("writing heartbeat: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("setting heartbeat read deadline: %v", err)
	}
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading heartbeat reply: %v", err)
	}
	if got, want := string(data), "pong"; got != want {
		t.Fatalf("heartbeat reply = %q, want %q", got, want)
	}

	input.unblock()
	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading completed record: %v", err)
	}
	if got, want := string(data), strings.Repeat("x", 4096)+"tail\n"; got != want {
		t.Fatalf("message length = %d, want %d", len(got), len(want))
	}

	if err := awaitStreamResult(t, result); err != nil {
		t.Fatalf("CopyTextLines() error = %v", err)
	}
}

func TestManagerCopyTextLinesEnforcesSharedBufferBudget(t *testing.T) {
	const fragmentSize int64 = 4096
	preReserved := maxBufferedTextBytes - 2*fragmentSize
	if !reserveBufferedTextBytes(preReserved) {
		t.Fatal("reserving shared buffer capacity for test")
	}
	defer func() {
		if preReserved > 0 {
			bufferedTextBytes.Add(-preReserved)
		}
	}()

	firstInput := newGatedLongLineReader(strings.Repeat("a", int(fragmentSize)), "\n")
	firstConn, firstResult := openCopiedStream(t, firstInput, (*Manager).CopyTextLines)
	defer firstInput.unblock()
	select {
	case <-firstInput.waiting:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first active buffer")
	}

	secondConn, secondResult := openCopiedStream(
		t,
		strings.NewReader(strings.Repeat("b", int(fragmentSize)+1)),
		(*Manager).CopyTextLines,
	)
	if err := awaitStreamResult(t, secondResult); !errors.Is(err, ErrTextBufferBudgetExceeded) {
		t.Fatalf("CopyTextLines() error = %v, want %v", err, ErrTextBufferBudgetExceeded)
	}
	if err := secondConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("setting rejected stream read deadline: %v", err)
	}
	if _, data, err := secondConn.ReadMessage(); err == nil || len(data) != 0 {
		t.Fatalf("incomplete record read = %q, %v; want no message", data, err)
	}

	firstInput.unblock()
	_, data, err := firstConn.ReadMessage()
	if err != nil {
		t.Fatalf("reading first completed record: %v", err)
	}
	if got, want := string(data), strings.Repeat("a", int(fragmentSize))+"\n"; got != want {
		t.Fatalf("message length = %d, want %d", len(got), len(want))
	}
	if err := awaitStreamResult(t, firstResult); err != nil {
		t.Fatalf("first CopyTextLines() error = %v", err)
	}

	bufferedTextBytes.Add(-preReserved)
	preReserved = 0
	if !reserveBufferedTextBytes(maxBufferedTextBytes) {
		t.Fatal("shared buffer capacity was not restored")
	}
	bufferedTextBytes.Add(-maxBufferedTextBytes)
}

func TestManagerCopyTextLinesReturnsReadError(t *testing.T) {
	readErr := errors.New("read failed")
	conn, result := openCopiedStream(
		t,
		io.MultiReader(strings.NewReader("complete line\n"), iotest.ErrReader(readErr)),
		(*Manager).CopyTextLines,
	)

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading first WebSocket message: %v", err)
	}
	if got, want := string(data), "complete line\n"; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}

	if err := awaitStreamResult(t, result); !errors.Is(err, readErr) {
		t.Fatalf("CopyTextLines() error = %v, want %v", err, readErr)
	}
}

type gatedLongLineReader struct {
	first       string
	rest        string
	waiting     chan struct{}
	release     chan struct{}
	waitOnce    sync.Once
	releaseOnce sync.Once
	state       int
}

func newGatedLongLineReader(first, rest string) *gatedLongLineReader {
	return &gatedLongLineReader{
		first:   first,
		rest:    rest,
		waiting: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (r *gatedLongLineReader) Read(p []byte) (int, error) {
	switch r.state {
	case 0:
		r.state++
		n := copy(p, r.first)
		r.first = r.first[n:]
		return n, nil
	case 1:
		r.waitOnce.Do(func() { close(r.waiting) })
		<-r.release
		r.state++
		n := copy(p, r.rest)
		r.rest = r.rest[n:]
		return n, nil
	default:
		return 0, io.EOF
	}
}

func (r *gatedLongLineReader) unblock() {
	r.releaseOnce.Do(func() { close(r.release) })
}
