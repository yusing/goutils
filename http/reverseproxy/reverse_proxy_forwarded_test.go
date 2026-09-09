package reverseproxy

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestForwardedFor(t *testing.T) {
	for _, tc := range []struct {
		name, remote string
		prior        []string
		setPrior     bool
		want         string
		present      bool
	}{
		{name: "internal"},
		{name: "internal empty middleware", prior: []string{""}, setPrior: true},
		{name: "internal existing chain", prior: []string{"192.0.2.1", "192.0.2.2"}, setPrior: true, want: "192.0.2.1, 192.0.2.2", present: true},
		{name: "client empty middleware", remote: "192.0.2.3:1234", prior: []string{""}, setPrior: true, want: "192.0.2.3", present: true},
		{name: "client", remote: "192.0.2.3:1234", want: "192.0.2.3", present: true},
		{name: "client chain", remote: "192.0.2.3:1234", prior: []string{"192.0.2.1"}, setPrior: true, want: "192.0.2.1, 192.0.2.3", present: true},
		{name: "ipv6", remote: "[2001:db8::1]:1234", want: "2001:db8::1", present: true},
		{name: "bare address", remote: "192.0.2.3", want: "192.0.2.3", present: true},
		{name: "explicit suppression", remote: "192.0.2.3:1234", setPrior: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := make(chan http.Header, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got <- r.Header.Clone()
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()
			target, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.RemoteAddr = tc.remote
			if tc.setPrior {
				req.Header["X-Forwarded-For"] = tc.prior
			}
			recorder := httptest.NewRecorder()
			NewReverseProxy("test", target, server.Client().Transport).ServeHTTP(recorder, req)
			if recorder.Code != http.StatusNoContent {
				t.Fatalf("status = %d", recorder.Code)
			}
			headers := <-got
			_, present := headers["X-Forwarded-For"]
			if present != tc.present || headers.Get("X-Forwarded-For") != tc.want {
				t.Fatalf("X-Forwarded-For = %q (present %t), want %q (present %t)", headers.Get("X-Forwarded-For"), present, tc.want, tc.present)
			}
		})
	}
}
