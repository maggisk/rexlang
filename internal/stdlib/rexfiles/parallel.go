//go:build ignore

package main

import "runtime"

var Std_Parallel_numCPU = int64(runtime.NumCPU())
