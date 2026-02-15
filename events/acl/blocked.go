package aclevents

import (
	"github.com/yusing/goutils/events"
	"golang.org/x/sync/singleflight"
)

var singleFlight singleflight.Group

func Blocked(ip string, reason string) {
	_, _, _ = singleFlight.Do(ip, func() (any, error) {
		events.Global.Add(events.NewEvent(events.LevelInfo, "acl_event", "blocked", map[string]any{
			"ip":     ip,
			"reason": reason,
		}))
		return nil, nil
	})
}
