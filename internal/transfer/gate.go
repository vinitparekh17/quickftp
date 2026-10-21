package transfer

import (
	"context"
	"sync"
	"time"
)

// Gate is shared by manual transfers and deployment uploads. Limit changes affect
// subsequent acquisitions while already running work is allowed to finish.
type Gate struct {
	mu            sync.Mutex
	limit, active int
}

func NewGate(limit int) *Gate { g := &Gate{}; g.SetLimit(limit); return g }
func (g *Gate) SetLimit(limit int) {
	if limit < 1 {
		limit = 1
	}
	if limit > 5 {
		limit = 5
	}
	g.mu.Lock()
	g.limit = limit
	g.mu.Unlock()
}
func (g *Gate) Limit() int { g.mu.Lock(); defer g.mu.Unlock(); return g.limit }
func (g *Gate) Acquire(ctx context.Context) (func(), error) {
	for {
		g.mu.Lock()
		if g.active < g.limit {
			g.active++
			g.mu.Unlock()
			var once sync.Once
			return func() { once.Do(func() { g.mu.Lock(); g.active--; g.mu.Unlock() }) }, nil
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(75 * time.Millisecond):
		}
	}
}
