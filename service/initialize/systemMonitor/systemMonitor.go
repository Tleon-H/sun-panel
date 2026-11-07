package systemMonitor

import (
	"sync"
	"time"

	"sun-panel/global"
	"sun-panel/lib/cache"
	"sun-panel/lib/monitor"
	"sun-panel/lib/pollguard"
)

type collector func() global.ModelSystemMonitor
type publisher func(global.ModelSystemMonitor)

type clock interface {
	C() <-chan time.Time
	Stop()
}

type tickerClock struct {
	ticker *time.Ticker
}

func (c tickerClock) C() <-chan time.Time { return c.ticker.C }
func (c tickerClock) Stop()               { c.ticker.Stop() }

// Start polls machine stats until the returned function is called.
// InitApp does not call Start; the homepage uses the per-card frontend poller.
// stop waits until the ticker goroutine has exited and the ticker is stopped.
// A sample already in progress is not written after stop.
func Start(cacher cache.Cacher[global.ModelSystemMonitor], interval time.Duration) (stop func()) {
	return startLoop(tickerClock{ticker: time.NewTicker(interval)}, GetInfo, func(info global.ModelSystemMonitor) {
		cacher.SetDefault("value", info)
	})
}

func startLoop(clk clock, collect collector, publish publisher) (stop func()) {
	gate := pollguard.New()
	gate.Start()
	done := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once

	go func() {
		defer close(finished)
		defer clk.Stop()
		defer gate.Stop()

		for {
			select {
			case <-done:
				return
			case <-clk.C():
				ok, current := gate.TryBegin()
				if !ok {
					continue
				}
				go func() {
					defer gate.End()
					if !current() {
						return
					}
					info := collect()
					if current() {
						publish(info)
					}
				}()
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
}

func GetInfo() global.ModelSystemMonitor {

	var modelSystemMonitor global.ModelSystemMonitor

	if cpuInfo, err := monitor.GetCPUInfo(); err == nil {
		modelSystemMonitor.CPUInfo = cpuInfo
	}

	if v, err := monitor.GetDiskInfo(); err == nil {
		modelSystemMonitor.DiskInfo = v
	}

	if v, err := monitor.GetNetIOCountersInfo(); err == nil {
		modelSystemMonitor.NetIOCountersInfo = v
	}

	if v, err := monitor.GetMemoryInfo(); err == nil {
		modelSystemMonitor.MemoryInfo = v
	}

	return modelSystemMonitor
}
