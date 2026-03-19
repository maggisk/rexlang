package main

import (
	"fmt"
	"os"
	"time"
)

// timing tracks per-phase compilation timing when --time is active.
type timing struct {
	enabled bool
	phases  []timedPhase
	start   time.Time
}

type timedPhase struct {
	name     string
	duration time.Duration
}

func (t *timing) phase(name string) func() {
	if !t.enabled {
		return func() {}
	}
	start := time.Now()
	return func() {
		t.phases = append(t.phases, timedPhase{name, time.Since(start)})
	}
}

func (t *timing) print() {
	if !t.enabled {
		return
	}
	total := time.Since(t.start)
	for _, p := range t.phases {
		pct := float64(p.duration) / float64(total) * 100
		fmt.Fprintf(os.Stderr, "  %-20s %s (%4.1f%%)\n", p.name, p.duration.Truncate(time.Millisecond), pct)
	}
	fmt.Fprintf(os.Stderr, "  %-20s %s\n", "Total", total.Truncate(time.Millisecond))
}
