package eval

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/stdlib"
)

// ---------------------------------------------------------------------------
// ApplyValue — apply a function value to an argument (used by call builtin)
// ---------------------------------------------------------------------------

// ApplyValue applies fn to arg, handling closures, builtins, and constructors.
func ApplyValue(fn, arg Value) (Value, error) {
	switch f := fn.(type) {
	case VClosure:
		return Eval(f.Env.Extend(f.Param, arg), f.Body)
	case VBuiltin:
		return f.Fn(arg)
	case VCtorFn:
		if f.Remaining == 1 {
			combined := make([]Value, 1+len(f.AccArgs))
			combined[0] = arg
			copy(combined[1:], f.AccArgs)
			for i, j := 0, len(combined)-1; i < j; i, j = i+1, j-1 {
				combined[i], combined[j] = combined[j], combined[i]
			}
			return VCtor{Name: f.Name, Args: combined}, nil
		}
		return VCtorFn{
			Name:      f.Name,
			Remaining: f.Remaining - 1,
			AccArgs:   append([]Value{arg}, f.AccArgs...),
		}, nil
	}
	return nil, runtimeErr("cannot apply %s as a function", ValueToString(fn))
}

// ---------------------------------------------------------------------------
// Binop evaluation
// ---------------------------------------------------------------------------

func evalBinop(op string, l, r Value) (Value, error) {
	sym := ast.BinopSym[op]
	if sym == "" {
		sym = op
	}
	switch op {
	case "Add":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				return VInt{V: a.V + b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VFloat{V: a.V + b.V}, nil
			}
		}
	case "Sub":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				return VInt{V: a.V - b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VFloat{V: a.V - b.V}, nil
			}
		}
	case "Mul":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				return VInt{V: a.V * b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VFloat{V: a.V * b.V}, nil
			}
		}
	case "Div":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				if b.V == 0 {
					return nil, runtimeErr("division by zero")
				}
				return VInt{V: a.V / b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VFloat{V: a.V / b.V}, nil
			}
		}
	case "Mod":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				if b.V == 0 {
					return nil, runtimeErr("modulo by zero")
				}
				return VInt{V: a.V % b.V}, nil
			}
		}
	case "Concat":
		if a, ok := l.(VString); ok {
			if b, ok := r.(VString); ok {
				return VString{V: a.V + b.V}, nil
			}
		}
	case "Lt":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				return VBool{V: a.V < b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VBool{V: a.V < b.V}, nil
			}
		}
		if a, ok := l.(VString); ok {
			if b, ok := r.(VString); ok {
				return VBool{V: a.V < b.V}, nil
			}
		}
		if a, ok := l.(VBool); ok {
			if b, ok := r.(VBool); ok {
				return VBool{V: !a.V && b.V}, nil
			}
		}
	case "Gt":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				return VBool{V: a.V > b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VBool{V: a.V > b.V}, nil
			}
		}
		if a, ok := l.(VString); ok {
			if b, ok := r.(VString); ok {
				return VBool{V: a.V > b.V}, nil
			}
		}
		if a, ok := l.(VBool); ok {
			if b, ok := r.(VBool); ok {
				return VBool{V: a.V && !b.V}, nil
			}
		}
	case "Leq":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				return VBool{V: a.V <= b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VBool{V: a.V <= b.V}, nil
			}
		}
		if a, ok := l.(VString); ok {
			if b, ok := r.(VString); ok {
				return VBool{V: a.V <= b.V}, nil
			}
		}
		if a, ok := l.(VBool); ok {
			if b, ok := r.(VBool); ok {
				return VBool{V: !a.V || b.V}, nil
			}
		}
	case "Geq":
		if a, ok := l.(VInt); ok {
			if b, ok := r.(VInt); ok {
				return VBool{V: a.V >= b.V}, nil
			}
		}
		if a, ok := l.(VFloat); ok {
			if b, ok := r.(VFloat); ok {
				return VBool{V: a.V >= b.V}, nil
			}
		}
		if a, ok := l.(VString); ok {
			if b, ok := r.(VString); ok {
				return VBool{V: a.V >= b.V}, nil
			}
		}
		if a, ok := l.(VBool); ok {
			if b, ok := r.(VBool); ok {
				return VBool{V: a.V || !b.V}, nil
			}
		}
	case "Eq":
		return VBool{V: StructuralEq(l, r)}, nil
	case "Neq":
		return VBool{V: !StructuralEq(l, r)}, nil
	case "And":
		if a, ok := l.(VBool); ok {
			if b, ok := r.(VBool); ok {
				return VBool{V: a.V && b.V}, nil
			}
		}
	case "Or":
		if a, ok := l.(VBool); ok {
			if b, ok := r.(VBool); ok {
				return VBool{V: a.V || b.V}, nil
			}
		}
	case "Cons":
		if lst, ok := r.(VList); ok {
			newItems := make([]Value, 1+len(lst.Items))
			newItems[0] = l
			copy(newItems[1:], lst.Items)
			return VList{Items: newItems}, nil
		}
	}
	return nil, runtimeErr("type error: %s %s %s", ValueToString(l), sym, ValueToString(r))
}

// ---------------------------------------------------------------------------
// Pattern matching
// ---------------------------------------------------------------------------

func matchPattern(pat ast.Pattern, value Value) (map[string]Value, bool) {
	switch p := pat.(type) {
	case ast.PWild:
		return map[string]Value{}, true
	case ast.PUnit:
		if _, ok := value.(VUnit); ok {
			return map[string]Value{}, true
		}
	case ast.PVar:
		return map[string]Value{p.Name: value}, true
	case ast.PInt:
		if v, ok := value.(VInt); ok && p.Value == v.V {
			return map[string]Value{}, true
		}
	case ast.PFloat:
		if v, ok := value.(VFloat); ok && p.Value == v.V {
			return map[string]Value{}, true
		}
	case ast.PString:
		if v, ok := value.(VString); ok && p.Value == v.V {
			return map[string]Value{}, true
		}
	case ast.PBool:
		if v, ok := value.(VBool); ok && p.Value == v.V {
			return map[string]Value{}, true
		}
	case ast.PCtor:
		if v, ok := value.(VCtor); ok && p.Name == v.Name && len(p.Args) == len(v.Args) {
			bindings := map[string]Value{}
			for i, argPat := range p.Args {
				b, ok := matchPattern(argPat, v.Args[i])
				if !ok {
					return nil, false
				}
				for k, val := range b {
					bindings[k] = val
				}
			}
			return bindings, true
		}
	case ast.PNil:
		if v, ok := value.(VList); ok && len(v.Items) == 0 {
			return map[string]Value{}, true
		}
	case ast.PCons:
		if v, ok := value.(VList); ok && len(v.Items) > 0 {
			hb, ok := matchPattern(p.Head, v.Items[0])
			if !ok {
				return nil, false
			}
			tb, ok := matchPattern(p.Tail, VList{Items: v.Items[1:]})
			if !ok {
				return nil, false
			}
			bindings := map[string]Value{}
			for k, val := range hb {
				bindings[k] = val
			}
			for k, val := range tb {
				bindings[k] = val
			}
			return bindings, true
		}
	case ast.PTuple:
		if v, ok := value.(VTuple); ok && len(p.Pats) == len(v.Items) {
			bindings := map[string]Value{}
			for i, pp := range p.Pats {
				b, ok := matchPattern(pp, v.Items[i])
				if !ok {
					return nil, false
				}
				for k, val := range b {
					bindings[k] = val
				}
			}
			return bindings, true
		}
	case ast.PRecord:
		if v, ok := value.(VRecord); ok && p.TypeName == v.TypeName {
			bindings := map[string]Value{}
			for _, f := range p.Fields {
				fv, ok := v.Fields[f.Name]
				if !ok {
					return nil, false
				}
				b, ok := matchPattern(f.Pat, fv)
				if !ok {
					return nil, false
				}
				for k, val := range b {
					bindings[k] = val
				}
			}
			return bindings, true
		}
	}
	return nil, false
}

// ---------------------------------------------------------------------------
// Show dispatch for string interpolation
// ---------------------------------------------------------------------------

// showValue converts a value to string using the Show trait when available.
func showValue(env Env, v Value) (string, error) {
	// Short-circuit for strings
	if s, ok := v.(VString); ok {
		return s.V, nil
	}
	// Try Show trait dispatch
	typeName, err := RuntimeTypeName(v)
	if err == nil {
		if inst, ok := env["__instances__"]; ok {
			if vi, ok := inst.(VInstances); ok {
				key := "Show:" + typeName + ":show"
				if implFn, ok := vi.M[key]; ok {
					result, err := ApplyValue(implFn, v)
					if err != nil {
						return "", err
					}
					if s, ok := result.(VString); ok {
						return s.V, nil
					}
				}
			}
		}
	}
	// Fallback to ValueToString (for types without Show instance)
	return Display(v), nil
}

// ---------------------------------------------------------------------------
// Record update helpers
// ---------------------------------------------------------------------------

func cloneRecord(r VRecord) VRecord {
	fields := make(map[string]Value, len(r.Fields))
	for k, v := range r.Fields {
		fields[k] = v
	}
	return VRecord{TypeName: r.TypeName, Fields: fields}
}

func applyRecordUpdate(rec VRecord, path []string, value Value) (VRecord, error) {
	if len(path) == 1 {
		rec.Fields[path[0]] = value
		return rec, nil
	}
	nested, ok := rec.Fields[path[0]]
	if !ok {
		return rec, runtimeErr("record '%s' has no field '%s'", rec.TypeName, path[0])
	}
	nestedRec, ok := nested.(VRecord)
	if !ok {
		return rec, runtimeErr("field '%s' is not a record", path[0])
	}
	updated := cloneRecord(nestedRec)
	updated, err := applyRecordUpdate(updated, path[1:], value)
	if err != nil {
		return rec, err
	}
	rec.Fields[path[0]] = updated
	return rec, nil
}

// ---------------------------------------------------------------------------
// Evaluator
// ---------------------------------------------------------------------------

// Eval evaluates expr in env with a trampoline loop.
func Eval(env Env, expr ast.Expr) (Value, error) {
	for {
		switch e := expr.(type) {
		case ast.IntLit:
			return VInt{V: e.Value}, nil
		case ast.FloatLit:
			return VFloat{V: e.Value}, nil
		case ast.StringLit:
			return VString{V: e.Value}, nil
		case ast.StringInterp:
			var buf strings.Builder
			for _, part := range e.Parts {
				v, err := Eval(env, part)
				if err != nil {
					return nil, err
				}
				s, err := showValue(env, v)
				if err != nil {
					return nil, err
				}
				buf.WriteString(s)
			}
			return VString{V: buf.String()}, nil
		case ast.BoolLit:
			return VBool{V: e.Value}, nil
		case ast.UnitLit:
			return VUnit{}, nil

		case ast.Var:
			v, ok := env[e.Name]
			if !ok {
				return nil, runtimeErr("unbound variable: %s", e.Name)
			}
			return v, nil

		case ast.UnaryMinus:
			v, err := Eval(env, e.Expr)
			if err != nil {
				return nil, err
			}
			switch val := v.(type) {
			case VInt:
				return VInt{V: -val.V}, nil
			case VFloat:
				return VFloat{V: -val.V}, nil
			}
			return nil, runtimeErr("type error: unary minus on %s", ValueToString(v))

		case ast.Binop:
			if e.Op == "And" {
				l, err := Eval(env, e.Left)
				if err != nil {
					return nil, err
				}
				b, err := AsBool(l)
				if err != nil {
					return nil, err
				}
				if !b {
					return VBool{V: false}, nil
				}
				expr = e.Right
				continue
			}
			if e.Op == "Or" {
				l, err := Eval(env, e.Left)
				if err != nil {
					return nil, err
				}
				b, err := AsBool(l)
				if err != nil {
					return nil, err
				}
				if b {
					return VBool{V: true}, nil
				}
				expr = e.Right
				continue
			}
			l, err := Eval(env, e.Left)
			if err != nil {
				return nil, err
			}
			r, err := Eval(env, e.Right)
			if err != nil {
				return nil, err
			}
			return evalBinop(e.Op, l, r)

		case ast.If:
			cond, err := Eval(env, e.Cond)
			if err != nil {
				return nil, err
			}
			b, err := AsBool(cond)
			if err != nil {
				return nil, err
			}
			if b {
				expr = e.ThenExpr
			} else {
				expr = e.ElseExpr
			}

		case ast.Fun:
			return VClosure{Param: e.Param, Body: e.Body, Env: env.Clone()}, nil

		case ast.App:
			funcV, err := Eval(env, e.Func)
			if err != nil {
				return nil, err
			}
			arg, err := Eval(env, e.Arg)
			if err != nil {
				return nil, err
			}
			switch fn := funcV.(type) {
			case VClosure:
				env = fn.Env.Extend(fn.Param, arg)
				expr = fn.Body
			case VCtorFn:
				if fn.Remaining == 1 {
					// Reverse: AccArgs was built with prepend ([new] + acc)
					combined := make([]Value, 1+len(fn.AccArgs))
					combined[0] = arg
					copy(combined[1:], fn.AccArgs)
					// Reverse to get correct order
					for i, j := 0, len(combined)-1; i < j; i, j = i+1, j-1 {
						combined[i], combined[j] = combined[j], combined[i]
					}
					return VCtor{Name: fn.Name, Args: combined}, nil
				}
				return VCtorFn{
					Name:      fn.Name,
					Remaining: fn.Remaining - 1,
					AccArgs:   append([]Value{arg}, fn.AccArgs...),
				}, nil
			case VBuiltin:
				return fn.Fn(arg)
			case VTraitMethod:
				typeName, err := RuntimeTypeName(arg)
				if err != nil {
					return nil, err
				}
				var instances map[string]Value
				if iv, ok := env["__instances__"]; ok {
					if vi, ok := iv.(VInstances); ok {
						instances = vi.M
					}
				}
				key := fn.TraitName + ":" + typeName + ":" + fn.MethodName
				implFn, ok := instances[key]
				if !ok {
					return nil, runtimeErr("no %s instance for %s", fn.TraitName, typeName)
				}
				switch impl := implFn.(type) {
				case VClosure:
					env = impl.Env.Extend(impl.Param, arg)
					expr = impl.Body
				case VBuiltin:
					return impl.Fn(arg)
				default:
					return nil, runtimeErr("invalid trait impl for %s", key)
				}
			default:
				return nil, runtimeErr("cannot apply %s as a function", ValueToString(funcV))
			}

		case ast.Match:
			value, err := Eval(env, e.Scrutinee)
			if err != nil {
				return nil, err
			}
			matched := false
			for _, arm := range e.Arms {
				bindings, ok := matchPattern(arm.Pat, value)
				if ok {
					env = env.ExtendMany(bindings)
					expr = arm.Body
					matched = true
					break
				}
			}
			if !matched {
				return nil, runtimeErr("match failure: no pattern matched")
			}

		case ast.ListLit:
			items := make([]Value, len(e.Items))
			for i, item := range e.Items {
				v, err := Eval(env, item)
				if err != nil {
					return nil, err
				}
				items[i] = v
			}
			return VList{Items: items}, nil

		case ast.TupleLit:
			items := make([]Value, len(e.Items))
			for i, item := range e.Items {
				v, err := Eval(env, item)
				if err != nil {
					return nil, err
				}
				items[i] = v
			}
			return VTuple{Items: items}, nil

		case ast.LetPat:
			value, err := Eval(env, e.Body)
			if err != nil {
				return nil, err
			}
			bindings, ok := matchPattern(e.Pat, value)
			if !ok {
				return nil, runtimeErr("let pattern match failure")
			}
			if e.InExpr != nil {
				env = env.ExtendMany(bindings)
				expr = e.InExpr
			} else {
				return value, nil
			}

		case ast.RecordCreate:
			fields := make(map[string]Value, len(e.Fields))
			for _, f := range e.Fields {
				v, err := Eval(env, f.Value)
				if err != nil {
					return nil, err
				}
				fields[f.Name] = v
			}
			return VRecord{TypeName: e.TypeName, Fields: fields}, nil

		case ast.FieldAccess:
			v, err := Eval(env, e.Record)
			if err != nil {
				return nil, err
			}
			rec, ok := v.(VRecord)
			if !ok {
				return nil, runtimeErr("cannot access field '%s' on non-record value %s", e.Field, ValueToString(v))
			}
			fv, ok := rec.Fields[e.Field]
			if !ok {
				return nil, runtimeErr("record '%s' has no field '%s'", rec.TypeName, e.Field)
			}
			return fv, nil

		case ast.RecordUpdate:
			recVal, err := Eval(env, e.Record)
			if err != nil {
				return nil, err
			}
			rec, ok := recVal.(VRecord)
			if !ok {
				return nil, runtimeErr("record update requires a record, got %s", ValueToString(recVal))
			}
			result := cloneRecord(rec)
			for _, upd := range e.Updates {
				valV, err := Eval(env, upd.Value)
				if err != nil {
					return nil, err
				}
				result, err = applyRecordUpdate(result, upd.Path, valV)
				if err != nil {
					return nil, err
				}
			}
			return result, nil

		case ast.DotAccess:
			v := env[e.ModuleName]
			mod, ok := v.(VModule)
			if !ok {
				return nil, runtimeErr("'%s' is not a module", e.ModuleName)
			}
			field, ok := mod.Env[e.FieldName]
			if !ok {
				return nil, runtimeErr("module '%s' does not export '%s'", e.ModuleName, e.FieldName)
			}
			return field, nil

		case ast.TypeDecl:
			return VBool{V: false}, nil

		case ast.Let:
			var value Value
			if e.Recursive {
				bodyVal, err := Eval(env, e.Body)
				if err != nil {
					return nil, err
				}
				if cl, ok := bodyVal.(VClosure); ok {
					fixedEnv := cl.Env.Clone()
					closure := VClosure{Param: cl.Param, Body: cl.Body, Env: fixedEnv}
					fixedEnv[e.Name] = closure
					value = closure
				} else {
					value = bodyVal
				}
			} else {
				v, err := Eval(env, e.Body)
				if err != nil {
					return nil, err
				}
				value = v
			}
			if e.InExpr != nil {
				env = env.Extend(e.Name, value)
				expr = e.InExpr
			} else {
				return value, nil
			}

		case ast.LetRec:
			sharedEnv := env.Clone()
			raw := map[string]Value{}
			for _, b := range e.Bindings {
				val, err := Eval(env, b.Body)
				if err != nil {
					return nil, err
				}
				if cl, ok := val.(VClosure); ok {
					raw[b.Name] = VClosure{Param: cl.Param, Body: cl.Body, Env: sharedEnv}
				} else {
					raw[b.Name] = val
				}
			}
			for k, v := range raw {
				sharedEnv[k] = v
			}
			lastVal := raw[e.Bindings[len(e.Bindings)-1].Name]
			if e.InExpr != nil {
				env = sharedEnv
				expr = e.InExpr
			} else {
				return lastVal, nil
			}

		case ast.Assert:
			v, err := Eval(env, e.Expr)
			if err != nil {
				return nil, err
			}
			if b, ok := v.(VBool); ok && b.V {
				return VUnit{}, nil
			}
			// For == comparisons, show both sides
			if binop, ok := e.Expr.(ast.Binop); ok && binop.Op == "Eq" {
				lv, lerr := Eval(env, binop.Left)
				rv, rerr := Eval(env, binop.Right)
				if lerr == nil && rerr == nil {
					return nil, &AssertError{Msg: fmt.Sprintf(
						"assert failed at line %d\n  left:  %s\n  right: %s",
						e.Line, ValueToString(lv), ValueToString(rv))}
				}
			}
			return nil, &AssertError{Msg: fmt.Sprintf("assert failed at line %d", e.Line)}

		default:
			return nil, runtimeErr("unknown AST node: %T", expr)
		}
	}
}

// ---------------------------------------------------------------------------
// Module loading
// ---------------------------------------------------------------------------

type moduleResult struct {
	env     Env
	exports map[string]bool
}

var (
	evalModuleCache   = map[string]*moduleResult{}
	evalModuleCacheMu sync.Mutex
)

func loadModule(moduleName string, programArgs []string) (*moduleResult, error) {
	evalModuleCacheMu.Lock()
	if r, ok := evalModuleCache[moduleName]; ok {
		evalModuleCacheMu.Unlock()
		return r, nil
	}
	evalModuleCacheMu.Unlock()

	var name string
	if len(moduleName) > 4 && moduleName[:4] == "std:" {
		name = moduleName[4:]
	} else {
		return nil, runtimeErr("bare module name '%s': use 'std:%s' for stdlib", moduleName, moduleName)
	}

	src, err := stdlib.Source(name)
	if err != nil {
		return nil, runtimeErr("unknown module: %s", moduleName)
	}
	exprs, parseErr := parser.Parse(src)
	if parseErr != nil {
		return nil, parseErr
	}
	prelude, err := loadPreludeEval(programArgs)
	if err != nil {
		return nil, err
	}
	env := prelude.Clone()
	// Inject module-specific builtins
	for k, v := range BuiltinsForModule(name, programArgs) {
		env[k] = v
	}
	exports := map[string]bool{}
	for _, expr := range exprs {
		if ex, ok := expr.(ast.Export); ok {
			for _, n := range ex.Names {
				exports[n] = true
			}
		} else {
			// Collect export names from Exported flags
			switch e := expr.(type) {
			case ast.Let:
				if e.Exported && e.InExpr == nil {
					exports[e.Name] = true
				}
			case ast.LetRec:
				if e.Exported && e.InExpr == nil {
					for _, b := range e.Bindings {
						exports[b.Name] = true
					}
				}
			case ast.TypeDecl:
				if e.Exported {
					for _, ctor := range e.Ctors {
						exports[ctor.Name] = true
					}
				}
			case ast.TraitDecl:
				if e.Exported {
					for _, m := range e.Methods {
						exports[m.Name] = true
					}
				}
			}
			_, newEnv, err := EvalToplevel(env, expr, programArgs)
			if err != nil {
				return nil, err
			}
			env = newEnv
		}
	}
	for name := range exports {
		if _, ok := env[name]; !ok {
			return nil, runtimeErr("module '%s' exports undefined name(s): %s", moduleName, name)
		}
	}
	result := &moduleResult{env: env, exports: exports}
	evalModuleCacheMu.Lock()
	evalModuleCache[moduleName] = result
	evalModuleCacheMu.Unlock()
	return result, nil
}

// ---------------------------------------------------------------------------
// EvalToplevel
// ---------------------------------------------------------------------------

// EvalToplevel evaluates a top-level expression.
func EvalToplevel(env Env, expr ast.Expr, programArgs []string) (Value, Env, error) {
	switch e := expr.(type) {
	case ast.TypeDecl:
		newEnv := env.Clone()
		if len(e.RecordFields) > 0 {
			// Record type — store field info for runtime (used by FieldAccess pattern matching)
			return VBool{V: false}, newEnv, nil
		}
		for _, ctor := range e.Ctors {
			if len(ctor.ArgTypes) == 0 {
				newEnv[ctor.Name] = VCtor{Name: ctor.Name, Args: nil}
			} else {
				newEnv[ctor.Name] = VCtorFn{Name: ctor.Name, Remaining: len(ctor.ArgTypes)}
			}
		}
		return VBool{V: false}, newEnv, nil

	case ast.Let:
		if e.InExpr == nil {
			value, err := Eval(env, expr)
			if err != nil {
				return nil, nil, err
			}
			return value, env.Extend(e.Name, value), nil
		}

	case ast.LetPat:
		if e.InExpr == nil {
			value, err := Eval(env, e.Body)
			if err != nil {
				return nil, nil, err
			}
			bindings, ok := matchPattern(e.Pat, value)
			if !ok {
				return nil, nil, runtimeErr("let pattern match failure")
			}
			return value, env.ExtendMany(bindings), nil
		}

	case ast.Import:
		mod, err := loadModule(e.Module, programArgs)
		if err != nil {
			return nil, nil, err
		}
		if e.Alias != "" {
			modBindings := Env{}
			for n := range mod.exports {
				if v, ok := mod.env[n]; ok {
					modBindings[n] = v
				}
			}
			return VBool{V: false}, env.Extend(e.Alias, VModule{Name: e.Alias, Env: modBindings}), nil
		}
		newEnv := env.Clone()
		for _, name := range e.Names {
			if !mod.exports[name] {
				return nil, nil, runtimeErr("'%s' is not exported by module '%s'", name, e.Module)
			}
			newEnv[name] = mod.env[name]
		}
		return VBool{V: false}, newEnv, nil

	case ast.Export:
		return VBool{V: false}, env, nil

	case ast.TraitDecl:
		newEnv := env.Clone()
		for _, m := range e.Methods {
			newEnv[m.Name] = VTraitMethod{TraitName: e.Name, MethodName: m.Name}
		}
		return VBool{V: false}, newEnv, nil

	case ast.ImplDecl:
		newEnv := env.Clone()
		instances := map[string]Value{}
		if inst, ok := env["__instances__"]; ok {
			if vi, ok := inst.(VInstances); ok {
				for k, v := range vi.M {
					instances[k] = v
				}
			}
		}
		for _, m := range e.Methods {
			implVal, err := Eval(env, m.Body)
			if err != nil {
				return nil, nil, err
			}
			key := e.TraitName + ":" + e.TargetType + ":" + m.Name
			instances[key] = implVal
		}
		newEnv["__instances__"] = VInstances{M: instances}
		return VBool{V: false}, newEnv, nil

	case ast.TestDecl:
		return VUnit{}, env, nil

	case ast.TypeAnnotation:
		return VUnit{}, env, nil

	case ast.LetRec:
		if e.InExpr == nil {
			sharedEnv := env.Clone()
			raw := map[string]Value{}
			for _, b := range e.Bindings {
				val, err := Eval(env, b.Body)
				if err != nil {
					return nil, nil, err
				}
				if cl, ok := val.(VClosure); ok {
					raw[b.Name] = VClosure{Param: cl.Param, Body: cl.Body, Env: sharedEnv}
				} else {
					raw[b.Name] = val
				}
			}
			for k, v := range raw {
				sharedEnv[k] = v
			}
			lastVal := raw[e.Bindings[len(e.Bindings)-1].Name]
			return lastVal, env.ExtendMany(raw), nil
		}
	}

	value, err := Eval(env, expr)
	if err != nil {
		return nil, nil, err
	}
	return value, env, nil
}

// ---------------------------------------------------------------------------
// Prelude cache
// ---------------------------------------------------------------------------

var (
	preludeEvalCache *Env
	preludeEvalMu    sync.Mutex
)

func loadPreludeEval(programArgs []string) (Env, error) {
	preludeEvalMu.Lock()
	defer preludeEvalMu.Unlock()
	if preludeEvalCache != nil {
		return preludeEvalCache.Clone(), nil
	}
	src, err := stdlib.Source("Prelude")
	if err != nil {
		return nil, err
	}
	exprs, parseErr := parser.Parse(src)
	if parseErr != nil {
		return nil, parseErr
	}
	env := initialEnvForPrelude()
	for _, expr := range exprs {
		_, newEnv, err := EvalToplevel(env, expr, programArgs)
		if err != nil {
			return nil, err
		}
		env = newEnv
	}
	preludeEvalCache = &env
	return env.Clone(), nil
}

func initialEnvForPrelude() Env {
	env := Env{}
	for k, v := range CoreBuiltins() {
		env[k] = v
	}
	return env
}

// InitialEnv returns the full env with all builtins (including a fresh main-process mailbox).
func InitialEnv(programArgs []string) Env {
	env := Env{}
	for k, v := range CoreBuiltins() {
		env[k] = v
	}
	for k, v := range IOBuiltins() {
		env[k] = v
	}
	for k, v := range MathBuiltins() {
		env[k] = v
	}
	for k, v := range StringBuiltins() {
		env[k] = v
	}
	for k, v := range EnvBuiltins(programArgs) {
		env[k] = v
	}
	return WithProcessBuiltins(env)
}

// ---------------------------------------------------------------------------
// RunProgram and RunTests
// ---------------------------------------------------------------------------

// RunProgram evaluates a list of pre-parsed (and reordered) top-level expressions.
func RunProgram(exprs []ast.Expr, programArgs []string) (Value, error) {
	env, err := loadPreludeEval(programArgs)
	if err != nil {
		return nil, err
	}
	env = WithProcessBuiltins(env)
	var last Value = VBool{V: false}
	for _, expr := range exprs {
		val, newEnv, err := EvalToplevel(env, expr, programArgs)
		if err != nil {
			return nil, err
		}
		last = val
		env = newEnv
	}
	return last, nil
}

// isTTY reports whether stdout is a terminal.
func isTTY() bool {
	fi, err := os.Stdout.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// FailedTest holds the name and source location of a test that did not pass.
type FailedTest struct {
	Name string
	Line int
}

// RunTests runs all test blocks from a list of pre-parsed (and reordered) expressions.
// label is used in the header line (e.g. "=== label ==="); empty to suppress.
// only, if non-empty, restricts execution to tests whose name contains the substring.
// The third return value is the list of failed tests with their source locations.
func RunTests(exprs []ast.Expr, programArgs []string, extraBuiltins map[string]Value, label string, only string) (int, int, []FailedTest, error) {
	env, err := loadPreludeEval(programArgs)
	if err != nil {
		return 0, 0, nil, err
	}
	env = WithProcessBuiltins(env)
	for k, v := range extraBuiltins {
		env[k] = v
	}
	var tests []ast.TestDecl
	for _, expr := range exprs {
		if td, ok := expr.(ast.TestDecl); ok {
			tests = append(tests, td)
		} else {
			_, newEnv, err := EvalToplevel(env, expr, programArgs)
			if err != nil {
				return 0, 0, nil, err
			}
			env = newEnv
		}
	}
	// Filter by --only pattern
	if only != "" {
		filtered := tests[:0]
		for _, t := range tests {
			if strings.Contains(t.Name, only) {
				filtered = append(filtered, t)
			}
		}
		tests = filtered
	}
	if len(tests) == 0 {
		return 0, 0, nil, nil
	}
	if label != "" {
		fmt.Printf("\n=== %s ===\n", label)
	}
	colorGreen, colorRed, colorReset := "", "", ""
	if isTTY() {
		colorGreen = "\033[32m"
		colorRed = "\033[31m"
		colorReset = "\033[0m"
	}
	passed, failed := 0, 0
	var failedTests []FailedTest
	for _, test := range tests {
		testEnv := env.Clone()
		var errs []error
		for _, bodyExpr := range test.Body {
			_, newEnv, err := EvalToplevel(testEnv, bodyExpr, programArgs)
			if err != nil {
				errs = append(errs, err)
				if _, isAssert := err.(*AssertError); !isAssert {
					break // fatal error: stop this test
				}
				// assertion failure: continue collecting, keep last good env
			} else {
				testEnv = newEnv
			}
		}
		if len(errs) > 0 {
			failed++
			failedTests = append(failedTests, FailedTest{Name: test.Name, Line: test.Line})
			fmt.Printf("%sFAIL%s  %s\n", colorRed, colorReset, test.Name)
			for _, e := range errs {
				fmt.Printf("  %s\n", e.Error())
			}
		} else {
			passed++
			fmt.Printf("%sPASS%s  %s\n", colorGreen, colorReset, test.Name)
		}
	}
	fmt.Printf("%d passed, %d failed\n", passed, failed)
	return passed, failed, failedTests, nil
}

// LoadPreludeForREPL loads the prelude for use in the REPL.
func LoadPreludeForREPL(programArgs []string) (Env, error) {
	return loadPreludeEval(programArgs)
}

// ResetCaches clears all module caches.
func ResetCaches() {
	evalModuleCacheMu.Lock()
	evalModuleCache = map[string]*moduleResult{}
	evalModuleCacheMu.Unlock()
	preludeEvalMu.Lock()
	preludeEvalCache = nil
	preludeEvalMu.Unlock()
}
