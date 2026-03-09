// Package ir defines the A-normal form intermediate representation for RexLang.
//
// In ANF, every subexpression is named via a let binding. Function applications
// and operators take only atoms (variables or literals) as arguments. This makes
// evaluation order explicit and simplifies codegen — no stack juggling needed.
//
//	expr ::= let x : ty = cexpr in expr   -- binding
//	       | atom                          -- return/tail position
//	       | cexpr                         -- tail-position complex expr
//
//	cexpr ::= atom atom                    -- application
//	        | prim op atom atom            -- binary operator
//	        | if atom then expr else expr  -- conditional
//	        | match atom with arms         -- pattern match
//	        | ctor name [atom]             -- constructor application
//	        | record TypeName [(name,atom)] -- record creation
//	        | atom.field                   -- field access
//	        | {atom | field=atom, ...}     -- record update
//	        | [atom, ...]                  -- list literal
//	        | (atom, ...)                  -- tuple literal
//	        | \param -> expr               -- lambda (closure)
//
//	atom ::= var | int | float | string | bool | unit
package ir

import "github.com/maggisk/rexlang/internal/types"

// ---------------------------------------------------------------------------
// Atoms — values that don't require further evaluation
// ---------------------------------------------------------------------------

type Atom interface{ atomNode() }

type AVar struct {
	Name string
	Ty   types.Type
}

type AInt struct{ Value int }
type AFloat struct{ Value float64 }
type AString struct{ Value string }
type ABool struct{ Value bool }
type AUnit struct{}

func (AVar) atomNode()    {}
func (AInt) atomNode()    {}
func (AFloat) atomNode()  {}
func (AString) atomNode() {}
func (ABool) atomNode()   {}
func (AUnit) atomNode()   {}

// ---------------------------------------------------------------------------
// Complex expressions — operations that produce values
// ---------------------------------------------------------------------------

type CExpr interface{ cexprNode() }

// CApp is function application: func arg (both atoms).
type CApp struct {
	Func Atom
	Arg  Atom
	Ty   types.Type // result type
}

// CBinop is a binary operator: left op right.
type CBinop struct {
	Op    string
	Left  Atom
	Right Atom
	Ty    types.Type
}

// CUnaryMinus negates a numeric atom.
type CUnaryMinus struct {
	Expr Atom
	Ty   types.Type
}

// CIf is a conditional: if cond then thenBranch else elseBranch.
type CIf struct {
	Cond Atom
	Then Expr
	Else Expr
	Ty   types.Type
}

// CMatch is pattern matching on a scrutinee atom.
type CMatch struct {
	Scrutinee Atom
	Arms      []MatchArm
	Ty        types.Type
}

// MatchArm is one arm of a match expression.
type MatchArm struct {
	Pat  Pattern
	Body Expr
}

// CLambda creates a closure: \param -> body.
type CLambda struct {
	Param string
	Body  Expr
	Ty    types.Type // full function type (param -> result)
}

// CCtor constructs an ADT value.
type CCtor struct {
	Name string
	Args []Atom
	Ty   types.Type
}

// CRecord creates a record value.
type CRecord struct {
	TypeName string
	Fields   []FieldInit
	Ty       types.Type
}

type FieldInit struct {
	Name  string
	Value Atom
}

// CFieldAccess reads a field from a record.
type CFieldAccess struct {
	Record Atom
	Field  string
	Ty     types.Type
}

// CRecordUpdate creates a new record with some fields changed.
type CRecordUpdate struct {
	Record  Atom
	Updates []FieldUpdate
	Ty      types.Type
}

type FieldUpdate struct {
	Path  []string // ["name"] or ["user", "name"] for nested
	Value Atom
}

// CList creates a list literal.
type CList struct {
	Items []Atom
	Ty    types.Type
}

// CTuple creates a tuple literal.
type CTuple struct {
	Items []Atom
	Ty    types.Type
}

// CStringInterp creates an interpolated string.
type CStringInterp struct {
	Parts []Atom // alternating string literals and expression results
}

func (CApp) cexprNode()          {}
func (CBinop) cexprNode()        {}
func (CUnaryMinus) cexprNode()   {}
func (CIf) cexprNode()           {}
func (CMatch) cexprNode()        {}
func (CLambda) cexprNode()       {}
func (CCtor) cexprNode()         {}
func (CRecord) cexprNode()       {}
func (CFieldAccess) cexprNode()  {}
func (CRecordUpdate) cexprNode() {}
func (CList) cexprNode()         {}
func (CTuple) cexprNode()        {}
func (CStringInterp) cexprNode() {}

// ---------------------------------------------------------------------------
// Expressions — the top-level ANF form
// ---------------------------------------------------------------------------

type Expr interface{ exprNode() }

// EAtom returns an atom (variable or literal) — the base case.
type EAtom struct{ A Atom }

// EComplex evaluates a complex expression in tail position.
type EComplex struct{ C CExpr }

// ELet binds the result of a complex expression to a name, then continues.
type ELet struct {
	Name string
	Ty   types.Type
	Bind CExpr
	Body Expr
}

// ELetRec binds mutually recursive definitions (closures).
type ELetRec struct {
	Bindings []RecBinding
	Body     Expr
}

type RecBinding struct {
	Name string
	Ty   types.Type
	Bind CExpr // typically CLambda
}

func (EAtom) exprNode()    {}
func (EComplex) exprNode() {}
func (ELet) exprNode()     {}
func (ELetRec) exprNode()  {}

// ---------------------------------------------------------------------------
// Patterns (mirrored from AST but part of the IR package)
// ---------------------------------------------------------------------------

type Pattern interface{ patternNode() }

type PWild struct{}
type PVar struct{ Name string }
type PInt struct{ Value int }
type PFloat struct{ Value float64 }
type PString struct{ Value string }
type PBool struct{ Value bool }
type PUnit struct{}
type PNil struct{}

type PCons struct {
	Head Pattern
	Tail Pattern
}

type PTuple struct{ Pats []Pattern }

type PCtor struct {
	Name string
	Args []Pattern
}

type PRecord struct {
	TypeName string
	Fields   []PRecordField
}

type PRecordField struct {
	Name string
	Pat  Pattern
}

func (PWild) patternNode()   {}
func (PVar) patternNode()    {}
func (PInt) patternNode()    {}
func (PFloat) patternNode()  {}
func (PString) patternNode() {}
func (PBool) patternNode()   {}
func (PUnit) patternNode()   {}
func (PNil) patternNode()    {}
func (PCons) patternNode()   {}
func (PTuple) patternNode()  {}
func (PCtor) patternNode()   {}
func (PRecord) patternNode() {}

// ---------------------------------------------------------------------------
// Top-level declarations
// ---------------------------------------------------------------------------

type Decl interface{ declNode() }

// DLet is a top-level value binding.
type DLet struct {
	Name     string
	Exported bool
	Ty       types.Type
	Body     Expr
}

// DLetRec is a group of mutually recursive top-level bindings.
type DLetRec struct {
	Bindings []RecBinding
	Exported map[string]bool
}

// DType is a type declaration (ADT or record). Preserved for codegen
// to emit struct type definitions.
type DType struct {
	Name     string
	Exported bool
	Opaque   bool
	Params   []string
	Ctors    []CtorDef
	Fields   []FieldDef // non-nil for records
}

type CtorDef struct {
	Name     string
	ArgTypes []types.Type
}

type FieldDef struct {
	Name string
	Ty   types.Type
}

// DTrait is a trait declaration.
type DTrait struct {
	Name     string
	Exported bool
	Param    string
	Methods  []TraitMethodDef
}

type TraitMethodDef struct {
	Name string
	Ty   types.Type
}

// DImpl is a trait implementation.
type DImpl struct {
	TraitName      string
	TargetTypeName string     // e.g. "Int", "List", "Maybe"
	TargetType     types.Type // resolved type (for future use)
	Methods        []ImplMethodDef
}

type ImplMethodDef struct {
	Name string
	Body Expr
}

// DImport is a module import.
type DImport struct {
	Module string
	Names  []string
	Alias  string
}

// DTest is a test declaration (only evaluated in test mode).
type DTest struct {
	Name string
	Body Expr
}

func (DLet) declNode()    {}
func (DLetRec) declNode() {}
func (DType) declNode()   {}
func (DTrait) declNode()  {}
func (DImpl) declNode()   {}
func (DImport) declNode() {}
func (DTest) declNode()   {}

// Program is the complete lowered program.
type Program struct {
	Decls []Decl
}
