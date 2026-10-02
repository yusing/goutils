package pool

import (
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/puzpuzpuz/xsync/v4"
	"github.com/yusing/goutils/events"
	"github.com/yusing/goutils/logging"
)

const (
	recentlyRemovedTTL = time.Second
	tombPurgeThreshold = 256
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

type poolState[T Object] struct {
	m     *xsync.Map[string, entry[T]]
	tombs atomic.Int64
}

func displayNameOf(obj Object) string {
	if withDisp, ok := obj.(ObjectWithDisplayName); ok {
		return withDisp.DisplayName()
	}
	return obj.Name()
}

type (
	Pool[T Object] struct {
		state      atomic.Pointer[poolState[T]]
		name       string
		eventKey   string
		history    *events.History
		disableLog atomic.Bool
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

func New[T Object](name, eventKey string) *Pool[T] {
	p := &Pool[T]{name: name, eventKey: eventKey}
	p.Clear()
	return p
}

func (p *Pool[T]) SetEventHistory(history *events.History) {
	p.history = history
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
	s := p.state.Load()
	action := "added"
	var added, replaced bool
	s.m.Compute(key, func(cur entry[T], exists bool) (entry[T], xsync.ComputeOp) {
		if exists && !cur.tomb {
			if newPref, ok := any(obj).(Preferable); ok && !newPref.PreferOver(cur.obj) {
				return cur, xsync.CancelOp
			}
			replaced = true
		}
		if exists && cur.tomb {
			if time.Since(cur.removed.removedAt) < recentlyRemovedTTL {
				action = "reloaded"
			}
			s.tombs.Add(-1)
		}
		added = true
		return entry[T]{obj: obj}, xsync.UpdateOp
	})
	if !added {
		return
	}
	if replaced {
		p.logExisting(key)
	}
	p.logAction(action, obj)
}

func (p *Pool[T]) AddIfNotExists(obj T) (actual T, added bool) {
	s := p.state.Load()
	key := obj.Key()
	action := "added"
	cur, _ := s.m.Compute(key, func(cur entry[T], exists bool) (entry[T], xsync.ComputeOp) {
		if exists {
			if !cur.tomb {
				return cur, xsync.CancelOp
			}
			if time.Since(cur.removed.removedAt) < recentlyRemovedTTL {
				action = "reloaded"
			}
			s.tombs.Add(-1)
		}
		added = true
		return entry[T]{obj: obj}, xsync.UpdateOp
	})
	if added {
		p.logAction(action, obj)
	}
	return cur.obj, added
}

func (p *Pool[T]) Del(obj T) {
	p.delKey(obj.Key(), displayNameOf(obj))
}

func (p *Pool[T]) DelKey(key string) {
	p.delKey(key, "")
}

func (p *Pool[T]) delKey(key string, display string) {
	s := p.state.Load()
	var tombs int64
	s.m.Compute(key, func(cur entry[T], exists bool) (entry[T], xsync.ComputeOp) {
		if !exists || cur.tomb {
			return cur, xsync.CancelOp
		}
		info := removedInfo{
			removedAt: time.Now(),
			name:      cur.obj.Name(),
			display:   display,
		}
		if info.display == "" {
			info.display = displayNameOf(cur.obj)
		}
		tombs = s.tombs.Add(1)
		return entry[T]{removed: info, tomb: true}, xsync.UpdateOp
	})
	if tombs > tombPurgeThreshold {
		p.purgeExpiredTombs(s)
	}
}

func (p *Pool[T]) Get(key string) (T, bool) {
	var zero T
	cur, ok := p.state.Load().m.Load(key)
	if !ok || cur.tomb {
		return zero, false
	}
	return cur.obj, true
}

func (p *Pool[T]) Size() int {
	return p.state.Load().m.Size()
}

func (p *Pool[T]) Clear() {
	// In-flight operations retain the old generation, including its tombstone count.
	p.state.Store(&poolState[T]{m: xsync.NewMap[string, entry[T]]()})
}

func (p *Pool[T]) Iter(fn func(k string, v T) bool) {
	for k, v := range p.state.Load().m.Range {
		if v.tomb {
			continue
		}
		if !fn(k, v.obj) {
			return
		}
	}
}

func (p *Pool[T]) Slice() []T {
	s := p.state.Load()
	slice := make([]T, 0, s.m.Size())
	for _, v := range s.m.Range {
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
	if p.history != nil {
		p.history.Add(events.NewEvent(events.LevelInfo, "pool."+p.eventKey, "removed", struct {
			Name      string    `json:"name"`
			Display   string    `json:"display"`
			RemovedAt time.Time `json:"removed_at"`
		}{info.name, info.display, info.removedAt}))
	}
	if p.disableLog.Load() {
		return
	}
	if info.display != info.name {
		logging.Log(logging.Info, fmt.Sprintf("%s: removed %s (%s)", p.name, info.display, info.name))
	} else {
		logging.Log(logging.Info, fmt.Sprintf("%s: removed %s", p.name, info.name))
	}
}

func (p *Pool[T]) logAction(action string, obj T) {
	if p.history != nil {
		p.history.Add(events.NewEvent(events.LevelInfo, "pool."+p.eventKey, action, obj))
	}
	if p.disableLog.Load() {
		return
	}
	name := obj.Name()
	disp := displayNameOf(obj)
	if disp != name {
		logging.Log(logging.Info, fmt.Sprintf("%s: %s %s (%s)", p.name, action, disp, name))
		return
	}
	logging.Log(logging.Info, fmt.Sprintf("%s: %s %s", p.name, action, name))
}

func (p *Pool[T]) PurgeExpiredTombs() (purged int) {
	return p.purgeExpiredTombs(p.state.Load())
}

func (p *Pool[T]) purgeExpiredTombs(s *poolState[T]) (purged int) {
	now := time.Now()
	for k, v := range s.m.Range {
		if !v.tomb || now.Sub(v.removed.removedAt) < recentlyRemovedTTL {
			continue
		}

		var removed bool
		var info removedInfo
		s.m.Compute(k, func(cur entry[T], exists bool) (entry[T], xsync.ComputeOp) {
			if !exists || !cur.tomb || now.Sub(cur.removed.removedAt) < recentlyRemovedTTL {
				return cur, xsync.CancelOp
			}
			s.tombs.Add(-1)
			info = cur.removed
			removed = true
			return cur, xsync.DeleteOp
		})
		if removed {
			purged++
			p.logRemoved(info)
		}
	}
	return purged
}
