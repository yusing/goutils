package httpevents

import (
	"net"
	"net/http"
	"strings"

	"github.com/yusing/goutils/events"
	"golang.org/x/sync/singleflight"
)

var singleFlight singleflight.Group

func Blocked(r *http.Request, source, reason string) {
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr
	}
	proto := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		proto = "https"
	}
	baseURL := proto + "://" + r.Host

	singleFlightKey := remoteIP + "|" + r.Host

	_, _, _ = singleFlight.Do(singleFlightKey, func() (any, error) {
		events.Global.Add(events.NewEvent(events.LevelInfo, "http_event", "blocked", map[string]any{
			"remote_ip":   remoteIP,
			"request_url": baseURL,
			"source":      source,
			"reason":      reason,
		}))
		return nil, nil
	})
}
