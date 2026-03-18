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

type PWild struct{}                     // _
type PUnit struct{}                     // ()
type PVar struct{ Name string }         // x
type PInt struct{ Value int }           // 42
type PFloat struct{ Value float64 }     // 3.14
type PString struct{ Value string }     // "hello"
type PBool struct{ Value bool }         // true / false
type PNil struct{}                      // []
type PCons struct{ Head, Tail Pattern } // [h|t]
type PTuple struct{ Pats []Pattern }    // (a, b)
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
func (PRecord) patternNode() {}

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
	"Neq":    "!=",
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
type Var struct {
	Name string
	Line int
}

type UnaryMinus struct {
	Expr Expr
	Line int
}

type Binop struct {
	Op    string
	Left  Expr
	Right Expr
	Line  int
}

type If struct {
	Cond     Expr
	ThenExpr Expr
	ElseExpr Expr
	Line     int
	Col      int
}

type Let struct {
	Name      string
	Recursive bool
	Exported  bool
	Body      Expr
	InExpr    Expr // nil = top-level
	Line      int
	Col       int
}

type Fun struct {
	Param string
	Body  Expr
	Line  int
}

type App struct {
	Func Expr
	Arg  Expr
	Line int
}

type Match struct {
	Scrutinee Expr
	Arms      []MatchArm
	Line      int
	Col       int
}

type MatchArm struct {
	Pat      Pattern
	Body     Expr
	Line     int // line of 'when' keyword
	Col      int // col of 'when' keyword
	BodyLine int // line of first token in body
	BodyCol  int // col of first token in body
}

type TypeDecl struct {
	Name         string
	Exported     bool
	Opaque       bool
	Params       []string
	Ctors        []CtorDef
	RecordFields []RecordFieldDef // non-nil for record types (mutually exclusive with Ctors)
	AliasType    TySyntax         // non-nil for type aliases (mutually exclusive with Ctors/RecordFields)
}

type RecordFieldDef struct {
	Name string
	Type TySyntax
}

type RecordCreate struct {
	TypeName string
	Fields   []RecordFieldExpr
	Line     int
}

type RecordFieldExpr struct {
	Name  string
	Value Expr
}

type FieldAccess struct {
	Record Expr
	Field  string
	Line   int
}

type RecordUpdate struct {
	Record  Expr
	Updates []RecordFieldUpdate
	Line    int
	Ty      interface{} // set by typechecker (types.Type — the record's concrete type)
}

type RecordFieldUpdate struct {
	Path  []string // ["name"] for flat, ["user", "name"] for nested
	Value Expr
}

type CtorDef struct {
	Name     string
	ArgTypes []TySyntax
}

type StringInterp struct {
	Parts []Expr
	Line  int
}

type TaggedTemplate struct {
	Tag     string
	Strings []string // literal string fragments (len = len(Values) + 1)
	Values  []Expr   // interpolated expressions
	Line    int
}

type ListLit struct {
	Items []Expr
	Line  int
}
type TupleLit struct {
	Items []Expr
	Line  int
}

type LetPat struct {
	Exported bool
	Pat      Pattern
	Body     Expr
	InExpr   Expr // nil = top-level
	Line     int
	Col      int
}

type LetRec struct {
	Exported bool
	Bindings []LetRecBinding
	InExpr   Expr // nil = top-level
	Line     int
	Col      int
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
	Name     string
	Exported bool
	Param    string
	Methods  []TraitMethod
}

type TraitMethod struct {
	Name string
	Type TySyntax
}

type ImplDecl struct {
	TraitName  string
	TargetType TySyntax
	Methods    []ImplMethod
}

type ImplMethod struct {
	Name string
	Body Expr
}

// Test declarations

type TestDecl struct {
	Name string
	Line int
	Body []Expr
}

type Assert struct {
	Expr Expr
	Line int
}

// Type annotations (optional, for documentation / checking)

type TypeAnnotation struct {
	Name string
	Type TySyntax
}

// External declarations — functions implemented in a host language (Go, JS).
// The type is checked, but no Rex body exists.
type ExternalDecl struct {
	Name     string
	Type     TySyntax
	Exported bool
}

// Implement exprNode for all expression types
func (IntLit) exprNode()         {}
func (FloatLit) exprNode()       {}
func (StringLit) exprNode()      {}
func (BoolLit) exprNode()        {}
func (UnitLit) exprNode()        {}
func (Var) exprNode()            {}
func (UnaryMinus) exprNode()     {}
func (Binop) exprNode()          {}
func (If) exprNode()             {}
func (Let) exprNode()            {}
func (Fun) exprNode()            {}
func (App) exprNode()            {}
func (Match) exprNode()          {}
func (StringInterp) exprNode()    {}
func (TaggedTemplate) exprNode()  {}
func (TypeDecl) exprNode()       {}
func (ListLit) exprNode()        {}
func (TupleLit) exprNode()       {}
func (LetPat) exprNode()         {}
func (LetRec) exprNode()         {}
func (Import) exprNode()         {}
func (DotAccess) exprNode()      {}
func (Export) exprNode()         {}
func (TraitDecl) exprNode()      {}
func (ImplDecl) exprNode()       {}
func (TestDecl) exprNode()       {}
func (Assert) exprNode()         {}
func (TypeAnnotation) exprNode() {}
func (ExternalDecl) exprNode()   {}
func (RecordCreate) exprNode()   {}
func (FieldAccess) exprNode()    {}
func (RecordUpdate) exprNode()   {}

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
type TyRecord struct {
	Fields []TyRecordField
}
type TyRecordField struct {
	Name string
	Type TySyntax
}

type TyConstraint struct {
	Trait string
	Var   string
}

type TyConstrained struct {
	Constraints []TyConstraint
	Inner       TySyntax
}

func (TyName) tySyntaxNode()        {}
func (TyApp) tySyntaxNode()         {}
func (TyFun) tySyntaxNode()         {}
func (TyList) tySyntaxNode()        {}
func (TyTuple) tySyntaxNode()       {}
func (TyUnit) tySyntaxNode()        {}
func (TyRecord) tySyntaxNode()      {}
func (TyConstrained) tySyntaxNode() {}
