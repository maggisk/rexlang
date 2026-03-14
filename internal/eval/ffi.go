package eval

import (
	"fmt"
	"reflect"

	"github.com/maggisk/rexlang/internal/stdlib/rexfiles"
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()

// wrapGoFunction wraps a Go function (or constant) as an eval.Value.
// For functions, it generates curried wrappers based on the Go signature.
// Constants (non-function values) are converted directly.
func wrapGoFunction(name string, fn any) Value {
	rv := reflect.ValueOf(fn)
	rt := rv.Type()

	if rt.Kind() != reflect.Func {
		return goToValue(rv)
	}

	numIn := rt.NumIn()
	hasError := rt.NumOut() > 0 && rt.Out(rt.NumOut()-1).Implements(errorType)

	callFn := func(args []Value) (val Value, err error) {
		defer func() {
			if r := recover(); r != nil {
				switch msg := r.(type) {
				case string:
					err = &RuntimeError{Msg: msg}
				case error:
					err = &RuntimeError{Msg: msg.Error()}
				default:
					panic(r)
				}
			}
		}()

		goArgs := make([]reflect.Value, len(args))
		for i, arg := range args {
			goArgs[i] = valueToGoReflect(arg, rt.In(i))
		}
		results := rv.Call(goArgs)
		return convertGoResults(rt, results, hasError)
	}

	if numIn == 0 {
		return makeBuiltin(name, func(_ Value) (Value, error) {
			return callFn(nil)
		})
	}
	return curryN(name, numIn, callFn)
}

// curryN builds a curried chain of builtins for any arity.
func curryN(name string, arity int, callFn func([]Value) (Value, error)) Value {
	var build func(collected []Value, remaining int) Value
	build = func(collected []Value, remaining int) Value {
		suffix := ""
		if len(collected) > 0 {
			suffix = fmt.Sprintf("$%d", len(collected))
		}
		return makeBuiltin(name+suffix, func(v Value) (Value, error) {
			args := append(collected[:len(collected):len(collected)], v)
			if remaining == 1 {
				return callFn(args)
			}
			return build(args, remaining-1), nil
		})
	}
	return build(nil, arity)
}

func valueToGoReflect(v Value, t reflect.Type) reflect.Value {
	switch t.Kind() {
	case reflect.String:
		return reflect.ValueOf(v.(VString).V)
	case reflect.Int:
		return reflect.ValueOf(v.(VInt).V)
	case reflect.Float64:
		return reflect.ValueOf(v.(VFloat).V)
	case reflect.Bool:
		return reflect.ValueOf(v.(VBool).V)
	case reflect.Interface:
		return reflect.ValueOf(v)
	case reflect.Slice:
		lst := v.(VList)
		elemType := t.Elem()
		slice := reflect.MakeSlice(t, len(lst.Items), len(lst.Items))
		for i, item := range lst.Items {
			slice.Index(i).Set(valueToGoReflect(item, elemType))
		}
		return slice
	}
	panic(fmt.Sprintf("valueToGoReflect: unsupported type %s", t))
}

func goToValue(v reflect.Value) Value {
	if !v.IsValid() {
		return VUnit{}
	}

	t := v.Type()

	if t.Kind() == reflect.Ptr {
		if v.IsNil() {
			return VCtor{Name: "Nothing", Args: nil}
		}
		return VCtor{Name: "Just", Args: []Value{goToValue(v.Elem())}}
	}

	switch t.Kind() {
	case reflect.String:
		return VString{V: v.String()}
	case reflect.Int:
		return VInt{V: int(v.Int())}
	case reflect.Float64:
		return VFloat{V: v.Float()}
	case reflect.Bool:
		return VBool{V: v.Bool()}
	case reflect.Slice:
		if v.IsNil() {
			return VList{Items: nil}
		}
		items := make([]Value, v.Len())
		for i := 0; i < v.Len(); i++ {
			items[i] = goToValue(v.Index(i))
		}
		return VList{Items: items}
	case reflect.Interface:
		if v.IsNil() {
			return VUnit{}
		}
		if val, ok := v.Interface().(Value); ok {
			return val
		}
		return goToValue(v.Elem())
	}
	panic(fmt.Sprintf("goToValue: unsupported type %s", t))
}

func convertGoResults(ft reflect.Type, results []reflect.Value, hasError bool) (Value, error) {
	numOut := ft.NumOut()

	if numOut == 0 {
		return VUnit{}, nil
	}

	if hasError {
		errVal := results[numOut-1]
		if !errVal.IsNil() {
			errStr := errVal.Interface().(error).Error()
			return VCtor{Name: "Err", Args: []Value{VString{V: errStr}}}, nil
		}
		if numOut == 1 {
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}
		if numOut == 2 {
			return VCtor{Name: "Ok", Args: []Value{goToValue(results[0])}}, nil
		}
		items := make([]Value, numOut-1)
		for i := 0; i < numOut-1; i++ {
			items[i] = goToValue(results[i])
		}
		return VCtor{Name: "Ok", Args: []Value{VTuple{Items: items}}}, nil
	}

	if numOut == 1 {
		return goToValue(results[0]), nil
	}

	items := make([]Value, numOut)
	for i := 0; i < numOut; i++ {
		items[i] = goToValue(results[i])
	}
	return VTuple{Items: items}, nil
}

// loadRegisteredBuiltins loads auto-discovered builtins from rexfiles.Registry.
func loadRegisteredBuiltins(moduleName string, dest map[string]Value) {
	fns, ok := rexfiles.Registry[moduleName]
	if !ok {
		return
	}
	for fnName, goFn := range fns {
		dest[fnName] = wrapGoFunction(fnName, goFn)
	}
}
