//go:build ignore

package main

import "math"

func Std_Math_toFloat(i int64) float64  { return float64(i) }
func Std_Math_round(f float64) int64    { return int64(math.Round(f)) }
func Std_Math_floor(f float64) int64    { return int64(math.Floor(f)) }
func Std_Math_ceiling(f float64) int64  { return int64(math.Ceil(f)) }
func Std_Math_truncate(f float64) int64 { return int64(f) }
func Std_Math_sqrt(f float64) float64   { return math.Sqrt(f) }

func Std_Math_abs(v any) any {
	switch n := v.(type) {
	case int64:
		if n < 0 {
			return -n
		}
		return n
	case float64:
		return math.Abs(n)
	}
	panic("abs: expected number")
}

func Std_Math_min(x, y any) any {
	switch a := x.(type) {
	case int64:
		if b, ok := y.(int64); ok {
			if a <= b {
				return a
			}
			return b
		}
	case float64:
		if b, ok := y.(float64); ok {
			if a <= b {
				return a
			}
			return b
		}
	}
	panic("min: expected matching numeric types")
}

func Std_Math_max(x, y any) any {
	switch a := x.(type) {
	case int64:
		if b, ok := y.(int64); ok {
			if a >= b {
				return a
			}
			return b
		}
	case float64:
		if b, ok := y.(float64); ok {
			if a >= b {
				return a
			}
			return b
		}
	}
	panic("max: expected matching numeric types")
}

func Std_Math_pow(x, y any) float64 {
	var xf, yf float64
	switch v := x.(type) {
	case int64:
		xf = float64(v)
	case float64:
		xf = v
	}
	switch v := y.(type) {
	case int64:
		yf = float64(v)
	case float64:
		yf = v
	}
	return math.Pow(xf, yf)
}

func Std_Math_sin(f float64) float64  { return math.Sin(f) }
func Std_Math_cos(f float64) float64  { return math.Cos(f) }
func Std_Math_tan(f float64) float64  { return math.Tan(f) }
func Std_Math_asin(f float64) float64 { return math.Asin(f) }
func Std_Math_acos(f float64) float64 { return math.Acos(f) }
func Std_Math_atan(f float64) float64 { return math.Atan(f) }
func Std_Math_atan2(y, x float64) float64 { return math.Atan2(y, x) }
func Std_Math_log(f float64) float64  { return math.Log(f) }
func Std_Math_exp(f float64) float64  { return math.Exp(f) }

var Std_Math_pi = math.Pi
var Std_Math_e = math.E
