//go:build debug

package pool

import (
	"runtime/debug"

	"github.com/rs/zerolog/log"
)

func (p *Pool[T]) checkExists(key string) {
	if cur, ok := p.m.Load(key); ok && !cur.tomb {
		log.Warn().Msgf("%s: key %s already exists\nstacktrace: %s", p.name, key, string(debug.Stack()))
	}
}
