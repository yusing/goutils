package gperr

import "sync"

// Group is a collection of errors that can be added to and waited on.
//
// Unlike sync/errgroup.Group, Group does not stop on the first error.
type Group struct {
	b  Builder
	mu sync.Mutex
	wg sync.WaitGroup
}

// NewGroup creates a new Group.
func NewGroup(context string) Group {
	return Group{
		b: NewBuilder(context),
	}
}

// Go runs a function in a goroutine and adds the error to the Group.
func (g *Group) Go(fn func() error) {
	// not using wg.Go here to avoid wrapping fn twice
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := fn(); err != nil {
			g.Add(err)
		}
	}()
}

// Add adds an error to the Group.
//
// It is concurrent safe.
func (g *Group) Add(err error) {
	if err == nil {
		return
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.b.Add(err)
}

// Addf adds a formatted error to the Group.
//
// It is concurrent safe.
func (g *Group) Addf(format string, args ...any) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.b.Addf(format, args...)
}

// Wait waits for all errors to be added and returns the Builder.
func (g *Group) Wait() *Builder {
	g.wg.Wait()
	return &g.b
}
