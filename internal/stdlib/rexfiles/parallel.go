//go:build ignore

package main

import "runtime"

var Stdlib_Parallel_numCPU = int64(runtime.NumCPU())
