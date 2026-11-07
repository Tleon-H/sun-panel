package pollguard

import "sync"

// Gate keeps one monitor collection in flight and invalidates it after Stop.
type Gate struct {
	mu      sync.Mutex
	stopped bool
	busy    bool
	gen     int
}

func New() *Gate {
	return &Gate{stopped: true}
}

func (g *Gate) Start() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stopped = false
}

func (g *Gate) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.stopped = true
	g.gen++
}

// TryBegin starts a collection unless the gate is stopped or one is already running.
// current reports whether that collection is still the active one.
func (g *Gate) TryBegin() (ok bool, current func() bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.stopped || g.busy {
		return false, func() bool { return false }
	}
	g.busy = true
	gen := g.gen
	return true, func() bool {
		g.mu.Lock()
		defer g.mu.Unlock()
		return !g.stopped && g.gen == gen
	}
}

func (g *Gate) End() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.busy = false
}
