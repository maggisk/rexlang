// Package eval implements the tree-walking evaluator for RexLang.
package eval

import (
	"fmt"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/maggisk/rexlang/internal/ast"
)

// ---------------------------------------------------------------------------
// Value types
// ---------------------------------------------------------------------------

// Value is the interface all runtime values implement.
type Value interface{ valueKind() }

type VInt struct{ V int }
type VFloat struct{ V float64 }
type VString struct{ V string }
type VBool struct{ V bool }
type VUnit struct{}
type VList struct{ Items []Value }
type VTuple struct{ Items []Value }

type VCtor struct {
	Name string
	Args []Value
}

// VCtorFn is a partially-applied constructor.
type VCtorFn struct {
	Name      string
	Remaining int
	AccArgs   []Value // built in reverse; reversed when Remaining==1
}

type VClosure struct {
	Param string
	Body  ast.Expr
	Env   Env
}

type VBuiltin struct {
	Name string
	Fn   func(Value) (Value, error)
}

type VModule struct {
	Name string
	Env  Env // exported name → Value
}

type VTraitMethod struct {
	TraitName  string
	MethodName string
}

// VInstances wraps the trait instance dispatch table.
type VInstances struct {
	M map[string]Value // key: "TraitName:TypeName:MethodName"
}

// ---------------------------------------------------------------------------
// Process / actor types
// ---------------------------------------------------------------------------

var pidCounter int64

// Mailbox is a buffered channel used as an actor's message queue.
type Mailbox struct {
	ch chan Value
	id int64
}

func newMailbox() *Mailbox {
	id := atomic.AddInt64(&pidCounter, 1)
	return &Mailbox{ch: make(chan Value, 1024), id: id}
}

// VPid is a process identifier (an opaque handle to an actor mailbox).
type VPid struct {
	Mailbox *Mailbox
	ID      int64
}

type VRecord struct {
	TypeName string
	Fields   map[string]Value
}

func (VInt) valueKind()         {}
func (VInstances) valueKind()   {}
func (VFloat) valueKind()       {}
func (VString) valueKind()      {}
func (VBool) valueKind()        {}
func (VUnit) valueKind()        {}
func (VList) valueKind()        {}
func (VTuple) valueKind()       {}
func (VCtor) valueKind()        {}
func (VCtorFn) valueKind()      {}
func (VClosure) valueKind()     {}
func (VBuiltin) valueKind()     {}
func (VModule) valueKind()      {}
func (VTraitMethod) valueKind() {}
func (VPid) valueKind()         {}
func (VRecord) valueKind()      {}

// RuntimeError is a runtime error.
type RuntimeError struct{ Msg string }

func (e *RuntimeError) Error() string { return e.Msg }

func runtimeErr(format string, args ...interface{}) error {
	return &RuntimeError{Msg: fmt.Sprintf(format, args...)}
}

// ---------------------------------------------------------------------------
// Environment
// ---------------------------------------------------------------------------

// Env is a map from name to Value. Copying for closure snapshots is explicit.
type Env map[string]Value

// Extend returns a new Env with name bound to val.
func (e Env) Extend(name string, val Value) Env {
	out := make(Env, len(e)+1)
	for k, v := range e {
		out[k] = v
	}
	out[name] = val
	return out
}

// ExtendMany returns a new Env with all provided bindings.
func (e Env) ExtendMany(bindings map[string]Value) Env {
	out := make(Env, len(e)+len(bindings))
	for k, v := range e {
		out[k] = v
	}
	for k, v := range bindings {
		out[k] = v
	}
	return out
}

// Clone returns a shallow copy.
func (e Env) Clone() Env {
	out := make(Env, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Structural equality
// ---------------------------------------------------------------------------

// StructuralEq recursively compares two Rex values.
func StructuralEq(l, r Value) bool {
	switch a := l.(type) {
	case VInt:
		if b, ok := r.(VInt); ok {
			return a.V == b.V
		}
	case VFloat:
		if b, ok := r.(VFloat); ok {
			return a.V == b.V
		}
	case VString:
		if b, ok := r.(VString); ok {
			return a.V == b.V
		}
	case VBool:
		if b, ok := r.(VBool); ok {
			return a.V == b.V
		}
	case VUnit:
		_, ok := r.(VUnit)
		return ok
	case VList:
		b, ok := r.(VList)
		if !ok || len(a.Items) != len(b.Items) {
			return false
		}
		for i := range a.Items {
			if !StructuralEq(a.Items[i], b.Items[i]) {
				return false
			}
		}
		return true
	case VTuple:
		b, ok := r.(VTuple)
		if !ok || len(a.Items) != len(b.Items) {
			return false
		}
		for i := range a.Items {
			if !StructuralEq(a.Items[i], b.Items[i]) {
				return false
			}
		}
		return true
	case VCtor:
		b, ok := r.(VCtor)
		if !ok || a.Name != b.Name || len(a.Args) != len(b.Args) {
			return false
		}
		for i := range a.Args {
			if !StructuralEq(a.Args[i], b.Args[i]) {
				return false
			}
		}
		return true
	case VPid:
		if b, ok := r.(VPid); ok {
			return a.ID == b.ID
		}
	case VRecord:
		b, ok := r.(VRecord)
		if !ok || a.TypeName != b.TypeName || len(a.Fields) != len(b.Fields) {
			return false
		}
		for k, av := range a.Fields {
			bv, ok := b.Fields[k]
			if !ok || !StructuralEq(av, bv) {
				return false
			}
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Value to string
// ---------------------------------------------------------------------------

func escapeString(s string) string {
	var buf strings.Builder
	buf.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\n':
			buf.WriteString(`\n`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			buf.WriteRune(c)
		}
	}
	buf.WriteByte('"')
	return buf.String()
}

func floatToString(f float64) string {
	// Match Python's float repr behavior for common cases
	s := fmt.Sprintf("%g", f)
	// If no decimal point and no 'e', Python would add .0 for integer floats
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") &&
		!strings.Contains(s, "n") && !strings.Contains(s, "N") { // NaN/Inf
		s += ".0"
	}
	return s
}

// ValueToString returns the machine-readable representation of v.
func ValueToString(v Value) string {
	switch val := v.(type) {
	case VInt:
		return fmt.Sprintf("%d", val.V)
	case VFloat:
		return floatToString(val.V)
	case VString:
		return escapeString(val.V)
	case VBool:
		if val.V {
			return "true"
		}
		return "false"
	case VUnit:
		return "()"
	case VClosure:
		return "<fn>"
	case VBuiltin:
		return fmt.Sprintf("<builtin %s>", val.Name)
	case VCtor:
		if len(val.Args) == 0 {
			return val.Name
		}
		parts := make([]string, len(val.Args))
		for i, a := range val.Args {
			parts[i] = ValueToString(a)
		}
		return "(" + val.Name + " " + strings.Join(parts, " ") + ")"
	case VCtorFn:
		return fmt.Sprintf("<ctor %s>", val.Name)
	case VList:
		parts := make([]string, len(val.Items))
		for i, item := range val.Items {
			parts[i] = ValueToString(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case VTuple:
		parts := make([]string, len(val.Items))
		for i, item := range val.Items {
			parts[i] = ValueToString(item)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case VModule:
		return fmt.Sprintf("<module %s>", val.Name)
	case VTraitMethod:
		return fmt.Sprintf("<trait method %s.%s>", val.TraitName, val.MethodName)
	case VPid:
		return fmt.Sprintf("<pid %d>", val.ID)
	case VRecord:
		parts := make([]string, 0, len(val.Fields))
		for k, v := range val.Fields {
			parts = append(parts, k+" = "+ValueToString(v))
		}
		// Sort for deterministic output
		sort.Strings(parts)
		return val.TypeName + " { " + strings.Join(parts, ", ") + " }"
	}
	panic(fmt.Sprintf("unknown value type: %T", v))
}

// Display returns a human-readable string (strings without quotes).
func Display(v Value) string {
	if s, ok := v.(VString); ok {
		return s.V
	}
	return ValueToString(v)
}

// CheckStr asserts v is a VString and returns its value.
func CheckStr(name string, v Value) (string, error) {
	if s, ok := v.(VString); ok {
		return s.V, nil
	}
	return "", runtimeErr("%s: expected string, got %s", name, ValueToString(v))
}

// AsInt asserts v is a VInt.
func AsInt(v Value) (int, error) {
	if i, ok := v.(VInt); ok {
		return i.V, nil
	}
	return 0, runtimeErr("expected int, got %s", ValueToString(v))
}

// AsFloat asserts v is a VFloat.
func AsFloat(v Value) (float64, error) {
	if f, ok := v.(VFloat); ok {
		return f.V, nil
	}
	return 0, runtimeErr("expected float, got %s", ValueToString(v))
}

// AsBool asserts v is a VBool.
func AsBool(v Value) (bool, error) {
	if b, ok := v.(VBool); ok {
		return b.V, nil
	}
	return false, runtimeErr("expected bool, got %s", ValueToString(v))
}

// RuntimeTypeName returns the runtime type name for trait dispatch.
func RuntimeTypeName(v Value) (string, error) {
	switch v.(type) {
	case VInt:
		return "Int", nil
	case VFloat:
		return "Float", nil
	case VString:
		return "String", nil
	case VBool:
		return "Bool", nil
	}
	return "", runtimeErr("no trait dispatch for %s", ValueToString(v))
}
