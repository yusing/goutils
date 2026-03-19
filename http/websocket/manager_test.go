package websocket

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFWebSocketSubprotocol(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "chat, csrf.token.value")

	if got := csrfWebSocketSubprotocol(req); got != "csrf.token.value" {
		t.Fatalf("csrfWebSocketSubprotocol() = %q, want %q", got, "csrf.token.value")
	}
}

func TestWebsocketUpgradeResponseHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "csrf.token.value")

	header := websocketUpgradeResponseHeader(req)
	if header == nil {
		t.Fatal("websocketUpgradeResponseHeader() = nil, want header")
	}
	values := header["Sec-WebSocket-Protocol"]
	if len(values) != 1 || values[0] != "csrf.token.value" {
		t.Fatalf("Sec-WebSocket-Protocol = %v, want [%q]", values, "csrf.token.value")
	}
}

func TestWebsocketUpgradeResponseHeaderWithoutCSRFSubprotocol(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/ws", nil)
	req.Header.Set("Sec-WebSocket-Protocol", "chat")

	if header := websocketUpgradeResponseHeader(req); header != nil {
		t.Fatalf("websocketUpgradeResponseHeader() = %v, want nil", header)
	}
}
