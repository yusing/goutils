package task

import (
	"context"
	"time"
)

type ctxWithValues struct {
	task *Task
}

var _ context.Context = ctxWithValues{}

func (w ctxWithValues) Value(key any) any {
	if value := w.task.GetValue(key); value != nil {
		return value
	}
	return w.task.ctx.Value(key)
}

func (w ctxWithValues) Deadline() (time.Time, bool) {
	return w.task.ctx.Deadline()
}

func (w ctxWithValues) Done() <-chan struct{} {
	return w.task.ctx.Done()
}

func (w ctxWithValues) Err() error {
	return w.task.ctx.Err()
}
