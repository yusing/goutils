package gperr

import "sync"

type Group struct {
	b  Builder
	wg sync.WaitGroup
}

func NewGroup(context string) *Group {
	return &Group{
		b: *NewBuilder(context),
	}
}

func (g *Group) Go(fn func() error) {
	// not using wg.Go here to avoid wrapping fn twice
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := fn(); err != nil {
			g.b.Add(err)
		}
	}()
}

func (g *Group) Wait() *Builder {
	g.wg.Wait()
	return &g.b
}
