//go:build debug

package pool

import (
	"fmt"
	"runtime/debug"

	"github.com/yusing/goutils/logging"
)

func (p *Pool[T]) logExisting(key string) {
	logging.Log(logging.Warn, fmt.Sprintf("%s: key %s already exists\nstacktrace: %s", p.name, key, string(debug.Stack())))
}
