//go:build debug

package task

import (
	"runtime/debug"

	"github.com/yusing/goutils/logging"
)

func panicWithDebugStack() {
	panic(string(debug.Stack()))
}

func logStarted(t *Task) {
	logging.Log(logging.Info, "task "+t.String()+" started")
}

func logFinished(t *Task) {
	logging.Log(logging.Info, "task "+t.String()+" finished")
}
