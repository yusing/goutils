package cache

import (
	"context"
	"errors"
	"time"
)

func waitForBackoff(ctx context.Context, delay time.Duration) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-timer.C:
		return nil
	}
}

func shouldCacheResult(ctx context.Context, err error) bool {
	cause := context.Cause(ctx)
	if cause == nil {
		return true
	}
	return !errors.Is(err, cause)
}
