package systemMonitor

import (
	"sun-panel/global"
	"sun-panel/lib/cache"
	"sun-panel/lib/monitor"
	"sun-panel/lib/pollguard"
	"sync"
	"time"
)

// Start polls machine stats until the returned function is called.
// InitApp does not call Start; the homepage uses the per-card frontend poller.
// The previous loop had no stop and launched another collection on every tick.
func Start(cacher cache.Cacher[global.ModelSystemMonitor], interval time.Duration) (stop func()) {
	gate := pollguard.New()
	gate.Start()
	done := make(chan struct{})
	var once sync.Once

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer gate.Stop()

		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				ok, current := gate.TryBegin()
				if !ok {
					continue
				}
				go func() {
					defer gate.End()
					if !current() {
						return
					}
					monitorInfo := GetInfo()
					if current() {
						cacher.SetDefault("value", monitorInfo)
					}
				}()
			}
		}
	}()

	return func() {
		once.Do(func() {
			close(done)
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
