package httputils

import (
	"net/http/httptest"
	"testing"
)

func TestBasicAuthRepeatedMissRegression(t *testing.T) {
	for _, header := range []string{"", "Basic invalid", "Bearer token"} {
		t.Run(header, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set("Authorization", header)
			c := NewCache()
			defer c.Release()
			for range 3 {
				if got := c.GetBasicAuth(r); got != nil {
					t.Fatalf("GetBasicAuth = %v, want nil", got)
				}
			}
		})
	}
}
