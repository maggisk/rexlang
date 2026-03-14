package rexfiles

import (
	"math"

	"github.com/maggisk/rexlang/internal/eval"
)

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
	// Polymorphic builtins that handle both Int and Float
	"abs": eval.MakeBuiltin("abs", func(v eval.Value) (eval.Value, error) {
		switch n := v.(type) {
		case eval.VInt:
			a := n.V
			if a < 0 {
				a = -a
			}
			return eval.VInt{V: a}, nil
		case eval.VFloat:
			return eval.VFloat{V: math.Abs(n.V)}, nil
		}
		return nil, &eval.RuntimeError{Msg: "abs: expected number, got " + eval.ValueToString(v)}
	}),
	"min": eval.MakeBuiltin("min", func(x eval.Value) (eval.Value, error) {
		return eval.MakeBuiltin("min$1", func(y eval.Value) (eval.Value, error) {
			switch a := x.(type) {
			case eval.VInt:
				if b, ok := y.(eval.VInt); ok {
					if a.V <= b.V {
						return a, nil
					}
					return b, nil
				}
			case eval.VFloat:
				if b, ok := y.(eval.VFloat); ok {
					if a.V <= b.V {
						return a, nil
					}
					return b, nil
				}
			}
			return nil, &eval.RuntimeError{Msg: "min: expected matching numeric types"}
		}), nil
	}),
	"max": eval.MakeBuiltin("max", func(x eval.Value) (eval.Value, error) {
		return eval.MakeBuiltin("max$1", func(y eval.Value) (eval.Value, error) {
			switch a := x.(type) {
			case eval.VInt:
				if b, ok := y.(eval.VInt); ok {
					if a.V >= b.V {
						return a, nil
					}
					return b, nil
				}
			case eval.VFloat:
				if b, ok := y.(eval.VFloat); ok {
					if a.V >= b.V {
						return a, nil
					}
					return b, nil
				}
			}
			return nil, &eval.RuntimeError{Msg: "max: expected matching numeric types"}
		}), nil
	}),
	"pow": eval.MakeBuiltin("pow", func(x eval.Value) (eval.Value, error) {
		return eval.MakeBuiltin("pow$1", func(y eval.Value) (eval.Value, error) {
			var xf, yf float64
			switch v := x.(type) {
			case eval.VInt:
				xf = float64(v.V)
			case eval.VFloat:
				xf = v.V
			default:
				return nil, &eval.RuntimeError{Msg: "pow: expected number"}
			}
			switch v := y.(type) {
			case eval.VInt:
				yf = float64(v.V)
			case eval.VFloat:
				yf = v.V
			default:
				return nil, &eval.RuntimeError{Msg: "pow: expected number"}
			}
			return eval.VFloat{V: math.Pow(xf, yf)}, nil
		}), nil
	}),
}

func Math_toFloat(i int) float64  { return float64(i) }
func Math_round(f float64) int    { return int(math.Round(f)) }
func Math_floor(f float64) int    { return int(math.Floor(f)) }
func Math_ceiling(f float64) int  { return int(math.Ceil(f)) }
func Math_truncate(f float64) int { return int(f) }
