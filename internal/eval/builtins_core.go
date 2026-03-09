package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// Builtin helpers
// ---------------------------------------------------------------------------

func makeBuiltin(name string, fn func(Value) (Value, error)) Value {
	return VBuiltin{Name: name, Fn: fn}
}

func curried2(name string, fn func(Value, Value) (Value, error)) Value {
	return makeBuiltin(name, func(a Value) (Value, error) {
		return makeBuiltin(name+"$1", func(b Value) (Value, error) {
			return fn(a, b)
		}), nil
	})
}

func curried3(name string, fn func(Value, Value, Value) (Value, error)) Value {
	return makeBuiltin(name, func(a Value) (Value, error) {
		return makeBuiltin(name+"$1", func(b Value) (Value, error) {
			return makeBuiltin(name+"$2", func(c Value) (Value, error) {
				return fn(a, b, c)
			}), nil
		}), nil
	})
}

// CoreBuiltins returns the minimal builtins: not, error.
func CoreBuiltins() map[string]Value {
	return map[string]Value{
		"not": makeBuiltin("not", func(v Value) (Value, error) {
			b, err := AsBool(v)
			if err != nil {
				return nil, err
			}
			return VBool{V: !b}, nil
		}),
		"error": makeBuiltin("error", func(v Value) (Value, error) {
			s, err := CheckStr("error", v)
			if err != nil {
				return nil, err
			}
			return nil, &RuntimeError{Msg: s}
		}),
		"todo": makeBuiltin("todo", func(v Value) (Value, error) {
			s, err := CheckStr("todo", v)
			if err != nil {
				return nil, err
			}
			return nil, &RuntimeError{Msg: "TODO: " + s}
		}),
		"showInt": makeBuiltin("showInt", func(v Value) (Value, error) {
			i, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			return VString{V: fmt.Sprintf("%d", i)}, nil
		}),
		"showFloat": makeBuiltin("showFloat", func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VString{V: floatToStr(f)}, nil
		}),
	}
}

// IOBuiltins returns IO-related builtins.
func IOBuiltins() map[string]Value {
	return map[string]Value{
		"print": makeBuiltin("print", func(v Value) (Value, error) {
			fmt.Print(Display(v))
			return v, nil
		}),
		"println": makeBuiltin("println", func(v Value) (Value, error) {
			fmt.Println(Display(v))
			return v, nil
		}),
		"readLine": makeBuiltin("readLine", func(v Value) (Value, error) {
			prompt, err := CheckStr("readLine", v)
			if err != nil {
				return nil, err
			}
			fmt.Print(prompt)
			var line string
			fmt.Scanln(&line)
			return VString{V: line}, nil
		}),
		"readFile": makeBuiltin("readFile", func(v Value) (Value, error) {
			path, err := CheckStr("readFile", v)
			if err != nil {
				return nil, err
			}
			data, ioErr := os.ReadFile(path)
			if ioErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: ioErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VString{V: string(data)}}}, nil
		}),
		"writeFile": curried2("writeFile", func(pathV, contentV Value) (Value, error) {
			path, err := CheckStr("writeFile", pathV)
			if err != nil {
				return nil, err
			}
			content, err := CheckStr("writeFile", contentV)
			if err != nil {
				return nil, err
			}
			if ioErr := os.WriteFile(path, []byte(content), 0644); ioErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: ioErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
		"appendFile": curried2("appendFile", func(pathV, contentV Value) (Value, error) {
			path, err := CheckStr("appendFile", pathV)
			if err != nil {
				return nil, err
			}
			content, err := CheckStr("appendFile", contentV)
			if err != nil {
				return nil, err
			}
			f, ioErr := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if ioErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: ioErr.Error()}}}, nil
			}
			defer f.Close()
			if _, ioErr = f.WriteString(content); ioErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: ioErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
		"fileExists": makeBuiltin("fileExists", func(v Value) (Value, error) {
			path, err := CheckStr("fileExists", v)
			if err != nil {
				return nil, err
			}
			_, statErr := os.Stat(path)
			return VBool{V: statErr == nil}, nil
		}),
		"listDir": makeBuiltin("listDir", func(v Value) (Value, error) {
			path, err := CheckStr("listDir", v)
			if err != nil {
				return nil, err
			}
			entries, ioErr := os.ReadDir(path)
			if ioErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: ioErr.Error()}}}, nil
			}
			items := make([]Value, len(entries))
			for i, e := range entries {
				items[i] = VString{V: e.Name()}
			}
			return VCtor{Name: "Ok", Args: []Value{VList{Items: items}}}, nil
		}),
	}
}

// MathBuiltins returns math-related builtins.
func MathBuiltins() map[string]Value {
	mathFn1 := func(name string, fn func(float64) float64) Value {
		return makeBuiltin(name, func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VFloat{V: fn(f)}, nil
		})
	}
	return map[string]Value{
		"toFloat": makeBuiltin("toFloat", func(v Value) (Value, error) {
			i, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			return VFloat{V: float64(i)}, nil
		}),
		"round": makeBuiltin("round", func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VInt{V: int(math.Round(f))}, nil
		}),
		"floor": makeBuiltin("floor", func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VInt{V: int(math.Floor(f))}, nil
		}),
		"ceiling": makeBuiltin("ceiling", func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VInt{V: int(math.Ceil(f))}, nil
		}),
		"truncate": makeBuiltin("truncate", func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VInt{V: int(f)}, nil
		}),
		"abs": makeBuiltin("abs", func(v Value) (Value, error) {
			switch n := v.(type) {
			case VInt:
				a := n.V
				if a < 0 {
					a = -a
				}
				return VInt{V: a}, nil
			case VFloat:
				return VFloat{V: math.Abs(n.V)}, nil
			}
			return nil, &RuntimeError{Msg: "abs: expected number, got " + ValueToString(v)}
		}),
		"min": makeBuiltin("min", func(x Value) (Value, error) {
			return makeBuiltin("min$1", func(y Value) (Value, error) {
				switch a := x.(type) {
				case VInt:
					if b, ok := y.(VInt); ok {
						if a.V <= b.V {
							return a, nil
						}
						return b, nil
					}
				case VFloat:
					if b, ok := y.(VFloat); ok {
						if a.V <= b.V {
							return a, nil
						}
						return b, nil
					}
				}
				return nil, &RuntimeError{Msg: "min: expected matching numeric types"}
			}), nil
		}),
		"max": makeBuiltin("max", func(x Value) (Value, error) {
			return makeBuiltin("max$1", func(y Value) (Value, error) {
				switch a := x.(type) {
				case VInt:
					if b, ok := y.(VInt); ok {
						if a.V >= b.V {
							return a, nil
						}
						return b, nil
					}
				case VFloat:
					if b, ok := y.(VFloat); ok {
						if a.V >= b.V {
							return a, nil
						}
						return b, nil
					}
				}
				return nil, &RuntimeError{Msg: "max: expected matching numeric types"}
			}), nil
		}),
		"pow": makeBuiltin("pow", func(x Value) (Value, error) {
			return makeBuiltin("pow$1", func(y Value) (Value, error) {
				var xf, yf float64
				switch v := x.(type) {
				case VInt:
					xf = float64(v.V)
				case VFloat:
					xf = v.V
				default:
					return nil, &RuntimeError{Msg: "pow: expected number"}
				}
				switch v := y.(type) {
				case VInt:
					yf = float64(v.V)
				case VFloat:
					yf = v.V
				default:
					return nil, &RuntimeError{Msg: "pow: expected number"}
				}
				return VFloat{V: math.Pow(xf, yf)}, nil
			}), nil
		}),
		"sqrt": mathFn1("sqrt", math.Sqrt),
		"sin":  mathFn1("sin", math.Sin),
		"cos":  mathFn1("cos", math.Cos),
		"tan":  mathFn1("tan", math.Tan),
		"asin": mathFn1("asin", math.Asin),
		"acos": mathFn1("acos", math.Acos),
		"atan": mathFn1("atan", math.Atan),
		"log":  mathFn1("log", math.Log),
		"exp":  mathFn1("exp", math.Exp),
		"atan2": curried2("atan2", func(yV, xV Value) (Value, error) {
			y, err := AsFloat(yV)
			if err != nil {
				return nil, err
			}
			x, err := AsFloat(xV)
			if err != nil {
				return nil, err
			}
			return VFloat{V: math.Atan2(y, x)}, nil
		}),
		"pi": VFloat{V: math.Pi},
		"e":  VFloat{V: math.E},
	}
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// StringBuiltins returns string builtins.
func StringBuiltins() map[string]Value {
	return map[string]Value{
		"length": makeBuiltin("length", func(v Value) (Value, error) {
			s, err := CheckStr("length", v)
			if err != nil {
				return nil, err
			}
			return VInt{V: utf8.RuneCountInString(s)}, nil
		}),
		"toUpper": makeBuiltin("toUpper", func(v Value) (Value, error) {
			s, err := CheckStr("toUpper", v)
			if err != nil {
				return nil, err
			}
			return VString{V: strings.ToUpper(s)}, nil
		}),
		"toLower": makeBuiltin("toLower", func(v Value) (Value, error) {
			s, err := CheckStr("toLower", v)
			if err != nil {
				return nil, err
			}
			return VString{V: strings.ToLower(s)}, nil
		}),
		"trim": makeBuiltin("trim", func(v Value) (Value, error) {
			s, err := CheckStr("trim", v)
			if err != nil {
				return nil, err
			}
			return VString{V: strings.TrimSpace(s)}, nil
		}),
		"split": curried2("split", func(sepV, strV Value) (Value, error) {
			sep, err := CheckStr("split", sepV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("split", strV)
			if err != nil {
				return nil, err
			}
			parts := strings.Split(s, sep)
			items := make([]Value, len(parts))
			for i, p := range parts {
				items[i] = VString{V: p}
			}
			return VList{Items: items}, nil
		}),
		"join": curried2("join", func(sepV, lstV Value) (Value, error) {
			sep, err := CheckStr("join", sepV)
			if err != nil {
				return nil, err
			}
			lst, ok := lstV.(VList)
			if !ok {
				return nil, &RuntimeError{Msg: "join: expected list"}
			}
			parts := make([]string, len(lst.Items))
			for i, item := range lst.Items {
				s, err := CheckStr("join", item)
				if err != nil {
					return nil, err
				}
				parts[i] = s
			}
			return VString{V: strings.Join(parts, sep)}, nil
		}),
		"toString": makeBuiltin("toString", func(v Value) (Value, error) {
			switch val := v.(type) {
			case VInt:
				return VString{V: fmt.Sprintf("%d", val.V)}, nil
			case VFloat:
				return VString{V: floatToStr(val.V)}, nil
			case VBool:
				if val.V {
					return VString{V: "true"}, nil
				}
				return VString{V: "false"}, nil
			case VString:
				return v, nil
			}
			return nil, &RuntimeError{Msg: "toString: cannot convert " + ValueToString(v)}
		}),
		"contains": curried2("contains", func(subV, strV Value) (Value, error) {
			sub, err := CheckStr("contains", subV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("contains", strV)
			if err != nil {
				return nil, err
			}
			return VBool{V: strings.Contains(s, sub)}, nil
		}),
		"startsWith": curried2("startsWith", func(prefixV, strV Value) (Value, error) {
			prefix, err := CheckStr("startsWith", prefixV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("startsWith", strV)
			if err != nil {
				return nil, err
			}
			return VBool{V: strings.HasPrefix(s, prefix)}, nil
		}),
		"endsWith": curried2("endsWith", func(suffixV, strV Value) (Value, error) {
			suffix, err := CheckStr("endsWith", suffixV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("endsWith", strV)
			if err != nil {
				return nil, err
			}
			return VBool{V: strings.HasSuffix(s, suffix)}, nil
		}),
		"charAt": curried2("charAt", func(idxV, strV Value) (Value, error) {
			idx, err := AsInt(idxV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("charAt", strV)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			if idx >= 0 && idx < len(runes) {
				return VCtor{Name: "Just", Args: []Value{VString{V: string(runes[idx])}}}, nil
			}
			return VCtor{Name: "Nothing", Args: nil}, nil
		}),
		"substring": curried3("substring", func(startV, endV, strV Value) (Value, error) {
			start, err := AsInt(startV)
			if err != nil {
				return nil, err
			}
			end, err := AsInt(endV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("substring", strV)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			n := len(runes)
			sc := clampInt(start, 0, n)
			ec := clampInt(end, 0, n)
			return VString{V: string(runes[sc:ec])}, nil
		}),
		"indexOf": curried2("indexOf", func(needleV, haystackV Value) (Value, error) {
			needle, err := CheckStr("indexOf", needleV)
			if err != nil {
				return nil, err
			}
			haystack, err := CheckStr("indexOf", haystackV)
			if err != nil {
				return nil, err
			}
			byteIdx := strings.Index(haystack, needle)
			if byteIdx == -1 {
				return VCtor{Name: "Nothing", Args: nil}, nil
			}
			runeIdx := utf8.RuneCountInString(haystack[:byteIdx])
			return VCtor{Name: "Just", Args: []Value{VInt{V: runeIdx}}}, nil
		}),
		"replace": curried3("replace", func(findV, replV, strV Value) (Value, error) {
			find, err := CheckStr("replace", findV)
			if err != nil {
				return nil, err
			}
			repl, err := CheckStr("replace", replV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("replace", strV)
			if err != nil {
				return nil, err
			}
			return VString{V: strings.ReplaceAll(s, find, repl)}, nil
		}),
		"take": curried2("take", func(nV, strV Value) (Value, error) {
			n, err := AsInt(nV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("take", strV)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			end := clampInt(n, 0, len(runes))
			return VString{V: string(runes[:end])}, nil
		}),
		"drop": curried2("drop", func(nV, strV Value) (Value, error) {
			n, err := AsInt(nV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("drop", strV)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			start := clampInt(n, 0, len(runes))
			return VString{V: string(runes[start:])}, nil
		}),
		"repeat": curried2("repeat", func(nV, strV Value) (Value, error) {
			n, err := AsInt(nV)
			if err != nil {
				return nil, err
			}
			s, err := CheckStr("repeat", strV)
			if err != nil {
				return nil, err
			}
			if n < 0 {
				n = 0
			}
			return VString{V: strings.Repeat(s, n)}, nil
		}),
		"padLeft": curried3("padLeft", func(widthV, padV, strV Value) (Value, error) {
			width, err := AsInt(widthV)
			if err != nil {
				return nil, err
			}
			pad, err := CheckStr("padLeft", padV)
			if err != nil {
				return nil, err
			}
			if utf8.RuneCountInString(pad) != 1 {
				return nil, &RuntimeError{Msg: "padLeft: fill must be a single character"}
			}
			s, err := CheckStr("padLeft", strV)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			padRunes := []rune(pad)
			for len(runes) < width {
				runes = append(padRunes, runes...)
			}
			return VString{V: string(runes)}, nil
		}),
		"padRight": curried3("padRight", func(widthV, padV, strV Value) (Value, error) {
			width, err := AsInt(widthV)
			if err != nil {
				return nil, err
			}
			pad, err := CheckStr("padRight", padV)
			if err != nil {
				return nil, err
			}
			if utf8.RuneCountInString(pad) != 1 {
				return nil, &RuntimeError{Msg: "padRight: fill must be a single character"}
			}
			s, err := CheckStr("padRight", strV)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			padRune := []rune(pad)[0]
			for len(runes) < width {
				runes = append(runes, padRune)
			}
			return VString{V: string(runes)}, nil
		}),
		"words": makeBuiltin("words", func(v Value) (Value, error) {
			s, err := CheckStr("words", v)
			if err != nil {
				return nil, err
			}
			parts := strings.Fields(s)
			items := make([]Value, len(parts))
			for i, p := range parts {
				items[i] = VString{V: p}
			}
			return VList{Items: items}, nil
		}),
		"lines": makeBuiltin("lines", func(v Value) (Value, error) {
			s, err := CheckStr("lines", v)
			if err != nil {
				return nil, err
			}
			if s == "" {
				return VList{Items: nil}, nil
			}
			s = strings.ReplaceAll(s, "\r\n", "\n")
			parts := strings.Split(s, "\n")
			if len(parts) > 0 && parts[len(parts)-1] == "" {
				parts = parts[:len(parts)-1]
			}
			items := make([]Value, len(parts))
			for i, p := range parts {
				items[i] = VString{V: p}
			}
			return VList{Items: items}, nil
		}),
		"charCode": makeBuiltin("charCode", func(v Value) (Value, error) {
			s, err := CheckStr("charCode", v)
			if err != nil {
				return nil, err
			}
			if s == "" {
				return nil, &RuntimeError{Msg: "charCode: empty string"}
			}
			r, _ := utf8.DecodeRuneInString(s)
			return VInt{V: int(r)}, nil
		}),
		"fromCharCode": makeBuiltin("fromCharCode", func(v Value) (Value, error) {
			i, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			if i < 0 || i > 0x10FFFF {
				return nil, &RuntimeError{Msg: fmt.Sprintf("fromCharCode: invalid code point %d", i)}
			}
			return VString{V: string(rune(i))}, nil
		}),
		"parseInt": makeBuiltin("parseInt", func(v Value) (Value, error) {
			s, err := CheckStr("parseInt", v)
			if err != nil {
				return nil, err
			}
			s = strings.TrimSpace(s)
			i, parseErr := strconv.Atoi(s)
			if parseErr != nil {
				return VCtor{Name: "Nothing", Args: nil}, nil
			}
			return VCtor{Name: "Just", Args: []Value{VInt{V: i}}}, nil
		}),
		"parseFloat": makeBuiltin("parseFloat", func(v Value) (Value, error) {
			s, err := CheckStr("parseFloat", v)
			if err != nil {
				return nil, err
			}
			s = strings.TrimSpace(s)
			f, parseErr := strconv.ParseFloat(s, 64)
			if parseErr != nil {
				return VCtor{Name: "Nothing", Args: nil}, nil
			}
			return VCtor{Name: "Just", Args: []Value{VFloat{V: f}}}, nil
		}),
		"reverse": makeBuiltin("reverse", func(v Value) (Value, error) {
			s, err := CheckStr("reverse", v)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
				runes[i], runes[j] = runes[j], runes[i]
			}
			return VString{V: string(runes)}, nil
		}),
		"toList": makeBuiltin("toList", func(v Value) (Value, error) {
			s, err := CheckStr("toList", v)
			if err != nil {
				return nil, err
			}
			runes := []rune(s)
			items := make([]Value, len(runes))
			for i, r := range runes {
				items[i] = VString{V: string(r)}
			}
			return VList{Items: items}, nil
		}),
		"fromList": makeBuiltin("fromList", func(v Value) (Value, error) {
			lst, ok := v.(VList)
			if !ok {
				return nil, &RuntimeError{Msg: "fromList: expected list"}
			}
			var buf strings.Builder
			for _, item := range lst.Items {
				s, err := CheckStr("fromList", item)
				if err != nil {
					return nil, err
				}
				buf.WriteString(s)
			}
			return VString{V: buf.String()}, nil
		}),
		"trimLeft": makeBuiltin("trimLeft", func(v Value) (Value, error) {
			s, err := CheckStr("trimLeft", v)
			if err != nil {
				return nil, err
			}
			return VString{V: strings.TrimLeft(s, " \t\n\r")}, nil
		}),
		"trimRight": makeBuiltin("trimRight", func(v Value) (Value, error) {
			s, err := CheckStr("trimRight", v)
			if err != nil {
				return nil, err
			}
			return VString{V: strings.TrimRight(s, " \t\n\r")}, nil
		}),
	}
}

// ListBuiltins returns list-related builtins.
func ListBuiltins() map[string]Value {
	return map[string]Value{
		"sortWith": curried2("sortWith", func(cmpFn, lstV Value) (Value, error) {
			lst, ok := lstV.(VList)
			if !ok {
				return nil, &RuntimeError{Msg: "sortWith: expected list"}
			}
			// Copy the slice so we don't mutate the original
			items := make([]Value, len(lst.Items))
			copy(items, lst.Items)
			var sortErr error
			sort.SliceStable(items, func(i, j int) bool {
				if sortErr != nil {
					return false
				}
				// Apply cmpFn to items[i] then items[j]
				partial, err := ApplyValue(cmpFn, items[i])
				if err != nil {
					sortErr = err
					return false
				}
				result, err := ApplyValue(partial, items[j])
				if err != nil {
					sortErr = err
					return false
				}
				// Result should be an Ordering: LT, EQ, GT
				ctor, ok := result.(VCtor)
				if !ok {
					sortErr = &RuntimeError{Msg: "sortWith: comparison must return Ordering"}
					return false
				}
				return ctor.Name == "LT"
			})
			if sortErr != nil {
				return nil, sortErr
			}
			return VList{Items: items}, nil
		}),
	}
}

// EnvBuiltins returns environment-related builtins.
func EnvBuiltins(programArgs []string) map[string]Value {
	argValues := make([]Value, len(programArgs))
	for i, a := range programArgs {
		argValues[i] = VString{V: a}
	}
	return map[string]Value{
		"getEnv": makeBuiltin("getEnv", func(v Value) (Value, error) {
			name, err := CheckStr("getEnv", v)
			if err != nil {
				return nil, err
			}
			val, ok := os.LookupEnv(name)
			if !ok {
				return VCtor{Name: "Nothing", Args: nil}, nil
			}
			return VCtor{Name: "Just", Args: []Value{VString{V: val}}}, nil
		}),
		"getEnvOr": curried2("getEnvOr", func(nameV, defaultV Value) (Value, error) {
			name, err := CheckStr("getEnvOr", nameV)
			if err != nil {
				return nil, err
			}
			def, err := CheckStr("getEnvOr", defaultV)
			if err != nil {
				return nil, err
			}
			val, ok := os.LookupEnv(name)
			if !ok {
				return VString{V: def}, nil
			}
			return VString{V: val}, nil
		}),
		"args": VList{Items: argValues},
	}
}

// JsonBuiltins returns JSON-related builtins.
func JsonBuiltins() map[string]Value {
	return map[string]Value{
		"jsonParse": makeBuiltin("jsonParse", func(v Value) (Value, error) {
			s, err := CheckStr("jsonParse", v)
			if err != nil {
				return nil, err
			}
			var pyVal interface{}
			if jsonErr := json.Unmarshal([]byte(s), &pyVal); jsonErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: jsonErr.Error()}}}, nil
			}
			result, convErr := jsonValToRex(pyVal)
			if convErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: convErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{result}}, nil
		}),
	}
}

func jsonValToRex(v interface{}) (Value, error) {
	if v == nil {
		return VCtor{Name: "JNull", Args: nil}, nil
	}
	switch val := v.(type) {
	case bool:
		return VCtor{Name: "JBool", Args: []Value{VBool{V: val}}}, nil
	case float64:
		return VCtor{Name: "JNum", Args: []Value{VFloat{V: val}}}, nil
	case string:
		return VCtor{Name: "JStr", Args: []Value{VString{V: val}}}, nil
	case []interface{}:
		items := make([]Value, len(val))
		for i, elem := range val {
			item, err := jsonValToRex(elem)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return VCtor{Name: "JArr", Args: []Value{VList{Items: items}}}, nil
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]Value, len(keys))
		for i, k := range keys {
			item, err := jsonValToRex(val[k])
			if err != nil {
				return nil, err
			}
			pairs[i] = VTuple{Items: []Value{VString{V: k}, item}}
		}
		return VCtor{Name: "JObj", Args: []Value{VList{Items: pairs}}}, nil
	}
	return nil, &RuntimeError{Msg: fmt.Sprintf("jsonParse: unexpected type %T", v)}
}

// ---------------------------------------------------------------------------
// Process / actor builtins
// ---------------------------------------------------------------------------

func makeReceiveBuiltin(mb *Mailbox) Value {
	return makeBuiltin("receive", func(_ Value) (Value, error) {
		return mb.Receive(), nil
	})
}

// ProcessBuiltins returns the five process primitives bound to the given process pid.
func ProcessBuiltins(selfPid VPid) map[string]Value {
	return map[string]Value{
		"self":    selfPid,
		"receive": makeReceiveBuiltin(selfPid.Mailbox),
		"send": curried2("send", func(pidV, msgV Value) (Value, error) {
			pid, ok := pidV.(VPid)
			if !ok {
				return nil, runtimeErr("send: expected Pid, got %s", ValueToString(pidV))
			}
			pid.Mailbox.Send(msgV)
			return VUnit{}, nil
		}),
		"spawn": makeBuiltin("spawn", func(fnV Value) (Value, error) {
			cl, ok := fnV.(VClosure)
			if !ok {
				return nil, runtimeErr("spawn: expected closure, got %s", ValueToString(fnV))
			}
			mb := newMailbox()
			pid := VPid{Mailbox: mb, ID: mb.id}
			procEnv := cl.Env.
				Extend("self", pid).
				Extend("receive", makeReceiveBuiltin(mb)).
				Extend(cl.Param, VUnit{})
			go func() {
				Eval(procEnv, cl.Body) //nolint:errcheck
			}()
			return pid, nil
		}),
		"call": curried2("call", func(pidV, makeMsgV Value) (Value, error) {
			pid, ok := pidV.(VPid)
			if !ok {
				return nil, runtimeErr("call: expected Pid, got %s", ValueToString(pidV))
			}
			msg, err := ApplyValue(makeMsgV, selfPid)
			if err != nil {
				return nil, err
			}
			pid.Mailbox.Send(msg)
			return selfPid.Mailbox.Receive(), nil
		}),
	}
}

// BuiltinsForModule returns builtins for a stdlib module: CoreBuiltins + module-specific ones.
func BuiltinsForModule(name string, programArgs []string) map[string]Value {
	result := make(map[string]Value)
	for k, v := range CoreBuiltins() {
		result[k] = v
	}
	switch name {
	case "IO":
		for k, v := range IOBuiltins() {
			result[k] = v
		}
	case "Math":
		for k, v := range MathBuiltins() {
			result[k] = v
		}
	case "String":
		for k, v := range StringBuiltins() {
			result[k] = v
		}
	case "Env":
		for k, v := range EnvBuiltins(programArgs) {
			result[k] = v
		}
	case "List":
		for k, v := range ListBuiltins() {
			result[k] = v
		}
	case "Result":
		result["try"] = makeBuiltin("try", func(fnV Value) (Value, error) {
			val, err := ApplyValue(fnV, VUnit{})
			if err != nil {
				if re, ok := err.(*RuntimeError); ok {
					switch re.Msg {
					case "division by zero":
						return VCtor{Name: "Err", Args: []Value{VCtor{Name: "DivisionByZero"}}}, nil
					case "modulo by zero":
						return VCtor{Name: "Err", Args: []Value{VCtor{Name: "ModuloByZero"}}}, nil
					}
				}
				return nil, err
			}
			return VCtor{Name: "Ok", Args: []Value{val}}, nil
		})
	case "Json":
		for k, v := range JsonBuiltins() {
			result[k] = v
		}
	case "Process":
		mb := newMailbox()
		pid := VPid{Mailbox: mb, ID: mb.id}
		for k, v := range ProcessBuiltins(pid) {
			result[k] = v
		}
	case "Parallel":
		for k, v := range ParallelBuiltins() {
			result[k] = v
		}
	case "Net":
		for k, v := range NetBuiltins() {
			result[k] = v
		}
	case "Random":
		for k, v := range RandomBuiltins() {
			result[k] = v
		}
	case "Bitwise":
		for k, v := range BitwiseBuiltins() {
			result[k] = v
		}
	case "DateTime":
		for k, v := range DateTimeBuiltins() {
			result[k] = v
		}
	}
	return result
}

// ParallelBuiltins returns builtins for the Parallel module.
func ParallelBuiltins() map[string]Value {
	return map[string]Value{
		"numCPU": VInt{V: runtime.NumCPU()},
	}
}

// BitwiseBuiltins returns builtins for the Bitwise module.
func BitwiseBuiltins() map[string]Value {
	return map[string]Value{
		"bitAnd": curried2("bitAnd", func(a, b Value) (Value, error) {
			x, err := AsInt(a)
			if err != nil {
				return nil, err
			}
			y, err := AsInt(b)
			if err != nil {
				return nil, err
			}
			return VInt{V: x & y}, nil
		}),
		"bitOr": curried2("bitOr", func(a, b Value) (Value, error) {
			x, err := AsInt(a)
			if err != nil {
				return nil, err
			}
			y, err := AsInt(b)
			if err != nil {
				return nil, err
			}
			return VInt{V: x | y}, nil
		}),
		"bitXor": curried2("bitXor", func(a, b Value) (Value, error) {
			x, err := AsInt(a)
			if err != nil {
				return nil, err
			}
			y, err := AsInt(b)
			if err != nil {
				return nil, err
			}
			return VInt{V: x ^ y}, nil
		}),
		"bitNot": makeBuiltin("bitNot", func(v Value) (Value, error) {
			x, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			return VInt{V: ^x}, nil
		}),
		"shiftLeft": curried2("shiftLeft", func(a, b Value) (Value, error) {
			x, err := AsInt(a)
			if err != nil {
				return nil, err
			}
			n, err := AsInt(b)
			if err != nil {
				return nil, err
			}
			return VInt{V: x << uint(n)}, nil
		}),
		"shiftRight": curried2("shiftRight", func(a, b Value) (Value, error) {
			x, err := AsInt(a)
			if err != nil {
				return nil, err
			}
			n, err := AsInt(b)
			if err != nil {
				return nil, err
			}
			return VInt{V: x >> uint(n)}, nil
		}),
	}
}

// RandomBuiltins returns builtins for the Random module.
func RandomBuiltins() map[string]Value {
	return map[string]Value{
		"systemSeed": makeBuiltin("systemSeed", func(_ Value) (Value, error) {
			n := rand.IntN(2147483646) + 1 // [1, 2147483646]
			return VInt{V: n}, nil
		}),
	}
}

// DateTimeBuiltins returns builtins for the DateTime module.
func DateTimeBuiltins() map[string]Value {
	return map[string]Value{
		// dateTimeNow : () -> Int
		"dateTimeNow": makeBuiltin("dateTimeNow", func(_ Value) (Value, error) {
			return VInt{V: int(time.Now().UnixMilli())}, nil
		}),
		// dateTimeUtcOffset : () -> Int
		"dateTimeUtcOffset": makeBuiltin("dateTimeUtcOffset", func(_ Value) (Value, error) {
			_, offset := time.Now().Zone()
			return VInt{V: offset / 60}, nil
		}),
	}
}

// NetBuiltins returns TCP networking builtins.
func NetBuiltins() map[string]Value {
	return map[string]Value{
		"tcpListen": makeBuiltin("tcpListen", func(v Value) (Value, error) {
			port, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			ln, netErr := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if netErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: netErr.Error()}}}, nil
			}
			actualPort := ln.Addr().(*net.TCPAddr).Port
			return VCtor{Name: "Ok", Args: []Value{VTuple{Items: []Value{VListener{L: ln}, VInt{V: actualPort}}}}}, nil
		}),
		"tcpAccept": makeBuiltin("tcpAccept", func(v Value) (Value, error) {
			ln, ok := v.(VListener)
			if !ok {
				return nil, runtimeErr("tcpAccept: expected Listener, got %s", ValueToString(v))
			}
			conn, netErr := ln.L.Accept()
			if netErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: netErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VConn{C: conn}}}, nil
		}),
		"tcpConnect": curried2("tcpConnect", func(hostV, portV Value) (Value, error) {
			host, err := CheckStr("tcpConnect", hostV)
			if err != nil {
				return nil, err
			}
			port, err := AsInt(portV)
			if err != nil {
				return nil, err
			}
			conn, netErr := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
			if netErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: netErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VConn{C: conn}}}, nil
		}),
		"tcpRead": makeBuiltin("tcpRead", func(v Value) (Value, error) {
			c, ok := v.(VConn)
			if !ok {
				return nil, runtimeErr("tcpRead: expected Conn, got %s", ValueToString(v))
			}
			buf := make([]byte, 4096)
			n, readErr := c.C.Read(buf)
			if readErr != nil {
				if readErr == io.EOF {
					return VCtor{Name: "Err", Args: []Value{VString{V: "EOF"}}}, nil
				}
				return VCtor{Name: "Err", Args: []Value{VString{V: readErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VString{V: string(buf[:n])}}}, nil
		}),
		"tcpWrite": curried2("tcpWrite", func(connV, dataV Value) (Value, error) {
			c, ok := connV.(VConn)
			if !ok {
				return nil, runtimeErr("tcpWrite: expected Conn, got %s", ValueToString(connV))
			}
			data, err := CheckStr("tcpWrite", dataV)
			if err != nil {
				return nil, err
			}
			_, writeErr := c.C.Write([]byte(data))
			if writeErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: writeErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
		"tcpClose": makeBuiltin("tcpClose", func(v Value) (Value, error) {
			c, ok := v.(VConn)
			if !ok {
				return nil, runtimeErr("tcpClose: expected Conn, got %s", ValueToString(v))
			}
			if closeErr := c.C.Close(); closeErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: closeErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
		"tcpCloseListener": makeBuiltin("tcpCloseListener", func(v Value) (Value, error) {
			ln, ok := v.(VListener)
			if !ok {
				return nil, runtimeErr("tcpCloseListener: expected Listener, got %s", ValueToString(v))
			}
			if closeErr := ln.L.Close(); closeErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: closeErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
	}
}

func floatToStr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") &&
		!strings.Contains(s, "E") && !strings.Contains(s, "n") && !strings.Contains(s, "N") {
		s += ".0"
	}
	return s
}
