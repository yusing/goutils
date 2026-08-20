package websocket

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
)

func TestManagerCopyJSONStreamFramesValues(t *testing.T) {
	conn, result := openCopiedStream(t, iotest.OneByteReader(strings.NewReader(
		`{"id":1}{"id":2}`,
	)), (*Manager).CopyJSONStream)

	for _, want := range []string{`{"id":1}`, `{"id":2}`} {
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
		t.Fatalf("CopyJSONStream() error = %v", err)
	}
}

func TestManagerCopyJSONStreamRejectsTruncatedValue(t *testing.T) {
	conn, result := openCopiedStream(
		t,
		strings.NewReader(`{"id":1}{"id":`),
		(*Manager).CopyJSONStream,
	)

	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("reading first WebSocket message: %v", err)
	}
	if got, want := string(data), `{"id":1}`; got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}

	if err := awaitStreamResult(t, result); err == nil {
		t.Fatal("CopyJSONStream() error = nil, want truncated JSON error")
	}
}

func openCopiedStream(
	t *testing.T,
	input io.Reader,
	copyStream func(*Manager, io.Reader) error,
) (*gorilla.Conn, <-chan error) {
	t.Helper()

	result := make(chan error, 1)
	router := gin.New()
	router.GET("/stream", func(c *gin.Context) {
		manager, err := NewManagerWithUpgrade(c)
		if err == nil {
			defer manager.Close()
			err = copyStream(manager, input)
		}
		result <- err
	})

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/stream"
	conn, _, err := gorilla.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("connecting WebSocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
	})

	return conn, result
}

func awaitStreamResult(t *testing.T, result <-chan error) error {
	t.Helper()

	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for stream copy")
		return nil
	}
}
