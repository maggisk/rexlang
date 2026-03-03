package lexer

import "fmt"

// TokenKind constants - keyword/symbol strings + special kinds
const (
	// Literal kinds
	TokInt    = "int"
	TokFloat  = "float"
	TokString = "string"
	TokInterp = "interp"
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
	TokCase      = "case"
	TokType      = "type"
	TokOf        = "of"
	TokImport    = "import"
	TokExport    = "export"
	TokAs        = "as"
	TokTrait     = "trait"
	TokImpl      = "impl"
	TokWhere     = "where"
	TokTest      = "test"
	TokAssert    = "assert"

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

func (t Token) String() string {
	if t.Value != nil {
		return fmt.Sprintf("%s(%v)", t.Kind, t.Value)
	}
	return t.Kind
}
