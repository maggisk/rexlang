// Package ast defines the AST node types for RexLang.
package ast

// ---------------------------------------------------------------------------
// Interfaces
// ---------------------------------------------------------------------------

// Expr is the interface all expression nodes implement.
type Expr interface{ exprNode() }

// Pattern is the interface all pattern nodes implement.
type Pattern interface{ patternNode() }

// TySyntax is the interface all type syntax nodes implement.
type TySyntax interface{ tySyntaxNode() }

// ---------------------------------------------------------------------------
// Patterns
// ---------------------------------------------------------------------------

type PWild struct{}                  // _
type PUnit struct{}                  // ()
type PVar struct{ Name string }      // x
type PInt struct{ Value int }        // 42
type PFloat struct{ Value float64 }  // 3.14
type PString struct{ Value string }  // "hello"
type PBool struct{ Value bool }      // true / false
type PNil struct{}                   // []
type PCons struct{ Head, Tail Pattern }  // [h|t]
type PTuple struct{ Pats []Pattern }    // (a, b)
type PCtor struct {
	Name string
	Args []Pattern
}

func (PWild) patternNode()   {}
func (PUnit) patternNode()   {}
func (PVar) patternNode()    {}
func (PInt) patternNode()    {}
func (PFloat) patternNode()  {}
func (PString) patternNode() {}
func (PBool) patternNode()   {}
func (PNil) patternNode()    {}
func (PCons) patternNode()   {}
func (PTuple) patternNode()  {}
func (PCtor) patternNode()   {}

// ---------------------------------------------------------------------------
// Binary operator symbols
// ---------------------------------------------------------------------------

var BinopSym = map[string]string{
	"Add":    "+",
	"Sub":    "-",
	"Mul":    "*",
	"Div":    "/",
	"Mod":    "%",
	"Eq":     "==",
	"Lt":     "<",
	"Gt":     ">",
	"Leq":    "<=",
	"Geq":    ">=",
	"Neq":    "/=",
	"Concat": "++",
	"And":    "&&",
	"Or":     "||",
	"Cons":   "::",
}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

type IntLit struct{ Value int }
type FloatLit struct{ Value float64 }
type StringLit struct{ Value string }
type BoolLit struct{ Value bool }
type UnitLit struct{}
type Var struct{ Name string }

type UnaryMinus struct{ Expr Expr }

type Binop struct {
	Op    string
	Left  Expr
	Right Expr
}

type If struct {
	Cond     Expr
	ThenExpr Expr
	ElseExpr Expr
}

type Let struct {
	Name      string
	Recursive bool
	Body      Expr
	InExpr    Expr // nil = top-level
}

type Fun struct {
	Param string
	Body  Expr
}

type App struct {
	Func Expr
	Arg  Expr
}

type Match struct {
	Scrutinee Expr
	Arms      []MatchArm
}

type MatchArm struct {
	Pat  Pattern
	Body Expr
}

type TypeDecl struct {
	Name   string
	Params []string
	Ctors  []CtorDef
}

type CtorDef struct {
	Name     string
	ArgTypes []TySyntax
}

type ListLit struct{ Items []Expr }
type TupleLit struct{ Items []Expr }

type LetPat struct {
	Pat    Pattern
	Body   Expr
	InExpr Expr // nil = top-level
}

type LetRec struct {
	Bindings []LetRecBinding
	InExpr   Expr // nil = top-level
}

type LetRecBinding struct {
	Name string
	Body Expr
}

type Import struct {
	Module string   // e.g. "std:List"
	Names  []string // selective import names (empty when alias form)
	Alias  string   // e.g. "L" (empty when selective form)
}

type DotAccess struct {
	ModuleName string
	FieldName  string
}

type Export struct{ Names []string }

// Trait/impl declarations

type TraitDecl struct {
	Name    string
	Param   string
	Methods []TraitMethod
}

type TraitMethod struct {
	Name string
	Type TySyntax
}

type ImplDecl struct {
	TraitName  string
	TargetType string
	Methods    []ImplMethod
}

type ImplMethod struct {
	Name string
	Body Expr
}

// Test declarations

type TestDecl struct {
	Name string
	Body []Expr
}

type Assert struct {
	Expr Expr
	Line int
}

// Implement exprNode for all expression types
func (IntLit) exprNode()     {}
func (FloatLit) exprNode()   {}
func (StringLit) exprNode()  {}
func (BoolLit) exprNode()    {}
func (UnitLit) exprNode()    {}
func (Var) exprNode()        {}
func (UnaryMinus) exprNode() {}
func (Binop) exprNode()      {}
func (If) exprNode()         {}
func (Let) exprNode()        {}
func (Fun) exprNode()        {}
func (App) exprNode()        {}
func (Match) exprNode()      {}
func (TypeDecl) exprNode()   {}
func (ListLit) exprNode()    {}
func (TupleLit) exprNode()   {}
func (LetPat) exprNode()     {}
func (LetRec) exprNode()     {}
func (Import) exprNode()     {}
func (DotAccess) exprNode()  {}
func (Export) exprNode()     {}
func (TraitDecl) exprNode()  {}
func (ImplDecl) exprNode()   {}
func (TestDecl) exprNode()   {}
func (Assert) exprNode()     {}

// ---------------------------------------------------------------------------
// Type syntax nodes
// ---------------------------------------------------------------------------

type TyName struct{ Name string }
type TyApp struct {
	Name string
	Args []TySyntax
}
type TyFun struct {
	Arg TySyntax
	Ret TySyntax
}
type TyList struct{ Elem TySyntax }
type TyTuple struct{ Elems []TySyntax }
type TyUnit struct{}

func (TyName) tySyntaxNode()  {}
func (TyApp) tySyntaxNode()   {}
func (TyFun) tySyntaxNode()   {}
func (TyList) tySyntaxNode()  {}
func (TyTuple) tySyntaxNode() {}
func (TyUnit) tySyntaxNode()  {}
