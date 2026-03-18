//go:build ignore

package main

import (
	"runtime"
	"strings"
)

// Std_Result_try executes a thunk and catches division/modulo-by-zero panics,
// returning the full Result ADT directly (since the error type is RuntimeError, not String).
func Std_Result_try(thunk any) (result any) {
	defer func() {
		if r := recover(); r != nil {
			if re, ok := r.(runtime.Error); ok {
				msg := re.Error()
				switch {
				case strings.Contains(msg, "divide by zero"):
					result = Rex_Result_Err{F0: Rex_RuntimeError_DivisionByZero{}}
					return
				case strings.Contains(msg, "modulo by zero"):
					result = Rex_Result_Err{F0: Rex_RuntimeError_ModuloByZero{}}
					return
				}
			}
			panic(r) // re-panic for non-div/mod errors
		}
	}()
	val := rex__apply(thunk, nil)
	return Rex_Result_Ok{F0: val}
}
