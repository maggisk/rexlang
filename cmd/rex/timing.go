package main

import (
	"fmt"
	"os"
	"time"
)

// timeMode is set by the --time flag; prints timing breakdown per compilation phase.
var timeMode bool

// phaseTimer accumulates timing information for compilation phases.
type phaseTimer struct {
	start  time.Time
	prev   time.Time
	phases []phaseEntry
}

type phaseEntry struct {
	name     string
	duration time.Duration
}

func newPhaseTimer() *phaseTimer {
	now := time.Now()
	return &phaseTimer{start: now, prev: now}
}

// mark records the end of the current phase and starts a new one.
func (pt *phaseTimer) mark(name string) {
	now := time.Now()
	pt.phases = append(pt.phases, phaseEntry{name: name, duration: now.Sub(pt.prev)})
	pt.prev = now
}

func (pt *phaseTimer) print() {
	total := time.Since(pt.start)
	for _, p := range pt.phases {
		pct := float64(p.duration) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "  %-20s %s (%4.1f%%)\n", p.name, p.duration.Round(time.Millisecond), pct)
	}
	fmt.Fprintf(os.Stderr, "  %-20s %s\n", "Total", total.Round(time.Millisecond))
}
