package rexfiles

import "runtime"

var ParallelFFI = map[string]any{
	"numCPU": runtime.NumCPU(),
}
