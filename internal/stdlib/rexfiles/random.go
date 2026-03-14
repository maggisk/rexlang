package rexfiles

import "math/rand/v2"

var RandomFFI = map[string]any{
	"systemSeed": Random_systemSeed,
}

func Random_systemSeed() int {
	return rand.IntN(2147483646) + 1
}
