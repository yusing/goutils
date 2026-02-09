package pool

import (
	"sort"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/rs/zerolog/log"
)

const (
	recentlyRemovedTTL = time.Second
	tombPurgeThreshold = uint32(256)
)

type removedInfo struct {
	name      string
	display   string
	removedAt time.Time
}

type entry[T Object] struct {
	obj     T
	removed removedInfo
	tomb    bool
}

func displayNameOf(obj Object) string {
	if withDisp, ok := obj.(ObjectWithDisplayName); ok {
		return withDisp.DisplayName()
	}
	return obj.Name()
}

type (
	Pool[T Object] struct {
		m          *xsync.Map[string, entry[T]]
		name       string
		disableLog atomic.Bool
		tombs      atomic.Uint32
	}
	// Preferable allows an object to express deterministic replacement preference
	// when multiple objects with the same key are added to the pool.
	// If new.PreferOver(old) returns true, the new object replaces the old one.
	Preferable interface {
		PreferOver(other any) bool
	}
	Object interface {
		Key() string
		Name() string
	}
	ObjectWithDisplayName interface {
		Object
		DisplayName() string
	}
)

func New[T Object](name string) *Pool[T] {
	return &Pool[T]{m: xsync.NewMap[string, entry[T]](), name: name}
}

func (p *Pool[T]) DisableLog(v bool) {
	p.disableLog.Store(v)
}

func (p *Pool[T]) Name() string {
	return p.name
}

func (p *Pool[T]) Add(obj T) {
	p.AddKey(obj.Key(), obj)
}

func (p *Pool[T]) AddKey(key string, obj T) {
	now := time.Now()
	action := "added"
	if cur, exists := p.m.Load(key); exists && !cur.tomb {
		if newPref, ok := any(obj).(Preferable); ok && !newPref.PreferOver(cur.obj) {
			// keep existing
			return
		}
	}
	p.checkExists(key)

	if cur, exists := p.m.Load(key); exists && cur.tomb {
		if now.Sub(cur.removed.removedAt) < recentlyRemovedTTL {
			action = "reloaded"
		}
		p.tombs.Add(^uint32(0)) // decrement tomb count
	}

	p.m.Store(key, entry[T]{obj: obj})
	p.logAction(action, obj)
}

func (p *Pool[T]) AddIfNotExists(obj T) (actual T, added bool) {
	key := obj.Key()
	now := time.Now()
	cur, exists := p.m.Load(key)
	if exists {
		if !cur.tomb {
			return cur.obj, false
		}
		if now.Sub(cur.removed.removedAt) < recentlyRemovedTTL {
			p.tombs.Add(^uint32(0)) // decrement tomb count
			p.m.Store(key, entry[T]{obj: obj})
			p.logAction("reloaded", obj)
			return obj, true
		}
		return cur.obj, false
	}
	p.m.Store(key, entry[T]{obj: obj})
	p.logAction("added", obj)
	return obj, true
}

func (p *Pool[T]) Del(obj T) {
	p.delKey(obj.Key(), displayNameOf(obj))
}

func (p *Pool[T]) DelKey(key string) {
	p.delKey(key, "")
}

func (p *Pool[T]) delKey(key string, display string) {
	cur, exists := p.m.Load(key)
	if !exists || cur.tomb {
		return
	}

	info := removedInfo{
		removedAt: time.Now(),
		name:      cur.obj.Name(),
		display:   display,
	}
	if info.display == "" {
		info.display = displayNameOf(cur.obj)
	}
	p.m.Store(key, entry[T]{removed: info, tomb: true})
	if p.tombs.Add(1) > tombPurgeThreshold {
		p.PurgeExpiredTombs()
	}
}

func (p *Pool[T]) Get(key string) (T, bool) {
	var zero T
	cur, ok := p.m.Load(key)
	if !ok || cur.tomb {
		return zero, false
	}
	return cur.obj, true
}

func (p *Pool[T]) Size() int {
	return p.m.Size()
}

func (p *Pool[T]) Clear() {
	p.m.Clear()
}

func (p *Pool[T]) Iter(fn func(k string, v T) bool) {
	for k, v := range p.m.Range {
		if v.tomb {
			continue
		}
		if !fn(k, v.obj) {
			return
		}
	}
}

func (p *Pool[T]) Slice() []T {
	slice := make([]T, 0, p.m.Size()-int(p.tombs.Load()))
	for _, v := range p.m.Range {
		if v.tomb {
			continue
		}
		slice = append(slice, v.obj)
	}
	sort.Slice(slice, func(i, j int) bool {
		return slice[i].Name() < slice[j].Name()
	})
	return slice
}

func (p *Pool[T]) logRemoved(info removedInfo) {
	if p.disableLog.Load() {
		return
	}
	if info.display != info.name {
		log.Info().Msgf("%s: removed %s (%s)", p.name, info.display, info.name)
	} else {
		log.Info().Msgf("%s: removed %s", p.name, info.name)
	}
}

func (p *Pool[T]) logAction(action string, obj T) {
	if p.disableLog.Load() {
		return
	}
	name := obj.Name()
	disp := displayNameOf(obj)
	if disp != name {
		log.Info().Msgf("%s: %s %s (%s)", p.name, action, disp, name)
		return
	}
	log.Info().Msgf("%s: %s %s", p.name, action, name)
}

func (p *Pool[T]) PurgeExpiredTombs() (purged int) {
	now := time.Now()
	for k, v := range p.m.Range {
		if !v.tomb || now.Sub(v.removed.removedAt) < recentlyRemovedTTL {
			continue
		}

		cur, ok := p.m.Load(k)
		if !ok || !cur.tomb || cur.removed.removedAt != v.removed.removedAt {
			continue
		}

		deleted, ok := p.m.LoadAndDelete(k)
		if !ok {
			continue
		}
		if deleted.tomb && now.Sub(deleted.removed.removedAt) >= recentlyRemovedTTL {
			p.tombs.Add(^uint32(0))
			purged++
			p.logRemoved(deleted.removed)
			continue
		}
		p.m.Store(k, deleted)
	}
	return purged
}
