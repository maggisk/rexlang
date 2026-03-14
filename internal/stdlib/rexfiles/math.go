package rexfiles

import "math"

var MathFFI = map[string]any{
	"toFloat":  Math_toFloat,
	"round":    Math_round,
	"floor":    Math_floor,
	"ceiling":  Math_ceiling,
	"truncate": Math_truncate,
	"sqrt":     math.Sqrt,
	"sin":      math.Sin,
	"cos":      math.Cos,
	"tan":      math.Tan,
	"asin":     math.Asin,
	"acos":     math.Acos,
	"atan":     math.Atan,
	"log":      math.Log,
	"exp":      math.Exp,
	"atan2":    math.Atan2,
	"pi":       math.Pi,
	"e":        math.E,
}

func Math_toFloat(i int) float64  { return float64(i) }
func Math_round(f float64) int    { return int(math.Round(f)) }
func Math_floor(f float64) int    { return int(math.Floor(f)) }
func Math_ceiling(f float64) int  { return int(math.Ceil(f)) }
func Math_truncate(f float64) int { return int(f) }
