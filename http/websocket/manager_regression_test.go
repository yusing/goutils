package websocket

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorilla "github.com/gorilla/websocket"
)

func regressionManager(t *testing.T, frames ...[]byte) *Manager {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	m := &Manager{ctx: ctx, cancel: cancel, readCh: make(chan []byte, len(frames))}
	for _, frame := range frames {
		m.readCh <- frame
	}
	return m
}

func TestReaderPreservesRemainderRegression(t *testing.T) {
	m := regressionManager(t, []byte{}, []byte("abcdef"), []byte("ghi"))
	r := m.NewReader()
	var got strings.Builder
	for got.Len() < 9 {
		p := make([]byte, 2)
		n, err := r.Read(p)
		if err != nil {
			t.Fatal(err)
		}
		if n <= 0 || n > len(p) {
			t.Fatalf("Read = %d, want 1..%d", n, len(p))
		}
		got.Write(p[:n])
	}
	if got.String() != "abcdefghi" {
		t.Fatalf("read %q", got.String())
	}
	m.cancel()
	if n, err := r.Read(make([]byte, 2)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("closed Read = %d, %v, want 0, EOF", n, err)
	}
}

func TestReaderZeroLengthDoesNotConsumeRegression(t *testing.T) {
	m := regressionManager(t, []byte("a"))
	r := m.NewReader()
	done := make(chan error, 1)
	go func() {
		n, err := r.Read(nil)
		if n != 0 {
			err = errors.New("nonzero count")
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		m.cancel()
		t.Fatal("zero length Read blocked")
	}
	p := make([]byte, 1)
	if n, err := r.Read(p); n != 1 || err != nil || p[0] != 'a' {
		t.Fatalf("next Read = %d, %v, %q", n, err, p)
	}
}

func TestManagerClosedOperationsRegression(t *testing.T) {
	for _, recorded := range []error{nil, errors.New("recorded failure")} {
		m := regressionManager(t)
		if recorded != nil {
			m.setErrIfNil(recorded)
		}
		m.cancel()
		want := recorded
		if want == nil {
			want = net.ErrClosed
		}
		var out any
		if err := m.ReadJSON(&out, time.Second); !errors.Is(err, want) {
			t.Errorf("ReadJSON = %v, want %v", err, want)
		}
		if _, err := m.ReadBinary(time.Second); !errors.Is(err, want) {
			t.Errorf("ReadBinary = %v, want %v", err, want)
		}
		if err := m.WriteData(BinaryMessage, nil, time.Second); !errors.Is(err, want) {
			t.Errorf("WriteData = %v, want %v", err, want)
		}
	}
}

// Use actual gorilla writes, rather than a synthetic timeout error.
func regressionConnectedManager(t *testing.T) (*Manager, *gorilla.Conn) {
	t.Helper()
	managers := make(chan *Manager, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&gorilla.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		ctx, cancel := context.WithCancel(t.Context())
		managers <- &Manager{conn: conn, ctx: ctx, cancel: cancel, readCh: make(chan []byte), pingCheckTicker: time.NewTicker(time.Hour)}
	}))
	t.Cleanup(srv.Close)
	client, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	select {
	case m := <-managers:
		t.Cleanup(m.Close)
		return m, client
	case <-time.After(time.Second):
		t.Fatal("upgrade timed out")
		return nil, nil
	}
}

func TestGorillaWriteDeadlineRegression(t *testing.T) {
	m, _ := regressionConnectedManager(t)
	if err := m.WriteData(BinaryMessage, []byte("payload"), -time.Second); !errors.Is(err, ErrWriteTimeout) {
		t.Fatalf("WriteData = %v, want ErrWriteTimeout", err)
	}
}

func TestPeriodicWriteDeduplicateOrderRegression(t *testing.T) {
	m, client := regressionConnectedManager(t)
	result := make(chan error, 1)
	calls := 0
	pairs := make(chan [2]int, 1)
	go func() {
		result <- m.PeriodicWrite(20*time.Millisecond, func() (any, error) { calls++; return calls, nil }, func(last, current any) bool {
			pairs <- [2]int{last.(int), current.(int)}
			m.cancel()
			return last.(int) < current.(int)
		})
	}()
	_ = client.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := client.ReadMessage(); err != nil {
		t.Fatal(err)
	}
	select {
	case pair := <-pairs:
		if pair != [2]int{1, 2} {
			t.Errorf("dedup arguments = %v, want [1 2]", pair)
		}
	case <-time.After(time.Second):
		t.Fatal("dedup not called")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("clean close PeriodicWrite = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("PeriodicWrite did not stop")
	}
}

func TestDefaultOriginPolicyRegression(t *testing.T) {
	for _, tc := range []struct {
		name, host, origin string
		want               bool
	}{
		{"localhost cross origin", "localhost:8080", "https://evil.example", false},
		{"IPv4 loopback cross origin", "127.0.0.1:8080", "https://evil.example", false},
		{"localhost same host", "localhost:8080", "http://localhost:9000", true},
		{"IPv4 same host", "127.0.0.1:8080", "http://127.0.0.1:9000", true},
		{"case insensitive", "EXAMPLE.COM:8080", "https://example.com", true},
		{"IPv6 same host", "[::1]:8080", "http://[::1]:9000", true},
		{"IPv6 other host", "[::1]:8080", "http://[::2]:9000", false},
		{"malformed origin", "example.com", "http://[", false},
		{"relative origin", "example.com", "/relative", false},
		{"missing origin", "example.com", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "http://"+tc.host+"/", nil)
			r.Header.Set("Origin", tc.origin)
			if got := defaultUpgrader.CheckOrigin(r); got != tc.want {
				t.Fatalf("CheckOrigin = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCustomOriginUpgraderUnaffectedRegression(t *testing.T) {
	managers := make(chan *Manager, 1)
	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		c.Set("upgrader", &gorilla.Upgrader{CheckOrigin: func(*http.Request) bool { return true }})
		m, err := NewManagerWithUpgrade(c)
		if err != nil {
			return
		}
		managers <- m
		defer m.Close()
		<-m.Done()
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	client, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", http.Header{"Origin": {"https://other.example"}})
	if err != nil {
		t.Fatalf("custom upgrader rejected cross-origin handshake: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	select {
	case m := <-managers:
		t.Cleanup(m.Close)
	case <-time.After(time.Second):
		t.Fatal("custom upgrader did not return a manager")
	}
}

func TestReaderPreservesRecordedErrorRegression(t *testing.T) {
	m := regressionManager(t)
	recorded := errors.New("read failure")
	m.setErrIfNil(recorded)
	m.cancel()
	if n, err := m.NewReader().Read(make([]byte, 1)); n != 0 || !errors.Is(err, recorded) {
		t.Fatalf("Read = %d, %v, want recorded error", n, err)
	}
}

func TestDefaultOriginRejectsHandshakeRegression(t *testing.T) {
	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		if m, err := NewManagerWithUpgrade(c); err == nil {
			m.Close()
		}
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	for _, host := range []string{"127.0.0.1", "localhost"} {
		t.Run(host, func(t *testing.T) {
			_, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			conn, resp, err := gorilla.DefaultDialer.Dial("ws://"+net.JoinHostPort(host, port)+"/ws", http.Header{"Origin": {"https://other.example"}})
			if conn != nil {
				_ = conn.Close()
			}
			if resp != nil {
				defer resp.Body.Close()
			}
			if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
				t.Fatalf("cross-host handshake: response=%v, error=%v", resp, err)
			}
		})
	}
}

func TestReaderProductionFramesAndCleanCloseRegression(t *testing.T) {
	managers := make(chan *Manager, 1)
	router := gin.New()
	router.GET("/ws", func(c *gin.Context) {
		m, err := NewManagerWithUpgrade(c)
		if err != nil {
			return
		}
		managers <- m
		defer m.Close()
		<-m.Done()
	})
	srv := httptest.NewServer(router)
	t.Cleanup(srv.Close)
	client, _, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	var m *Manager
	select {
	case m = <-managers:
		t.Cleanup(m.Close)
	case <-time.After(time.Second):
		t.Fatal("manager upgrade timed out")
	}
	for _, payload := range []string{"", "abcdef", "ghi"} {
		if err := client.WriteMessage(gorilla.BinaryMessage, []byte(payload)); err != nil {
			t.Fatal(err)
		}
	}
	r := m.NewReader()
	var got strings.Builder
	for got.Len() < 9 {
		var buf [2]byte
		n, err := r.Read(buf[:])
		if err != nil || n <= 0 || n > len(buf) {
			t.Fatalf("Read=%d, %v", n, err)
		}
		got.Write(buf[:n])
	}
	if got.String() != "abcdefghi" {
		t.Fatalf("received %q", got.String())
	}
	if err := client.WriteControl(gorilla.CloseMessage, gorilla.FormatCloseMessage(gorilla.CloseNormalClosure, ""), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if n, err := r.Read(make([]byte, 2)); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("closed Read=%d, %v", n, err)
	}
}
