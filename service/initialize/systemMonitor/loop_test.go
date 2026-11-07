package systemMonitor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"sun-panel/global"
)

type fakeClock struct {
	ch      chan time.Time
	stopped atomic.Bool
}

func (f *fakeClock) C() <-chan time.Time { return f.ch }
func (f *fakeClock) Stop()               { f.stopped.Store(true) }

func TestLoopStopReleasesTickerAndDropsOverlappingOrLateSamples(t *testing.T) {
	clk := &fakeClock{ch: make(chan time.Time, 2)}
	release := make(chan struct{})
	started := make(chan struct{})
	var once sync.Once
	var calls atomic.Int32
	var published atomic.Int32

	stop := startLoop(clk, func() global.ModelSystemMonitor {
		calls.Add(1)
		once.Do(func() { close(started) })
		<-release
		return global.ModelSystemMonitor{}
	}, func(global.ModelSystemMonitor) {
		published.Add(1)
	})

	clk.ch <- time.Now()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("collect did not start")
	}
	clk.ch <- time.Now()
	time.Sleep(50 * time.Millisecond)
	if calls.Load() != 1 {
		t.Fatalf("overlapping collect calls = %d", calls.Load())
	}

	stop()
	if !clk.stopped.Load() {
		t.Fatal("ticker was not stopped")
	}
	close(release)
	time.Sleep(50 * time.Millisecond)
	if published.Load() != 0 {
		t.Fatalf("sample published after stop: %d", published.Load())
	}
	stop()
}

func TestLoopPublishesOneCompletedSample(t *testing.T) {
	clk := &fakeClock{ch: make(chan time.Time, 1)}
	started := make(chan struct{})
	var published atomic.Int32
	stop := startLoop(clk, func() global.ModelSystemMonitor {
		close(started)
		return global.ModelSystemMonitor{}
	}, func(global.ModelSystemMonitor) {
		published.Add(1)
	})
	defer stop()

	clk.ch <- time.Now()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("collect did not start")
	}
	deadline := time.Now().Add(time.Second)
	for published.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if published.Load() != 1 {
		t.Fatalf("published = %d", published.Load())
	}
}
