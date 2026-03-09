package ir

import (
	"fmt"

	"github.com/maggisk/rexlang/internal/ast"
)

// Lowerer converts an AST program into ANF IR.
type Lowerer struct {
	counter int
}

// NewLowerer creates a new Lowerer.
func NewLowerer() *Lowerer {
	return &Lowerer{}
}

// fresh generates a unique temporary variable name.
func (l *Lowerer) fresh(prefix string) string {
	l.counter++
	return fmt.Sprintf("_%s%d", prefix, l.counter)
}

// LowerProgram converts a list of top-level AST expressions into an IR Program.
func (l *Lowerer) LowerProgram(exprs []ast.Expr) (*Program, error) {
	var decls []Decl
	for _, expr := range exprs {
		d, err := l.lowerToplevel(expr)
		if err != nil {
			return nil, err
		}
		if d != nil {
			decls = append(decls, d)
		}
	}
	return &Program{Decls: decls}, nil
}

// lowerToplevel converts a single top-level AST expression into an IR declaration.
func (l *Lowerer) lowerToplevel(expr ast.Expr) (Decl, error) {
	switch e := expr.(type) {
	case ast.Let:
		body, err := l.lowerExpr(e.Body)
		if err != nil {
			return nil, err
		}
		return DLet{
			Name:     e.Name,
			Exported: e.Exported,
			Body:     body,
		}, nil

	case ast.LetRec:
		var bindings []RecBinding
		for _, b := range e.Bindings {
			body, err := l.lowerExpr(b.Body)
			if err != nil {
				return nil, err
			}
			bindings = append(bindings, RecBinding{Name: b.Name, Bind: CLambda{Body: body}})
		}
		exported := make(map[string]bool)
		if e.Exported {
			for _, b := range e.Bindings {
				exported[b.Name] = true
			}
		}
		return DLetRec{Bindings: bindings, Exported: exported}, nil

	case ast.LetPat:
		// Pattern bindings at top level — skip for now
		return nil, nil

	case ast.TypeDecl:
		return l.lowerTypeDecl(e), nil

	case ast.TraitDecl:
		return DTrait{
			Name:     e.Name,
			Exported: e.Exported,
			Param:    e.Param,
		}, nil

	case ast.ImplDecl:
		var methods []ImplMethodDef
		for _, m := range e.Methods {
			body, err := l.lowerExpr(m.Body)
			if err != nil {
				return nil, err
			}
			methods = append(methods, ImplMethodDef{Name: m.Name, Body: body})
		}
		return DImpl{
			TraitName:      e.TraitName,
			TargetTypeName: implTargetName(e.TargetType),
			Methods:        methods,
		}, nil

	case ast.Import:
		return DImport{Module: e.Module, Names: e.Names, Alias: e.Alias}, nil

	case ast.TestDecl:
		// Lower test body as a sequence of expressions
		body, err := l.lowerExprList(e.Body)
		if err != nil {
			return nil, err
		}
		return DTest{Name: e.Name, Body: body}, nil

	case ast.Export, ast.TypeAnnotation:
		// These are metadata — no IR equivalent needed
		return nil, nil

	default:
		return nil, fmt.Errorf("unsupported top-level expression: %T", expr)
	}
}

func (l *Lowerer) lowerTypeDecl(e ast.TypeDecl) Decl {
	if e.AliasType != nil {
		// Type aliases don't need IR representation
		return nil
	}
	d := DType{
		Name:     e.Name,
		Exported: e.Exported,
		Opaque:   e.Opaque,
		Params:   e.Params,
	}
	for _, c := range e.Ctors {
		d.Ctors = append(d.Ctors, CtorDef{Name: c.Name})
	}
	for _, f := range e.RecordFields {
		d.Fields = append(d.Fields, FieldDef{Name: f.Name})
	}
	return d
}

// lowerExprList lowers a sequence of expressions, threading them as
// let _ = e1 in let _ = e2 in ... en
func (l *Lowerer) lowerExprList(exprs []ast.Expr) (Expr, error) {
	if len(exprs) == 0 {
		return EAtom{A: AUnit{}}, nil
	}
	if len(exprs) == 1 {
		return l.lowerExpr(exprs[0])
	}
	// Thread: let _ = e1 in (let _ = e2 in ... en)
	rest, err := l.lowerExprList(exprs[1:])
	if err != nil {
		return nil, err
	}
	first, err := l.lowerExpr(exprs[0])
	if err != nil {
		return nil, err
	}
	// If first is already a simple atom, wrap in a let _ = ...
	switch f := first.(type) {
	case EAtom:
		_ = f // discard the value, proceed to rest
		return rest, nil
	case EComplex:
		return ELet{Name: "_", Bind: f.C, Body: rest}, nil
	default:
		// Wrap in a temporary
		tmp := l.fresh("seq")
		return l.wrapExpr(first, tmp, rest), nil
	}
}

// lowerExpr converts an AST expression to an IR expression in ANF.
// The result is in ANF: complex subexpressions are named via let bindings.
func (l *Lowerer) lowerExpr(expr ast.Expr) (Expr, error) {
	switch e := expr.(type) {
	// --- Atoms (already values) ---
	case ast.IntLit:
		return EAtom{A: AInt{Value: e.Value}}, nil
	case ast.FloatLit:
		return EAtom{A: AFloat{Value: e.Value}}, nil
	case ast.StringLit:
		return EAtom{A: AString{Value: e.Value}}, nil
	case ast.BoolLit:
		return EAtom{A: ABool{Value: e.Value}}, nil
	case ast.UnitLit:
		return EAtom{A: AUnit{}}, nil
	case ast.Var:
		return EAtom{A: AVar{Name: e.Name}}, nil

	// --- Unary minus ---
	case ast.UnaryMinus:
		return l.normalizeUnary(e)

	// --- Binary operators ---
	case ast.Binop:
		return l.normalizeBinop(e)

	// --- Function application ---
	case ast.App:
		return l.normalizeApp(e)

	// --- Lambda ---
	case ast.Fun:
		body, err := l.lowerExpr(e.Body)
		if err != nil {
			return nil, err
		}
		return EComplex{C: CLambda{Param: e.Param, Body: body}}, nil

	// --- If ---
	case ast.If:
		return l.normalizeIf(e)

	// --- Match ---
	case ast.Match:
		return l.normalizeMatch(e)

	// --- Let ---
	case ast.Let:
		return l.normalizeLet(e)

	// --- LetRec ---
	case ast.LetRec:
		return l.normalizeLetRec(e)

	// --- LetPat ---
	case ast.LetPat:
		return l.normalizeLetPat(e)

	// --- List literal ---
	case ast.ListLit:
		return l.normalizeList(e)

	// --- Tuple literal ---
	case ast.TupleLit:
		return l.normalizeTuple(e)

	// --- String interpolation ---
	case ast.StringInterp:
		return l.normalizeStringInterp(e)

	// --- Records ---
	case ast.RecordCreate:
		return l.normalizeRecordCreate(e)
	case ast.FieldAccess:
		return l.normalizeFieldAccess(e)
	case ast.RecordUpdate:
		return l.normalizeRecordUpdate(e)

	// --- Module access ---
	case ast.DotAccess:
		return EAtom{A: AVar{Name: e.ModuleName + "." + e.FieldName}}, nil

	// --- Assert ---
	case ast.Assert:
		return l.lowerExpr(e.Expr)

	// --- Declarations that can appear in expression position ---
	case ast.TypeDecl, ast.TraitDecl, ast.ImplDecl, ast.Import, ast.Export, ast.TypeAnnotation:
		return EAtom{A: AUnit{}}, nil

	case ast.TestDecl:
		return l.lowerExprList(e.Body)

	default:
		return nil, fmt.Errorf("unsupported expression in IR lowering: %T", expr)
	}
}

// ---------------------------------------------------------------------------
// Normalization helpers — convert AST nodes to ANF
// ---------------------------------------------------------------------------

// normalize converts an AST expression to an atom, inserting let bindings
// for complex subexpressions. It calls the continuation with the atom and
// builds up the let chain.
func (l *Lowerer) normalize(expr ast.Expr, k func(Atom) (Expr, error)) (Expr, error) {
	ir, err := l.lowerExpr(expr)
	if err != nil {
		return nil, err
	}
	switch e := ir.(type) {
	case EAtom:
		return k(e.A)
	case EComplex:
		tmp := l.fresh("t")
		a, err := k(AVar{Name: tmp})
		if err != nil {
			return nil, err
		}
		return ELet{Name: tmp, Bind: e.C, Body: a}, nil
	default:
		// General case: bind to a temp
		tmp := l.fresh("t")
		rest, err := k(AVar{Name: tmp})
		if err != nil {
			return nil, err
		}
		return l.wrapExpr(ir, tmp, rest), nil
	}
}

// wrapExpr wraps an IR expression in let bindings to make it produce a named atom.
func (l *Lowerer) wrapExpr(ir Expr, name string, body Expr) Expr {
	switch e := ir.(type) {
	case EAtom:
		// No binding needed — but we need to substitute. For simplicity,
		// just wrap in a let.
		return ELet{Name: name, Bind: CApp{Func: AVar{Name: "__id"}, Arg: e.A}, Body: body}
	case EComplex:
		return ELet{Name: name, Bind: e.C, Body: body}
	case ELet:
		// Nest: let x = ... in (let name = <inner result> in body)
		e.Body = l.wrapExpr(e.Body, name, body)
		return e
	case ELetRec:
		e.Body = l.wrapExpr(e.Body, name, body)
		return e
	default:
		return ir
	}
}

func (l *Lowerer) normalizeApp(e ast.App) (Expr, error) {
	return l.normalize(e.Func, func(fn Atom) (Expr, error) {
		return l.normalize(e.Arg, func(arg Atom) (Expr, error) {
			return EComplex{C: CApp{Func: fn, Arg: arg}}, nil
		})
	})
}

func (l *Lowerer) normalizeBinop(e ast.Binop) (Expr, error) {
	return l.normalize(e.Left, func(left Atom) (Expr, error) {
		return l.normalize(e.Right, func(right Atom) (Expr, error) {
			return EComplex{C: CBinop{Op: e.Op, Left: left, Right: right}}, nil
		})
	})
}

func (l *Lowerer) normalizeUnary(e ast.UnaryMinus) (Expr, error) {
	return l.normalize(e.Expr, func(a Atom) (Expr, error) {
		return EComplex{C: CUnaryMinus{Expr: a}}, nil
	})
}

func (l *Lowerer) normalizeIf(e ast.If) (Expr, error) {
	return l.normalize(e.Cond, func(cond Atom) (Expr, error) {
		thenBr, err := l.lowerExpr(e.ThenExpr)
		if err != nil {
			return nil, err
		}
		elseBr, err := l.lowerExpr(e.ElseExpr)
		if err != nil {
			return nil, err
		}
		return EComplex{C: CIf{Cond: cond, Then: thenBr, Else: elseBr}}, nil
	})
}

func (l *Lowerer) normalizeMatch(e ast.Match) (Expr, error) {
	return l.normalize(e.Scrutinee, func(scrut Atom) (Expr, error) {
		var arms []MatchArm
		for _, arm := range e.Arms {
			pat := lowerPattern(arm.Pat)
			body, err := l.lowerExpr(arm.Body)
			if err != nil {
				return nil, err
			}
			arms = append(arms, MatchArm{Pat: pat, Body: body})
		}
		return EComplex{C: CMatch{Scrutinee: scrut, Arms: arms}}, nil
	})
}

func (l *Lowerer) normalizeLet(e ast.Let) (Expr, error) {
	bind, err := l.lowerExpr(e.Body)
	if err != nil {
		return nil, err
	}
	if e.InExpr == nil {
		// Top-level let — just return the body
		return bind, nil
	}
	cont, err := l.lowerExpr(e.InExpr)
	if err != nil {
		return nil, err
	}
	return l.wrapExpr(bind, e.Name, cont), nil
}

func (l *Lowerer) normalizeLetRec(e ast.LetRec) (Expr, error) {
	var bindings []RecBinding
	for _, b := range e.Bindings {
		body, err := l.lowerExpr(b.Body)
		if err != nil {
			return nil, err
		}
		// Wrap the body in a CLambda if it's not already one
		var cexpr CExpr
		switch be := body.(type) {
		case EComplex:
			cexpr = be.C
		default:
			cexpr = CLambda{Body: body}
		}
		bindings = append(bindings, RecBinding{Name: b.Name, Bind: cexpr})
	}
	if e.InExpr == nil {
		return ELetRec{Bindings: bindings, Body: EAtom{A: AUnit{}}}, nil
	}
	cont, err := l.lowerExpr(e.InExpr)
	if err != nil {
		return nil, err
	}
	return ELetRec{Bindings: bindings, Body: cont}, nil
}

func (l *Lowerer) normalizeLetPat(e ast.LetPat) (Expr, error) {
	bind, err := l.lowerExpr(e.Body)
	if err != nil {
		return nil, err
	}
	if e.InExpr == nil {
		return bind, nil
	}
	cont, err := l.lowerExpr(e.InExpr)
	if err != nil {
		return nil, err
	}
	// Pattern let — for now, treat as a single match arm
	pat := lowerPattern(e.Pat)
	tmp := l.fresh("pat")
	matchExpr := EComplex{C: CMatch{
		Scrutinee: AVar{Name: tmp},
		Arms:      []MatchArm{{Pat: pat, Body: cont}},
	}}
	return l.wrapExpr(bind, tmp, matchExpr), nil
}

func (l *Lowerer) normalizeList(e ast.ListLit) (Expr, error) {
	return l.normalizeAtoms(e.Items, func(atoms []Atom) (Expr, error) {
		return EComplex{C: CList{Items: atoms}}, nil
	})
}

func (l *Lowerer) normalizeTuple(e ast.TupleLit) (Expr, error) {
	return l.normalizeAtoms(e.Items, func(atoms []Atom) (Expr, error) {
		return EComplex{C: CTuple{Items: atoms}}, nil
	})
}

func (l *Lowerer) normalizeStringInterp(e ast.StringInterp) (Expr, error) {
	return l.normalizeAtoms(e.Parts, func(atoms []Atom) (Expr, error) {
		return EComplex{C: CStringInterp{Parts: atoms}}, nil
	})
}

func (l *Lowerer) normalizeRecordCreate(e ast.RecordCreate) (Expr, error) {
	fieldExprs := make([]ast.Expr, len(e.Fields))
	for i, f := range e.Fields {
		fieldExprs[i] = f.Value
	}
	return l.normalizeAtoms(fieldExprs, func(atoms []Atom) (Expr, error) {
		var fields []FieldInit
		for i, f := range e.Fields {
			fields = append(fields, FieldInit{Name: f.Name, Value: atoms[i]})
		}
		return EComplex{C: CRecord{TypeName: e.TypeName, Fields: fields}}, nil
	})
}

func (l *Lowerer) normalizeFieldAccess(e ast.FieldAccess) (Expr, error) {
	return l.normalize(e.Record, func(rec Atom) (Expr, error) {
		return EComplex{C: CFieldAccess{Record: rec, Field: e.Field}}, nil
	})
}

func (l *Lowerer) normalizeRecordUpdate(e ast.RecordUpdate) (Expr, error) {
	// Normalize the record expression and all update values
	return l.normalize(e.Record, func(rec Atom) (Expr, error) {
		updateExprs := make([]ast.Expr, len(e.Updates))
		for i, u := range e.Updates {
			updateExprs[i] = u.Value
		}
		return l.normalizeAtoms(updateExprs, func(atoms []Atom) (Expr, error) {
			var updates []FieldUpdate
			for i, u := range e.Updates {
				updates = append(updates, FieldUpdate{Path: u.Path, Value: atoms[i]})
			}
			return EComplex{C: CRecordUpdate{Record: rec, Updates: updates}}, nil
		})
	})
}

// normalizeAtoms normalizes a list of AST expressions to atoms, threading
// let bindings for complex ones, then calls the continuation with all atoms.
func (l *Lowerer) normalizeAtoms(exprs []ast.Expr, k func([]Atom) (Expr, error)) (Expr, error) {
	atoms := make([]Atom, len(exprs))
	return l.normalizeAtomsHelper(exprs, atoms, 0, k)
}

func (l *Lowerer) normalizeAtomsHelper(exprs []ast.Expr, atoms []Atom, idx int, k func([]Atom) (Expr, error)) (Expr, error) {
	if idx >= len(exprs) {
		return k(atoms)
	}
	return l.normalize(exprs[idx], func(a Atom) (Expr, error) {
		atoms[idx] = a
		return l.normalizeAtomsHelper(exprs, atoms, idx+1, k)
	})
}

// ---------------------------------------------------------------------------
// Pattern lowering (AST patterns → IR patterns)
// ---------------------------------------------------------------------------

func lowerPattern(pat ast.Pattern) Pattern {
	switch p := pat.(type) {
	case ast.PWild:
		return PWild{}
	case ast.PUnit:
		return PUnit{}
	case ast.PVar:
		return PVar{Name: p.Name}
	case ast.PInt:
		return PInt{Value: p.Value}
	case ast.PFloat:
		return PFloat{Value: p.Value}
	case ast.PString:
		return PString{Value: p.Value}
	case ast.PBool:
		return PBool{Value: p.Value}
	case ast.PNil:
		return PNil{}
	case ast.PCons:
		return PCons{Head: lowerPattern(p.Head), Tail: lowerPattern(p.Tail)}
	case ast.PTuple:
		pats := make([]Pattern, len(p.Pats))
		for i, sub := range p.Pats {
			pats[i] = lowerPattern(sub)
		}
		return PTuple{Pats: pats}
	case ast.PCtor:
		args := make([]Pattern, len(p.Args))
		for i, sub := range p.Args {
			args[i] = lowerPattern(sub)
		}
		return PCtor{Name: p.Name, Args: args}
	case ast.PRecord:
		fields := make([]PRecordField, len(p.Fields))
		for i, f := range p.Fields {
			fields[i] = PRecordField{Name: f.Name, Pat: lowerPattern(f.Pat)}
		}
		return PRecord{TypeName: p.TypeName, Fields: fields}
	default:
		return PWild{}
	}
}

// implTargetName extracts the type name from an impl target type syntax.
func implTargetName(ty ast.TySyntax) string {
	switch t := ty.(type) {
	case ast.TyName:
		return t.Name
	case ast.TyApp:
		return t.Name
	case ast.TyList:
		return "List"
	case ast.TyTuple:
		return fmt.Sprintf("Tuple%d", len(t.Elems))
	case ast.TyUnit:
		return "Unit"
	}
	return fmt.Sprintf("%T", ty)
}
