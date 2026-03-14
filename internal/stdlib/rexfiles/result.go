package rexfiles

import "github.com/maggisk/rexlang/internal/eval"

var ResultFFI = map[string]any{
	"try": eval.MakeBuiltin("try", func(fnV eval.Value) (eval.Value, error) {
		val, err := eval.ApplyValue(fnV, eval.VUnit{})
		if err != nil {
			if re, ok := err.(*eval.RuntimeError); ok {
				switch re.Msg {
				case "division by zero":
					return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VCtor{Name: "DivisionByZero"}}}, nil
				case "modulo by zero":
					return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VCtor{Name: "ModuloByZero"}}}, nil
				}
			}
			return nil, err
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{val}}, nil
	}),
}
