package httputils

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

// Cache is a map of cached values for a request.
// It prevents the same value from being parsed multiple times.
type (
	Cache             map[string]any
	UpdateFunc[T any] func(T) T

	Credentials struct {
		Username string
		Password []byte
	}
)

const (
	cacheKeyQueries    = "queries"
	cacheKeyCookies    = "cookies"
	cacheKeyCookiesMap = "cookies_map"
	cacheKeyRemoteIP   = "remote_ip"
	cacheKeyBasicAuth  = "basic_auth"
)

var cachePool = sync.Pool{
	New: func() any {
		return make(Cache)
	},
}

// NewCache returns a new Cached.
func NewCache() Cache {
	return cachePool.Get().(Cache)
}

// Release clear the contents of the Cached and returns it to the pool.
func (c Cache) Release() {
	clear(c)
	cachePool.Put(c)
}

// GetQueries returns the queries.
// If r does not have queries, an empty map is returned.
func (c Cache) GetQueries(r *http.Request) url.Values {
	v, ok := c[cacheKeyQueries]
	if !ok {
		v = r.URL.Query()
		c[cacheKeyQueries] = v
	}
	return v.(url.Values)
}

func (c Cache) UpdateQueries(r *http.Request, update func(url.Values)) {
	queries := c.GetQueries(r)
	update(queries)
	r.URL.RawQuery = queries.Encode()
}

// GetCookies returns the cookies.
// If r does not have cookies, an empty slice is returned.
func (c Cache) GetCookies(r *http.Request) []*http.Cookie {
	v, ok := c[cacheKeyCookies]
	if !ok {
		v = r.Cookies()
		c[cacheKeyCookies] = v
	}
	return v.([]*http.Cookie)
}

func (c Cache) GetCookiesMap(r *http.Request) url.Values {
	v, ok := c[cacheKeyCookiesMap]
	if !ok {
		vv := make(url.Values)
		for _, cookie := range c.GetCookies(r) {
			vv[cookie.Name] = joinCookieValues(vv[cookie.Name], cookie.Value)
		}
		c[cacheKeyCookiesMap] = vv
		return vv
	}
	return v.(url.Values)
}

func joinCookieValues(cookies []string, newCookie string) []string {
	nNew := strings.Count(newCookie, ";")
	if nNew == 0 {
		return append(cookies, newCookie)
	}
	values := make([]string, 0, nNew)
	for value := range strings.SplitSeq(newCookie, ";") {
		values = append(values, strings.TrimSpace(value))
	}
	return values
}

func (c Cache) UpdateCookies(r *http.Request, update UpdateFunc[[]*http.Cookie]) {
	cookies := update(c.GetCookies(r))
	c[cacheKeyCookies] = cookies
	cookiesMap := make(url.Values)
	for _, cookie := range cookies {
		cookiesMap[cookie.Name] = joinCookieValues(cookiesMap[cookie.Name], cookie.Value)
	}
	c[cacheKeyCookiesMap] = cookiesMap
	r.Header.Del("Cookie")
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
}

// GetRemoteIP returns the remote ip address.
// If r.RemoteAddr is not a valid ip address, nil is returned.
func (c Cache) GetRemoteIP(r *http.Request) net.IP {
	v, ok := c[cacheKeyRemoteIP]
	if !ok {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		v = net.ParseIP(host)
		c[cacheKeyRemoteIP] = v
	}
	return v.(net.IP)
}

// GetBasicAuth returns *Credentials the basic auth username and password.
// If r does not have basic auth, nil is returned.
func (c Cache) GetBasicAuth(r *http.Request) *Credentials {
	v, ok := c[cacheKeyBasicAuth]
	if !ok {
		u, p, ok := r.BasicAuth()
		if ok {
			v = &Credentials{u, []byte(p)}
			c[cacheKeyBasicAuth] = v
		} else {
			c[cacheKeyBasicAuth] = nil
			return nil
		}
	}
	return v.(*Credentials)
}
