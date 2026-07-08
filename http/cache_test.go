package httputils

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCacheUpdateCookiesUpdatesCookiesMap(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)
	req.AddCookie(&http.Cookie{Name: "old", Value: "stale"})

	cache := NewCache()
	defer cache.Release()

	_ = cache.GetCookiesMap(req)

	cache.UpdateCookies(req, func(_ []*http.Cookie) []*http.Cookie {
		return []*http.Cookie{
			{Name: "session", Value: "alpha"},
			{Name: "session", Value: "beta"},
			{Name: "id", Value: "42"},
			{Name: "semicolon-separated", Value: "alpha; beta"},
		}
	})

	got := cache.GetCookiesMap(req)
	want := url.Values{
		"session":             {"alpha", "beta"},
		"id":                  {"42"},
		"semicolon-separated": {"alpha", "beta"},
	}

	require.Equal(t, got, want)
}

func TestJoinCookieValues(t *testing.T) {
	tests := []struct {
		name      string
		existing  []string
		newCookie string
		want      []string
	}{
		{
			name:      "single value",
			newCookie: "alpha",
			want:      []string{"alpha"},
		},
		{
			name:      "appends single value",
			existing:  []string{"alpha"},
			newCookie: "beta",
			want:      []string{"alpha", "beta"},
		},
		{
			name:      "splits semicolon values",
			newCookie: "alpha; beta;gamma",
			want:      []string{"alpha", "beta", "gamma"},
		},
		{
			name:      "appends split semicolon values",
			existing:  []string{"alpha"},
			newCookie: "beta; gamma",
			want:      []string{"alpha", "beta", "gamma"},
		},
		{
			name:      "preserves empty semicolon parts",
			newCookie: "alpha; ;",
			want:      []string{"alpha", "", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinCookieValues(tt.existing, tt.newCookie)
			require.Equal(t, tt.want, got)
		})
	}
}

func BenchmarkJoinCookieValues(b *testing.B) {
	benchmarks := []struct {
		name      string
		existing  []string
		newCookie string
	}{
		{
			name:      "single",
			newCookie: "session-token",
		},
		{
			name:      "append-single",
			existing:  []string{"alpha"},
			newCookie: "beta",
		},
		{
			name:      "semicolon-three",
			newCookie: "alpha; beta;gamma",
		},
		{
			name:      "semicolon-empty",
			newCookie: "alpha; ;",
		},
		{
			name:      "append-split-spare-capacity",
			existing:  append(make([]string, 0, 4), "alpha"),
			newCookie: "beta; gamma",
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				_ = joinCookieValues(bm.existing, bm.newCookie)
			}
		})
	}
}
