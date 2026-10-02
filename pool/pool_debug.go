//go:build debug

package pool

import (
	"fmt"
	"runtime/debug"

	"github.com/yusing/goutils/logging"
)

func (p *Pool[T]) checkExists(key string) {
	if cur, ok := p.m.Load(key); ok && !cur.tomb {
		logging.Log(logging.Warn, fmt.Sprintf("%s: key %s already exists\nstacktrace: %s", p.name, key, string(debug.Stack())))
	}
}
