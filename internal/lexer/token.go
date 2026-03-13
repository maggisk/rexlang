package lexer

import "fmt"

// TokenKind constants - keyword/symbol strings + special kinds
const (
	// Literal kinds
	TokInt    = "int"
	TokFloat  = "float"
	TokString = "string"
	TokInterp       = "interp"
	TokTaggedInterp = "taginterp"
	TokBool   = "bool"
	TokIdent  = "ident"
	TokEOF    = "eof"

	// Keywords
	TokLet       = "let"
	TokRec       = "rec"
	TokAnd       = "and"
	TokIn        = "in"
	TokIf        = "if"
	TokThen      = "then"
	TokElse      = "else"
	TokBackslash = "\\"
	TokMatch     = "match"
	TokWhen      = "when"
	TokType      = "type"
	TokImport    = "import"
	TokExport    = "export"
	TokExternal  = "external"
	TokAs        = "as"
	TokTrait     = "trait"
	TokImpl      = "impl"
	TokWhere     = "where"
	TokAlias     = "alias"
	TokTest      = "test"
	TokAssert    = "assert"
	TokOpaque    = "opaque"

	// Symbols
	TokPlus       = "+"
	TokPlusPlus   = "++"
	TokMinus      = "-"
	TokStar       = "*"
	TokSlash      = "/"
	TokPercent    = "%"
	TokEq         = "="
	TokEqEq       = "=="
	TokBangEq     = "!="
	TokLt         = "<"
	TokLtEq       = "<="
	TokGt         = ">"
	TokGtEq       = ">="
	TokAmpAmp     = "&&"
	TokPipePipe   = "||"
	TokPipeGt     = "|>"
	TokPipe       = "|"
	TokArrow      = "->"
	TokFatArrow   = "=>"
	TokColonColon = "::"
	TokColon      = ":"
	TokDot        = "."
	TokComma      = ","
	TokLParen     = "("
	TokRParen     = ")"
	TokLBrack     = "["
	TokRBrack     = "]"
	TokLBrace     = "{"
	TokRBrace     = "}"
)

// Token represents a lexed token.
type Token struct {
	Kind  string
	Value interface{} // int, float64, string, bool, or nil
	Line  int
	Col   int
}

// InterpPart represents one segment of an interpolated string.
type InterpPart struct {
	Literal bool    // true = plain string text
	Str     string  // the text when Literal == true
	Tokens  []Token // tokenized expression when Literal == false
}

// TaggedInterpValue holds the tag name and parts for a tagged template.
type TaggedInterpValue struct {
	Tag   string
	Parts []InterpPart
}

func (t Token) String() string {
	if t.Value != nil {
		return fmt.Sprintf("%s(%v)", t.Kind, t.Value)
	}
	return t.Kind
}
