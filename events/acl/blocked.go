package aclevents

import (
	"context"

	"github.com/yusing/goutils/events"
	"golang.org/x/sync/singleflight"
)

var singleFlight singleflight.Group

func Blocked(ctx context.Context, ip string, reason string) {
	_, _, _ = singleFlight.Do(ip, func() (any, error) {
		history := events.FromCtx(ctx)
		if history == nil {
			return nil, nil
		}
		history.Add(events.NewEvent(events.LevelInfo, "acl_event", "blocked", map[string]any{
			"ip":     ip,
			"reason": reason,
		}))
		return nil, nil
	})
}
