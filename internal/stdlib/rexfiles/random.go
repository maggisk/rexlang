//go:build ignore

package main

import "math/rand/v2"

func Std_Random_systemSeed(_ any) int64 {
	return int64(rand.IntN(2147483646) + 1)
}
