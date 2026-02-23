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
