package events

import "context"

type contextKey struct{}

func SetCtx(target interface{ SetValue(any, any) }, history *History) {
	target.SetValue(contextKey{}, history)
}

func FromCtx(ctx context.Context) *History {
	history, _ := ctx.Value(contextKey{}).(*History)
	return history
}
