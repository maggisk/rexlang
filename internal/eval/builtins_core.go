package eval

import (
	"fmt"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Builtin helpers
// ---------------------------------------------------------------------------

func MakeBuiltin(name string, fn func(Value) (Value, error)) Value {
	return VBuiltin{Name: name, Fn: fn}
}

func Curried2(name string, fn func(Value, Value) (Value, error)) Value {
	return MakeBuiltin(name, func(a Value) (Value, error) {
		return MakeBuiltin(name+"$1", func(b Value) (Value, error) {
			return fn(a, b)
		}), nil
	})
}

// CoreBuiltins returns the minimal builtins: not, error, todo, showInt, showFloat.
func CoreBuiltins() map[string]Value {
	return map[string]Value{
		"not": MakeBuiltin("not", func(v Value) (Value, error) {
			b, err := AsBool(v)
			if err != nil {
				return nil, err
			}
			return VBool{V: !b}, nil
		}),
		"error": MakeBuiltin("error", func(v Value) (Value, error) {
			s, err := CheckStr("error", v)
			if err != nil {
				return nil, err
			}
			return nil, &RuntimeError{Msg: s}
		}),
		"todo": MakeBuiltin("todo", func(v Value) (Value, error) {
			s, err := CheckStr("todo", v)
			if err != nil {
				return nil, err
			}
			return nil, &RuntimeError{Msg: "TODO: " + s}
		}),
		"showInt": MakeBuiltin("showInt", func(v Value) (Value, error) {
			i, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			return VString{V: fmt.Sprintf("%d", i)}, nil
		}),
		"showFloat": MakeBuiltin("showFloat", func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VString{V: FloatToStr(f)}, nil
		}),
	}
}

// BuiltinsForModule returns builtins for a stdlib module: CoreBuiltins + module-specific ones.
func BuiltinsForModule(name string) map[string]Value {
	result := make(map[string]Value)
	for k, v := range CoreBuiltins() {
		result[k] = v
	}

	// Auto-discover builtins from companion Go files
	loadRegisteredBuiltins(name, result)

	return result
}

func FloatToStr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") &&
		!strings.Contains(s, "E") && !strings.Contains(s, "n") && !strings.Contains(s, "N") {
		s += ".0"
	}
	return s
}
