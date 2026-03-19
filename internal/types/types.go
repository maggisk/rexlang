// Package types implements the Hindley-Milner type representation for RexLang.
package types

import (
	"fmt"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Type error
// ---------------------------------------------------------------------------

// TypeError is raised during HM inference.
type TypeError struct {
	Msg    string
	Line   int    // source line (0 = unknown)
	Source string // full source text (set by CLI for snippet display)
}

func (e *TypeError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

func typeErr(format string, args ...interface{}) *TypeError {
	return &TypeError{Msg: fmt.Sprintf(format, args...)}
}

// ---------------------------------------------------------------------------
// Type representation
// ---------------------------------------------------------------------------

// Type is the interface implemented by all type nodes.
type Type interface{ typeNode() }

// TVar is a type variable.
type TVar struct{ Name string }

// TCon is a type constructor with zero or more arguments.
type TCon struct {
	Name string
	Args []Type
}

func (TVar) typeNode() {}
func (TCon) typeNode() {}

// Constraint represents a trait constraint on a type variable.
type Constraint struct {
	Trait string
	Var   string // type variable name
}

// Scheme is a universally quantified type.
type Scheme struct {
	Vars        []string // quantified type variable names
	Constraints []Constraint
	Ty          Type
}

// ---------------------------------------------------------------------------
// Primitive type singletons
// ---------------------------------------------------------------------------

var (
	TInt    = TCon{Name: "Int"}
	TFloat  = TCon{Name: "Float"}
	TString = TCon{Name: "String"}
	TBool   = TCon{Name: "Bool"}
	TUnit   = TCon{Name: "Unit"}
)

// ---------------------------------------------------------------------------
// Type constructor helpers
// ---------------------------------------------------------------------------

func TFun(a, b Type) Type {
	return TCon{Name: "Fun", Args: []Type{a, b}}
}

func TList(a Type) Type {
	return TCon{Name: "List", Args: []Type{a}}
}

func TTuple(ts []Type) Type {
	args := make([]Type, len(ts))
	copy(args, ts)
	return TCon{Name: "Tuple", Args: args}
}

func TMaybe(a Type) Type {
	return TCon{Name: "Maybe", Args: []Type{a}}
}

func TResult(a, e Type) Type {
	return TCon{Name: "Result", Args: []Type{a, e}}
}

func TPid(a Type) Type {
	return TCon{Name: "Pid", Args: []Type{a}}
}

var TListener = TCon{Name: "Listener"}
var TConn = TCon{Name: "Conn"}

// ---------------------------------------------------------------------------
// Type equality (structural)
// ---------------------------------------------------------------------------

// TypesEqual checks structural equality of two types.
func TypesEqual(a, b Type) bool {
	switch at := a.(type) {
	case TVar:
		bt, ok := b.(TVar)
		return ok && at.Name == bt.Name
	case TCon:
		bt, ok := b.(TCon)
		if !ok || at.Name != bt.Name || len(at.Args) != len(bt.Args) {
			return false
		}
		for i := range at.Args {
			if !TypesEqual(at.Args[i], bt.Args[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// Record field metadata
// ---------------------------------------------------------------------------

// RecordInfo stores field metadata for a named record type.
type RecordInfo struct {
	Fields []RecordFieldInfo
	Params []string // type parameter names
}

// RecordFieldInfo stores one field's name and type.
type RecordFieldInfo struct {
	Name string
	Type Type
}

// ---------------------------------------------------------------------------
// Type alias metadata
// ---------------------------------------------------------------------------

// TypeAliasInfo stores a type alias definition.
type TypeAliasInfo struct {
	Params []string
	Body   Type
}

// ---------------------------------------------------------------------------
// Substitution — map from TVar name to Type
// ---------------------------------------------------------------------------

type Subst map[string]Type

// FreeVars collects all free type variable names in ty.
func FreeVars(ty Type) map[string]bool {
	result := map[string]bool{}
	freeVarsInto(ty, result)
	return result
}

func freeVarsInto(ty Type, out map[string]bool) {
	switch t := ty.(type) {
	case TVar:
		out[t.Name] = true
	case TCon:
		for _, a := range t.Args {
			freeVarsInto(a, out)
		}
	}
}

func FreeVarsScheme(s Scheme) map[string]bool {
	fv := FreeVars(s.Ty)
	for _, v := range s.Vars {
		delete(fv, v)
	}
	return fv
}

// SubstOnce applies a single-pass substitution (no transitive following).
// Used for type alias expansion where param→arg mappings can form cycles.
func SubstOnce(s Subst, ty Type) Type {
	switch t := ty.(type) {
	case TVar:
		if resolved, ok := s[t.Name]; ok {
			return resolved
		}
		return ty
	case TCon:
		newArgs := make([]Type, len(t.Args))
		for i, a := range t.Args {
			newArgs[i] = SubstOnce(s, a)
		}
		return TCon{Name: t.Name, Args: newArgs}
	}
	return ty
}

// ApplySubst applies substitution s to type ty.
func ApplySubst(s Subst, ty Type) Type {
	switch t := ty.(type) {
	case TVar:
		if resolved, ok := s[t.Name]; ok {
			if tv, isTVar := resolved.(TVar); isTVar && tv.Name == t.Name {
				return ty
			}
			return ApplySubst(s, resolved)
		}
		return ty
	case TCon:
		newArgs := make([]Type, len(t.Args))
		for i, a := range t.Args {
			newArgs[i] = ApplySubst(s, a)
		}
		return TCon{Name: t.Name, Args: newArgs}
	}
	return ty
}

// ApplySubstScheme applies s to a scheme, skipping bound variables.
func ApplySubstScheme(s Subst, scheme Scheme) Scheme {
	restricted := make(Subst, len(s))
	for k, v := range s {
		restricted[k] = v
	}
	for _, v := range scheme.Vars {
		delete(restricted, v)
	}
	// Remap constraints through the substitution
	var newConstraints []Constraint
	for _, c := range scheme.Constraints {
		if _, bound := restricted[c.Var]; !bound {
			newConstraints = append(newConstraints, c)
		} else {
			resolved := ApplySubst(restricted, TVar{Name: c.Var})
			if tv, ok := resolved.(TVar); ok {
				newConstraints = append(newConstraints, Constraint{Trait: c.Trait, Var: tv.Name})
			}
			// If resolved to concrete type, constraint is consumed (checked elsewhere)
		}
	}
	return Scheme{Vars: scheme.Vars, Constraints: newConstraints, Ty: ApplySubst(restricted, scheme.Ty)}
}

// ApplySubstEnv applies s to every Scheme value in env; non-Scheme values pass through.
func ApplySubstEnv(s Subst, env map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(env))
	for k, v := range env {
		if scheme, ok := v.(Scheme); ok {
			result[k] = ApplySubstScheme(s, scheme)
		} else {
			result[k] = v
		}
	}
	return result
}

// ComposeSubst computes s1 ∘ s2 (s2 applied first, then s1).
func ComposeSubst(s1, s2 Subst) Subst {
	result := make(Subst, len(s1)+len(s2))
	for k, v := range s2 {
		result[k] = ApplySubst(s1, v)
	}
	for k, v := range s1 {
		if _, exists := result[k]; !exists {
			result[k] = v
		}
	}
	return result
}

func occurs(varName string, ty Type) bool {
	switch t := ty.(type) {
	case TVar:
		return t.Name == varName
	case TCon:
		for _, a := range t.Args {
			if occurs(varName, a) {
				return true
			}
		}
	}
	return false
}

// Unify unifies two types, returning a substitution.
func Unify(t1, t2 Type) (Subst, error) {
	switch a := t1.(type) {
	case TVar:
		switch b := t2.(type) {
		case TVar:
			if a.Name == b.Name {
				return Subst{}, nil
			}
		}
		if occurs(a.Name, t2) {
			return nil, typeErr("infinite type: %s occurs in %s", TypeToString(t1), TypeToString(t2))
		}
		return Subst{a.Name: t2}, nil
	case TCon:
		switch b := t2.(type) {
		case TVar:
			return Unify(t2, t1)
		case TCon:
			if a.Name != b.Name || len(a.Args) != len(b.Args) {
				return nil, typeErr("type mismatch: %s vs %s", TypeToString(t1), TypeToString(t2))
			}
			subst := Subst{}
			for i := range a.Args {
				s, err := Unify(ApplySubst(subst, a.Args[i]), ApplySubst(subst, b.Args[i]))
				if err != nil {
					return nil, err
				}
				subst = ComposeSubst(s, subst)
			}
			return subst, nil
		}
	}
	return nil, typeErr("cannot unify %s with %s", TypeToString(t1), TypeToString(t2))
}

// Generalize generalizes ty over type variables not free in env.
// If constraints are provided, those on quantified vars are included in the Scheme.
func Generalize(env map[string]interface{}, ty Type, constraints ...[]Constraint) Scheme {
	envFree := map[string]bool{}
	for _, v := range env {
		if scheme, ok := v.(Scheme); ok {
			for name := range FreeVarsScheme(scheme) {
				envFree[name] = true
			}
		}
	}
	tyFree := FreeVars(ty)
	quantifiedSet := map[string]bool{}
	var quantified []string
	for name := range tyFree {
		if !envFree[name] {
			quantified = append(quantified, name)
			quantifiedSet[name] = true
		}
	}
	sort.Strings(quantified)
	var cs []Constraint
	if len(constraints) > 0 {
		seen := map[string]bool{}
		for _, c := range constraints[0] {
			if quantifiedSet[c.Var] {
				key := c.Trait + ":" + c.Var
				if !seen[key] {
					seen[key] = true
					cs = append(cs, c)
				}
			}
		}
		// Sort constraints for deterministic output
		sort.Slice(cs, func(i, j int) bool {
			if cs[i].Var != cs[j].Var {
				return cs[i].Var < cs[j].Var
			}
			return cs[i].Trait < cs[j].Trait
		})
	}
	return Scheme{Vars: quantified, Constraints: cs, Ty: ty}
}

// ---------------------------------------------------------------------------
// Pretty-printing
// ---------------------------------------------------------------------------

// TypeToString pretty-prints a type, renaming TVars to a, b, c... in order.
func TypeToString(ty Type) string {
	mapping := map[string]string{}
	counter := 0
	nameFor := func(varName string) string {
		if n, ok := mapping[varName]; ok {
			return n
		}
		var n string
		if counter < 26 {
			n = string(rune('a' + counter))
		} else {
			n = fmt.Sprintf("t%d", counter)
		}
		counter++
		mapping[varName] = n
		return n
	}
	var render func(ty Type, inFunArg bool) string
	render = func(ty Type, inFunArg bool) string {
		switch t := ty.(type) {
		case TVar:
			return nameFor(t.Name)
		case TCon:
			switch t.Name {
			case "Fun":
				a, b := t.Args[0], t.Args[1]
				result := render(a, true) + " -> " + render(b, false)
				if inFunArg {
					return "(" + result + ")"
				}
				return result
			case "Unit":
				if len(t.Args) == 0 {
					return "()"
				}
			case "List":
				if len(t.Args) == 1 {
					return "[" + render(t.Args[0], false) + "]"
				}
			case "Tuple":
				parts := make([]string, len(t.Args))
				for i, a := range t.Args {
					parts[i] = render(a, false)
				}
				return "(" + strings.Join(parts, ", ") + ")"
			}
			if len(t.Args) == 0 {
				return t.Name
			}
			parts := make([]string, len(t.Args))
			for i, a := range t.Args {
				parts[i] = render(a, false)
			}
			return "(" + t.Name + " " + strings.Join(parts, " ") + ")"
		}
		return fmt.Sprintf("%v", ty)
	}
	return render(ty, false)
}

// SchemeToString pretty-prints a scheme, including constraints if any.
// e.g. "Ord a => [a] -> [a]"
func SchemeToString(s Scheme) string {
	tyStr := TypeToString(s.Ty)
	if len(s.Constraints) == 0 {
		return tyStr
	}
	// Build the TVar name mapping consistent with TypeToString
	mapping := map[string]string{}
	counter := 0
	nameFor := func(varName string) string {
		if n, ok := mapping[varName]; ok {
			return n
		}
		var n string
		if counter < 26 {
			n = string(rune('a' + counter))
		} else {
			n = fmt.Sprintf("t%d", counter)
		}
		counter++
		mapping[varName] = n
		return n
	}
	// Walk the type to establish variable naming order (same as TypeToString)
	var walkType func(ty Type)
	walkType = func(ty Type) {
		switch t := ty.(type) {
		case TVar:
			nameFor(t.Name)
		case TCon:
			for _, a := range t.Args {
				walkType(a)
			}
		}
	}
	walkType(s.Ty)
	// Render constraints using the same mapping
	var parts []string
	for _, c := range s.Constraints {
		parts = append(parts, c.Trait+" "+nameFor(c.Var))
	}
	prefix := ""
	if len(parts) == 1 {
		prefix = parts[0] + " => "
	} else {
		prefix = "(" + strings.Join(parts, ", ") + ") => "
	}
	return prefix + tyStr
}
