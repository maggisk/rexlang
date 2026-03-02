// Package parser implements the recursive descent parser for RexLang.
package parser

import (
	"fmt"
	"unicode"

	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/lexer"
)

// ParseError is returned for parser errors.
type ParseError struct {
	Msg  string
	Line int
	Col  int
}

func (e *ParseError) Error() string {
	return e.Msg
}

type parser struct {
	tokens     []lexer.Token
	pos        int
	caseArmCol int // offside rule column; -1 = unrestricted
	letBindCol int // exact-match offside for multi-binding let; -1 = inactive
}

func isUppercase(s string) bool {
	if s == "" {
		return false
	}
	return unicode.IsUpper([]rune(s)[0])
}

func (p *parser) peek() lexer.Token {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return p.tokens[len(p.tokens)-1]
}

func (p *parser) advance() {
	p.pos++
}

func (p *parser) expect(kind string) error {
	tok := p.peek()
	if tok.Kind == kind {
		p.advance()
		return nil
	}
	return &ParseError{
		Msg:  fmt.Sprintf("expected '%s', got '%s' at line %d, col %d", kind, tok, tok.Line, tok.Col+1),
		Line: tok.Line,
		Col:  tok.Col,
	}
}

func (p *parser) expectIdent() (string, error) {
	tok := p.peek()
	if tok.Kind == lexer.TokIdent {
		p.advance()
		return tok.Value.(string), nil
	}
	return "", &ParseError{
		Msg:  fmt.Sprintf("expected identifier, got '%s' at line %d, col %d", tok, tok.Line, tok.Col+1),
		Line: tok.Line,
		Col:  tok.Col,
	}
}

// ---------------------------------------------------------------------------
// Atom parsing
// ---------------------------------------------------------------------------

func (p *parser) parseAtom() (ast.Expr, error) {
	tok := p.peek()
	switch tok.Kind {
	case lexer.TokInt:
		p.advance()
		return ast.IntLit{Value: tok.Value.(int)}, nil
	case lexer.TokFloat:
		p.advance()
		return ast.FloatLit{Value: tok.Value.(float64)}, nil
	case lexer.TokString:
		p.advance()
		return ast.StringLit{Value: tok.Value.(string)}, nil
	case lexer.TokInterp:
		p.advance()
		parts := tok.Value.([]lexer.InterpPart)
		var exprParts []ast.Expr
		for _, part := range parts {
			if part.Literal {
				exprParts = append(exprParts, ast.StringLit{Value: part.Str})
			} else {
				subParser := &parser{tokens: part.Tokens, pos: 0, caseArmCol: -1, letBindCol: -1}
				expr, err := subParser.parseExpr()
				if err != nil {
					return nil, err
				}
				exprParts = append(exprParts, expr)
			}
		}
		return ast.StringInterp{Parts: exprParts}, nil
	case lexer.TokBool:
		p.advance()
		return ast.BoolLit{Value: tok.Value.(bool)}, nil
	case lexer.TokIdent:
		p.advance()
		name := tok.Value.(string)
		// Uppercase ident + '{' = record construction
		if isUppercase(name) && p.peek().Kind == lexer.TokLBrace {
			p.advance() // consume '{'
			var fields []ast.RecordFieldExpr
			for p.peek().Kind != lexer.TokRBrace {
				fname, err := p.expectIdent()
				if err != nil {
					return nil, err
				}
				if err := p.expect(lexer.TokEq); err != nil {
					return nil, err
				}
				val, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				fields = append(fields, ast.RecordFieldExpr{Name: fname, Value: val})
				if p.peek().Kind == lexer.TokComma {
					p.advance()
				}
			}
			if err := p.expect(lexer.TokRBrace); err != nil {
				return nil, err
			}
			return ast.RecordCreate{TypeName: name, Fields: fields}, nil
		}
		if p.peek().Kind == lexer.TokDot {
			if isUppercase(name) {
				// Module access: M.foo
				p.advance()
				field, err := p.expectIdent()
				if err != nil {
					return nil, err
				}
				return ast.DotAccess{ModuleName: name, FieldName: field}, nil
			}
			// Record field access: expr.field (with chaining: expr.field.field2)
			var result ast.Expr = ast.Var{Name: name}
			for p.peek().Kind == lexer.TokDot {
				p.advance()
				field, err := p.expectIdent()
				if err != nil {
					return nil, err
				}
				result = ast.FieldAccess{Record: result, Field: field}
			}
			return result, nil
		}
		return ast.Var{Name: name}, nil
	case lexer.TokLBrace:
		// Record update: { expr | field = val, ... }
		p.advance() // consume '{'
		savedCAC := p.caseArmCol
		p.caseArmCol = -1
		recExpr, err := p.parseExpr()
		if err != nil {
			p.caseArmCol = savedCAC
			return nil, err
		}
		p.caseArmCol = savedCAC
		if err := p.expect(lexer.TokPipe); err != nil {
			return nil, err
		}
		var updates []ast.RecordFieldUpdate
		for {
			fname, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			path := []string{fname}
			for p.peek().Kind == lexer.TokDot {
				p.advance() // consume '.'
				next, err := p.expectIdent()
				if err != nil {
					return nil, err
				}
				path = append(path, next)
			}
			if err := p.expect(lexer.TokEq); err != nil {
				return nil, err
			}
			val, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			updates = append(updates, ast.RecordFieldUpdate{Path: path, Value: val})
			if p.peek().Kind == lexer.TokComma {
				p.advance()
			} else {
				break
			}
		}
		if err := p.expect(lexer.TokRBrace); err != nil {
			return nil, err
		}
		return ast.RecordUpdate{Record: recExpr, Updates: updates}, nil

	case lexer.TokLParen:
		p.advance()
		if p.peek().Kind == lexer.TokRParen {
			p.advance()
			return ast.UnitLit{}, nil
		}
		first, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		if p.peek().Kind == lexer.TokComma {
			items := []ast.Expr{first}
			for p.peek().Kind == lexer.TokComma {
				p.advance()
				item, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				items = append(items, item)
			}
			if err := p.expect(lexer.TokRParen); err != nil {
				return nil, err
			}
			return ast.TupleLit{Items: items}, nil
		}
		if err := p.expect(lexer.TokRParen); err != nil {
			return nil, err
		}
		return first, nil
	case lexer.TokLBrack:
		p.advance()
		if p.peek().Kind == lexer.TokRBrack {
			p.advance()
			return ast.ListLit{Items: nil}, nil
		}
		items := []ast.Expr{}
		item, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		for p.peek().Kind == lexer.TokComma {
			p.advance()
			item, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		if err := p.expect(lexer.TokRBrack); err != nil {
			return nil, err
		}
		return ast.ListLit{Items: items}, nil
	default:
		return nil, &ParseError{
			Msg:  fmt.Sprintf("unexpected token: '%s' at line %d, col %d", tok, tok.Line, tok.Col+1),
			Line: tok.Line,
			Col:  tok.Col,
		}
	}
}

// ---------------------------------------------------------------------------
// Application
// ---------------------------------------------------------------------------

func isAtomStart(kind string) bool {
	switch kind {
	case lexer.TokInt, lexer.TokFloat, lexer.TokString, lexer.TokInterp, lexer.TokBool,
		lexer.TokIdent, lexer.TokLParen, lexer.TokLBrack, lexer.TokLBrace:
		return true
	}
	return false
}

func (p *parser) parseApp() (ast.Expr, error) {
	f, err := p.parseAtom()
	if err != nil {
		return nil, err
	}
	for isAtomStart(p.peek().Kind) {
		if p.caseArmCol >= 0 && p.peek().Col <= p.caseArmCol {
			break
		}
		if p.letBindCol >= 0 && p.peek().Kind == lexer.TokIdent && p.peek().Col == p.letBindCol {
			break
		}
		arg, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		f = ast.App{Func: f, Arg: arg}
	}
	return f, nil
}

// ---------------------------------------------------------------------------
// Unary minus
// ---------------------------------------------------------------------------

func (p *parser) parseUnary() (ast.Expr, error) {
	if p.peek().Kind == lexer.TokMinus {
		p.advance()
		e, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return ast.UnaryMinus{Expr: e}, nil
	}
	return p.parseApp()
}

// ---------------------------------------------------------------------------
// Multiplicative
// ---------------------------------------------------------------------------

func (p *parser) parseMult() (ast.Expr, error) {
	lhs, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		k := p.peek().Kind
		if k == lexer.TokStar || k == lexer.TokSlash || k == lexer.TokPercent {
			p.advance()
			rhs, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			op := map[string]string{
				lexer.TokStar:    "Mul",
				lexer.TokSlash:   "Div",
				lexer.TokPercent: "Mod",
			}[k]
			lhs = ast.Binop{Op: op, Left: lhs, Right: rhs}
		} else {
			break
		}
	}
	return lhs, nil
}

// ---------------------------------------------------------------------------
// Additive
// ---------------------------------------------------------------------------

func (p *parser) parseAdd() (ast.Expr, error) {
	lhs, err := p.parseMult()
	if err != nil {
		return nil, err
	}
	for {
		k := p.peek().Kind
		if k == lexer.TokPlusPlus || k == lexer.TokPlus || k == lexer.TokMinus {
			p.advance()
			rhs, err := p.parseMult()
			if err != nil {
				return nil, err
			}
			op := map[string]string{
				lexer.TokPlusPlus: "Concat",
				lexer.TokPlus:     "Add",
				lexer.TokMinus:    "Sub",
			}[k]
			lhs = ast.Binop{Op: op, Left: lhs, Right: rhs}
		} else {
			break
		}
	}
	return lhs, nil
}

// ---------------------------------------------------------------------------
// Cons (right-associative)
// ---------------------------------------------------------------------------

func (p *parser) parseCons() (ast.Expr, error) {
	lhs, err := p.parseAdd()
	if err != nil {
		return nil, err
	}
	if p.peek().Kind == lexer.TokColonColon {
		p.advance()
		rhs, err := p.parseCons()
		if err != nil {
			return nil, err
		}
		return ast.Binop{Op: "Cons", Left: lhs, Right: rhs}, nil
	}
	return lhs, nil
}

// ---------------------------------------------------------------------------
// Comparison (non-associative)
// ---------------------------------------------------------------------------

func (p *parser) parseCompare() (ast.Expr, error) {
	lhs, err := p.parsePipe()
	if err != nil {
		return nil, err
	}
	opMap := map[string]string{
		lexer.TokLt:     "Lt",
		lexer.TokGt:     "Gt",
		lexer.TokLtEq:   "Leq",
		lexer.TokGtEq:   "Geq",
		lexer.TokEqEq:   "Eq",
		lexer.TokBangEq: "Neq",
	}
	k := p.peek().Kind
	if op, ok := opMap[k]; ok {
		p.advance()
		rhs, err := p.parsePipe()
		if err != nil {
			return nil, err
		}
		return ast.Binop{Op: op, Left: lhs, Right: rhs}, nil
	}
	return lhs, nil
}

// ---------------------------------------------------------------------------
// Logical and
// ---------------------------------------------------------------------------

func (p *parser) parseLogicAnd() (ast.Expr, error) {
	lhs, err := p.parseCompare()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == lexer.TokAmpAmp {
		p.advance()
		rhs, err := p.parseCompare()
		if err != nil {
			return nil, err
		}
		lhs = ast.Binop{Op: "And", Left: lhs, Right: rhs}
	}
	return lhs, nil
}

// ---------------------------------------------------------------------------
// Logical or
// ---------------------------------------------------------------------------

func (p *parser) parseLogicOr() (ast.Expr, error) {
	lhs, err := p.parseLogicAnd()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == lexer.TokPipePipe {
		p.advance()
		rhs, err := p.parseLogicAnd()
		if err != nil {
			return nil, err
		}
		lhs = ast.Binop{Op: "Or", Left: lhs, Right: rhs}
	}
	return lhs, nil
}

// ---------------------------------------------------------------------------
// Pipe: x |> f => App(f, x)
// ---------------------------------------------------------------------------

func (p *parser) parsePipe() (ast.Expr, error) {
	lhs, err := p.parseCons()
	if err != nil {
		return nil, err
	}
	for p.peek().Kind == lexer.TokPipeGt {
		p.advance()
		rhs, err := p.parseCons()
		if err != nil {
			return nil, err
		}
		lhs = ast.App{Func: rhs, Arg: lhs}
	}
	return lhs, nil
}

// ---------------------------------------------------------------------------
// Let
// ---------------------------------------------------------------------------

func (p *parser) parseLet() (ast.Expr, error) {
	p.advance() // consume 'let'
	if p.peek().Kind == lexer.TokLParen {
		pat, err := p.parseAtomPattern()
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.TokEq); err != nil {
			return nil, err
		}
		body, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		var inExpr ast.Expr
		if p.peek().Kind == lexer.TokIn {
			p.advance()
			inExpr, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
		}
		return ast.LetPat{Pat: pat, Body: body, InExpr: inExpr}, nil
	}

	recursive := p.peek().Kind == lexer.TokRec
	if recursive {
		p.advance()
	}
	nameCol := p.peek().Col
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	var params []string
	for p.peek().Kind == lexer.TokIdent {
		param, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		params = append(params, param)
	}
	if err := p.expect(lexer.TokEq); err != nil {
		return nil, err
	}

	// For non-recursive lets, set exact-match offside rule so body parsing
	// stops at an ident at nameCol (the next binding in a multi-binding let).
	// Uses letBindCol (exact match) instead of caseArmCol (<=) to allow
	// arguments that wrap to columns below nameCol.
	savedLBC := p.letBindCol
	if !recursive {
		p.letBindCol = nameCol
	}

	body, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	// Desugar parameters: let f x y = body => let f = fn x -> fn y -> body
	for i := len(params) - 1; i >= 0; i-- {
		body = ast.Fun{Param: params[i], Body: body}
	}

	// Mutual recursion: let rec f ... = ... and g ... = ...
	if recursive && p.peek().Kind == lexer.TokAnd {
		bindings := []ast.LetRecBinding{{Name: name, Body: body}}
		for p.peek().Kind == lexer.TokAnd {
			p.advance()
			name2, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			var params2 []string
			for p.peek().Kind == lexer.TokIdent {
				param, err := p.expectIdent()
				if err != nil {
					return nil, err
				}
				params2 = append(params2, param)
			}
			if err := p.expect(lexer.TokEq); err != nil {
				return nil, err
			}
			body2, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			for i := len(params2) - 1; i >= 0; i-- {
				body2 = ast.Fun{Param: params2[i], Body: body2}
			}
			bindings = append(bindings, ast.LetRecBinding{Name: name2, Body: body2})
		}
		var inExpr ast.Expr
		if p.peek().Kind == lexer.TokIn {
			p.advance()
			inExpr, err = p.parseExpr()
			if err != nil {
				return nil, err
			}
		}
		return ast.LetRec{Bindings: bindings, InExpr: inExpr}, nil
	}

	// Multi-binding let: additional bindings at the same column as the first name
	type letBinding struct {
		name string
		body ast.Expr
	}
	bindings := []letBinding{{name, body}}

	if !recursive {
		for p.peek().Col == nameCol && p.peek().Kind == lexer.TokIdent {
			name2, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			var params2 []string
			for p.peek().Kind == lexer.TokIdent {
				param, err := p.expectIdent()
				if err != nil {
					return nil, err
				}
				params2 = append(params2, param)
			}
			if err := p.expect(lexer.TokEq); err != nil {
				return nil, err
			}
			body2, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			for i := len(params2) - 1; i >= 0; i-- {
				body2 = ast.Fun{Param: params2[i], Body: body2}
			}
			bindings = append(bindings, letBinding{name2, body2})
		}
	}

	// Restore offside rule before parsing inExpr
	p.letBindCol = savedLBC

	var inExpr ast.Expr
	if p.peek().Kind == lexer.TokIn {
		p.advance()
		inExpr, err = p.parseExpr()
		if err != nil {
			return nil, err
		}
	}

	// Desugar multi-binding: [a=1, b=2, c=3] + inExpr → Let{a,1, Let{b,2, Let{c,3, inExpr}}}
	result := ast.Let{Name: bindings[len(bindings)-1].name, Body: bindings[len(bindings)-1].body, InExpr: inExpr}
	for i := len(bindings) - 2; i >= 0; i-- {
		result = ast.Let{Name: bindings[i].name, Body: bindings[i].body, InExpr: result}
	}
	// Preserve Recursive flag on the outermost Let (only relevant for single-binding let rec)
	result.Recursive = recursive
	return result, nil
}

// ---------------------------------------------------------------------------
// If
// ---------------------------------------------------------------------------

func (p *parser) parseIf() (ast.Expr, error) {
	p.advance() // consume 'if'
	cond, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TokThen); err != nil {
		return nil, err
	}
	then, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TokElse); err != nil {
		return nil, err
	}
	els, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	return ast.If{Cond: cond, ThenExpr: then, ElseExpr: els}, nil
}

// ---------------------------------------------------------------------------
// Fun
// ---------------------------------------------------------------------------

func (p *parser) parseFun() (ast.Expr, error) {
	p.advance() // consume 'fn'
	var params []string
	for p.peek().Kind != lexer.TokArrow {
		if p.peek().Kind == lexer.TokIdent {
			param, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			params = append(params, param)
		} else {
			tok := p.peek()
			return nil, &ParseError{
				Msg:  fmt.Sprintf("expected parameter or '->', got '%s' at line %d, col %d", tok, tok.Line, tok.Col+1),
				Line: tok.Line,
				Col:  tok.Col,
			}
		}
	}
	arrow := p.peek()
	p.advance() // consume '->'
	if len(params) == 0 {
		return nil, &ParseError{
			Msg:  fmt.Sprintf("fn requires at least one parameter at line %d, col %d", arrow.Line, arrow.Col+1),
			Line: arrow.Line,
			Col:  arrow.Col,
		}
	}
	body, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	for i := len(params) - 1; i >= 0; i-- {
		body = ast.Fun{Param: params[i], Body: body}
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// Patterns
// ---------------------------------------------------------------------------

func (p *parser) parseAtomPattern() (ast.Pattern, error) {
	tok := p.peek()
	if tok.Kind == lexer.TokIdent {
		name := tok.Value.(string)
		if name == "_" {
			p.advance()
			return ast.PWild{}, nil
		}
		if !isUppercase(name) {
			p.advance()
			return ast.PVar{Name: name}, nil
		}
		// uppercase ident — fall through to error (caller must handle via parsePattern)
	}
	switch tok.Kind {
	case lexer.TokInt:
		p.advance()
		return ast.PInt{Value: tok.Value.(int)}, nil
	case lexer.TokFloat:
		p.advance()
		return ast.PFloat{Value: tok.Value.(float64)}, nil
	case lexer.TokString:
		p.advance()
		return ast.PString{Value: tok.Value.(string)}, nil
	case lexer.TokBool:
		p.advance()
		return ast.PBool{Value: tok.Value.(bool)}, nil
	case lexer.TokLParen:
		p.advance()
		if p.peek().Kind == lexer.TokRParen {
			p.advance()
			return ast.PUnit{}, nil
		}
		first, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		if p.peek().Kind == lexer.TokComma {
			pats := []ast.Pattern{first}
			for p.peek().Kind == lexer.TokComma {
				p.advance()
				pat, err := p.parsePattern()
				if err != nil {
					return nil, err
				}
				pats = append(pats, pat)
			}
			if err := p.expect(lexer.TokRParen); err != nil {
				return nil, err
			}
			return ast.PTuple{Pats: pats}, nil
		}
		if err := p.expect(lexer.TokRParen); err != nil {
			return nil, err
		}
		return first, nil
	case lexer.TokLBrack:
		p.advance()
		if p.peek().Kind == lexer.TokRBrack {
			p.advance()
			return ast.PNil{}, nil
		}
		items := []ast.Pattern{}
		item, err := p.parseAtomPattern()
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		for p.peek().Kind == lexer.TokComma {
			p.advance()
			item, err := p.parseAtomPattern()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		var tail ast.Pattern = ast.PNil{}
		if p.peek().Kind == lexer.TokPipe {
			p.advance()
			tail, err = p.parseAtomPattern()
			if err != nil {
				return nil, err
			}
		}
		if err := p.expect(lexer.TokRBrack); err != nil {
			return nil, err
		}
		result := tail
		for i := len(items) - 1; i >= 0; i-- {
			result = ast.PCons{Head: items[i], Tail: result}
		}
		return result, nil
	}
	return nil, &ParseError{
		Msg:  fmt.Sprintf("expected pattern, got '%s' at line %d, col %d", tok, tok.Line, tok.Col+1),
		Line: tok.Line,
		Col:  tok.Col,
	}
}

func isPatternAtomStart(kind string, value interface{}) bool {
	if kind == lexer.TokIdent {
		s := value.(string)
		return s == "_" || !isUppercase(s)
	}
	switch kind {
	case lexer.TokInt, lexer.TokFloat, lexer.TokString, lexer.TokBool,
		lexer.TokLParen, lexer.TokLBrack:
		return true
	}
	return false
}

func (p *parser) parsePattern() (ast.Pattern, error) {
	tok := p.peek()
	if tok.Kind == lexer.TokIdent && isUppercase(tok.Value.(string)) {
		p.advance()
		name := tok.Value.(string)
		// Record pattern: Person { name = n, age = a }
		if p.peek().Kind == lexer.TokLBrace {
			p.advance() // consume '{'
			var fields []ast.PRecordField
			for p.peek().Kind != lexer.TokRBrace {
				fname, err := p.expectIdent()
				if err != nil {
					return nil, err
				}
				if err := p.expect(lexer.TokEq); err != nil {
					return nil, err
				}
				pat, err := p.parsePattern()
				if err != nil {
					return nil, err
				}
				fields = append(fields, ast.PRecordField{Name: fname, Pat: pat})
				if p.peek().Kind == lexer.TokComma {
					p.advance()
				}
			}
			if err := p.expect(lexer.TokRBrace); err != nil {
				return nil, err
			}
			return ast.PRecord{TypeName: name, Fields: fields}, nil
		}
		var args []ast.Pattern
		for {
			t := p.peek()
			if isPatternAtomStart(t.Kind, t.Value) {
				arg, err := p.parseAtomPattern()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			} else if t.Kind == lexer.TokIdent && isUppercase(t.Value.(string)) {
				// Uppercase in parens is handled in parseAtomPattern via TokLParen
				break
			} else {
				break
			}
		}
		return ast.PCtor{Name: name, Args: args}, nil
	}
	return p.parseAtomPattern()
}

// ---------------------------------------------------------------------------
// Case
// ---------------------------------------------------------------------------

func (p *parser) parseCase() (ast.Expr, error) {
	p.advance() // consume 'case'
	scrutinee, err := p.parseExpr()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TokOf); err != nil {
		return nil, err
	}
	// Optional leading '|'
	if p.peek().Kind == lexer.TokPipe {
		p.advance()
	}
	armCol := p.peek().Col
	saved := p.caseArmCol
	p.caseArmCol = armCol
	var arms []ast.MatchArm
	for {
		pat, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.TokArrow); err != nil {
			return nil, err
		}
		body, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		arms = append(arms, ast.MatchArm{Pat: pat, Body: body})
		tok := p.peek()
		if tok.Kind == lexer.TokEOF {
			break
		}
		if tok.Col == armCol {
			continue
		}
		break
	}
	p.caseArmCol = saved
	return ast.Match{Scrutinee: scrutinee, Arms: arms}, nil
}

// ---------------------------------------------------------------------------
// Type declaration
// ---------------------------------------------------------------------------

func (p *parser) parseTypeDecl() (ast.Expr, error) {
	p.advance() // consume 'type'
	nameTok := p.peek()
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if !isUppercase(name) {
		return nil, &ParseError{
			Msg:  fmt.Sprintf("type name must start with uppercase, got '%s' at line %d, col %d", name, nameTok.Line, nameTok.Col+1),
			Line: nameTok.Line,
			Col:  nameTok.Col,
		}
	}
	var params []string
	for p.peek().Kind == lexer.TokIdent && !isUppercase(p.peek().Value.(string)) {
		param, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		params = append(params, param)
	}
	if err := p.expect(lexer.TokEq); err != nil {
		return nil, err
	}
	// Record type: type Name = { field : Type, ... }
	if p.peek().Kind == lexer.TokLBrace {
		p.advance() // consume '{'
		var fields []ast.RecordFieldDef
		for p.peek().Kind != lexer.TokRBrace {
			fname, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			if err := p.expect(lexer.TokColon); err != nil {
				return nil, err
			}
			ftype, err := p.parseTypeSig()
			if err != nil {
				return nil, err
			}
			fields = append(fields, ast.RecordFieldDef{Name: fname, Type: ftype})
			if p.peek().Kind == lexer.TokComma {
				p.advance()
			}
		}
		if err := p.expect(lexer.TokRBrace); err != nil {
			return nil, err
		}
		return ast.TypeDecl{Name: name, Params: params, RecordFields: fields}, nil
	}
	// Alias detection: try parsing as type signature, then check for |
	if p.peek().Kind != lexer.TokPipe {
		savedPos := p.pos
		ty, err := p.parseTypeSig()
		if err == nil && p.peek().Kind != lexer.TokPipe {
			// No | follows — this is a type alias
			return ast.TypeDecl{Name: name, Params: params, AliasType: ty}, nil
		}
		// Has | or parse failed — restore and parse as ADT
		p.pos = savedPos
	}

	if p.peek().Kind == lexer.TokPipe {
		p.advance()
	}
	var ctors []ast.CtorDef
	for {
		ctorName, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		var argTypes []ast.TySyntax
		for {
			if p.caseArmCol >= 0 && p.peek().Col <= p.caseArmCol {
				break
			}
			tok := p.peek()
			if tok.Kind == lexer.TokLParen || tok.Kind == lexer.TokLBrack {
				ty, err := p.parseTypeSigAtom()
				if err != nil {
					return nil, err
				}
				argTypes = append(argTypes, ty)
			} else if tok.Kind == lexer.TokIdent {
				p.advance()
				argTypes = append(argTypes, ast.TyName{Name: tok.Value.(string)})
			} else {
				break
			}
		}
		ctors = append(ctors, ast.CtorDef{Name: ctorName, ArgTypes: argTypes})
		if p.peek().Kind == lexer.TokPipe {
			p.advance()
		} else {
			break
		}
	}
	return ast.TypeDecl{Name: name, Params: params, Ctors: ctors}, nil
}

// ---------------------------------------------------------------------------
// Import
// ---------------------------------------------------------------------------

func (p *parser) parseImport() (ast.Expr, error) {
	p.advance() // consume 'import'
	nsOrName, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	var module string
	if p.peek().Kind == lexer.TokColon {
		p.advance()
		rest, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		module = nsOrName + ":" + rest
	} else {
		module = nsOrName
	}
	if p.peek().Kind == lexer.TokAs {
		p.advance()
		alias, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		return ast.Import{Module: module, Names: nil, Alias: alias}, nil
	}
	if err := p.expect(lexer.TokLParen); err != nil {
		return nil, err
	}
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	names := []string{name}
	for p.peek().Kind == lexer.TokComma {
		p.advance()
		n, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	if err := p.expect(lexer.TokRParen); err != nil {
		return nil, err
	}
	return ast.Import{Module: module, Names: names}, nil
}

// ---------------------------------------------------------------------------
// Export
// ---------------------------------------------------------------------------

func (p *parser) parseExport() (ast.Expr, error) {
	p.advance() // consume 'export'
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	names := []string{name}
	for p.peek().Kind == lexer.TokComma {
		p.advance()
		n, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return ast.Export{Names: names}, nil
}

// ---------------------------------------------------------------------------
// Type signature (for trait method signatures)
// ---------------------------------------------------------------------------

func (p *parser) parseTypeSig() (ast.TySyntax, error) {
	ty, err := p.parseTypeSigAtom()
	if err != nil {
		return nil, err
	}
	if p.peek().Kind == lexer.TokArrow {
		p.advance()
		ret, err := p.parseTypeSig()
		if err != nil {
			return nil, err
		}
		return ast.TyFun{Arg: ty, Ret: ret}, nil
	}
	return ty, nil
}

func (p *parser) isTypeSigAtomStart() bool {
	switch p.peek().Kind {
	case lexer.TokIdent, lexer.TokLParen, lexer.TokLBrack, lexer.TokLBrace:
		return true
	}
	return false
}

func (p *parser) parseTypeSigAtom() (ast.TySyntax, error) {
	tok := p.peek()
	switch tok.Kind {
	case lexer.TokLBrace:
		p.advance() // consume '{'
		var fields []ast.TyRecordField
		for p.peek().Kind != lexer.TokRBrace {
			fname, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			if err := p.expect(lexer.TokColon); err != nil {
				return nil, err
			}
			ftype, err := p.parseTypeSig()
			if err != nil {
				return nil, err
			}
			fields = append(fields, ast.TyRecordField{Name: fname, Type: ftype})
			if p.peek().Kind == lexer.TokComma {
				p.advance()
			}
		}
		if err := p.expect(lexer.TokRBrace); err != nil {
			return nil, err
		}
		return ast.TyRecord{Fields: fields}, nil
	case lexer.TokLParen:
		p.advance()
		if p.peek().Kind == lexer.TokRParen {
			p.advance()
			return ast.TyUnit{}, nil
		}
		first, err := p.parseTypeSig()
		if err != nil {
			return nil, err
		}
		if p.peek().Kind == lexer.TokComma {
			elems := []ast.TySyntax{first}
			for p.peek().Kind == lexer.TokComma {
				p.advance()
				e, err := p.parseTypeSig()
				if err != nil {
					return nil, err
				}
				elems = append(elems, e)
			}
			if err := p.expect(lexer.TokRParen); err != nil {
				return nil, err
			}
			return ast.TyTuple{Elems: elems}, nil
		}
		if err := p.expect(lexer.TokRParen); err != nil {
			return nil, err
		}
		return first, nil
	case lexer.TokLBrack:
		p.advance()
		elem, err := p.parseTypeSig()
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.TokRBrack); err != nil {
			return nil, err
		}
		return ast.TyList{Elem: elem}, nil
	case lexer.TokIdent:
		p.advance()
		name := tok.Value.(string)
		if isUppercase(name) {
			var args []ast.TySyntax
			for p.isTypeSigAtomStart() {
				if p.caseArmCol >= 0 && p.peek().Col <= p.caseArmCol {
					break
				}
				if p.peek().Kind == lexer.TokIdent {
					argName := p.peek().Value.(string)
					if argName == "where" {
						break
					}
					// Bare ident in arg position — don't recurse into type application
					p.advance()
					args = append(args, ast.TyName{Name: argName})
					continue
				}
				// Parens, brackets, braces — parse fully
				arg, err := p.parseTypeSigAtom()
				if err != nil {
					return nil, err
				}
				args = append(args, arg)
			}
			if len(args) > 0 {
				return ast.TyApp{Name: name, Args: args}, nil
			}
			return ast.TyName{Name: name}, nil
		}
		return ast.TyName{Name: name}, nil
	default:
		return nil, &ParseError{
			Msg:  fmt.Sprintf("expected type, got '%s' at line %d, col %d", tok, tok.Line, tok.Col+1),
			Line: tok.Line,
			Col:  tok.Col,
		}
	}
}

// ---------------------------------------------------------------------------
// Trait declaration
// ---------------------------------------------------------------------------

func (p *parser) parseTraitDecl() (ast.Expr, error) {
	p.advance() // consume 'trait'
	traitName, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if !isUppercase(traitName) {
		return nil, &ParseError{Msg: fmt.Sprintf("trait name must start with uppercase, got '%s'", traitName)}
	}
	param, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TokWhere); err != nil {
		return nil, err
	}
	var methods []ast.TraitMethod
	methodCol := p.peek().Col
	for p.peek().Kind == lexer.TokIdent && p.peek().Col >= methodCol {
		mname, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.TokColon); err != nil {
			return nil, err
		}
		mtype, err := p.parseTypeSig()
		if err != nil {
			return nil, err
		}
		methods = append(methods, ast.TraitMethod{Name: mname, Type: mtype})
	}
	if len(methods) == 0 {
		return nil, &ParseError{Msg: fmt.Sprintf("trait '%s' must have at least one method", traitName)}
	}
	return ast.TraitDecl{Name: traitName, Param: param, Methods: methods}, nil
}

// ---------------------------------------------------------------------------
// Impl declaration
// ---------------------------------------------------------------------------

func (p *parser) parseImpl() (ast.Expr, error) {
	p.advance() // consume 'impl'
	traitName, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	targetType, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TokWhere); err != nil {
		return nil, err
	}
	var methods []ast.ImplMethod
	methodCol := p.peek().Col
	saved := p.caseArmCol
	p.caseArmCol = methodCol
	for p.peek().Kind == lexer.TokIdent && p.peek().Col >= methodCol {
		mname, err := p.expectIdent()
		if err != nil {
			return nil, err
		}
		var params []string
		for p.peek().Kind == lexer.TokIdent && p.peek().Kind != lexer.TokEq {
			param, err := p.expectIdent()
			if err != nil {
				return nil, err
			}
			params = append(params, param)
		}
		if err := p.expect(lexer.TokEq); err != nil {
			return nil, err
		}
		body, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		for i := len(params) - 1; i >= 0; i-- {
			body = ast.Fun{Param: params[i], Body: body}
		}
		methods = append(methods, ast.ImplMethod{Name: mname, Body: body})
	}
	p.caseArmCol = saved
	return ast.ImplDecl{TraitName: traitName, TargetType: targetType, Methods: methods}, nil
}

// ---------------------------------------------------------------------------
// Test declaration
// ---------------------------------------------------------------------------

func (p *parser) parseTest() (ast.Expr, error) {
	p.advance() // consume 'test'
	tok := p.peek()
	if tok.Kind != lexer.TokString {
		return nil, &ParseError{
			Msg:  fmt.Sprintf("expected test name string, got '%s' at line %d, col %d", tok, tok.Line, tok.Col+1),
			Line: tok.Line,
			Col:  tok.Col,
		}
	}
	name := tok.Value.(string)
	p.advance()
	if err := p.expect(lexer.TokEq); err != nil {
		return nil, err
	}
	bodyCol := p.peek().Col
	saved := p.caseArmCol
	p.caseArmCol = bodyCol
	var body []ast.Expr
	for p.peek().Kind != lexer.TokEOF && p.peek().Col >= bodyCol {
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		body = append(body, e)
	}
	p.caseArmCol = saved
	if len(body) == 0 {
		return nil, &ParseError{Msg: fmt.Sprintf("test '%s' has empty body", name)}
	}
	return ast.TestDecl{Name: name, Body: body}, nil
}

// ---------------------------------------------------------------------------
// Assert
// ---------------------------------------------------------------------------

func (p *parser) parseAssert() (ast.Expr, error) {
	tok := p.peek()
	line := tok.Line
	p.advance() // consume 'assert'
	expr, err := p.parseLogicOr()
	if err != nil {
		return nil, err
	}
	return ast.Assert{Expr: expr, Line: line}, nil
}

// ---------------------------------------------------------------------------
// Top-level dispatch
// ---------------------------------------------------------------------------

func (p *parser) parseExpr() (ast.Expr, error) {
	k := p.peek().Kind
	switch k {
	case lexer.TokLet:
		return p.parseLet()
	case lexer.TokIf:
		return p.parseIf()
	case lexer.TokFn:
		return p.parseFun()
	case lexer.TokCase:
		return p.parseCase()
	case lexer.TokType:
		return p.parseTypeDecl()
	case lexer.TokImport:
		return p.parseImport()
	case lexer.TokExport:
		return p.parseExport()
	case lexer.TokTrait:
		return p.parseTraitDecl()
	case lexer.TokImpl:
		return p.parseImpl()
	case lexer.TokTest:
		return p.parseTest()
	case lexer.TokAssert:
		return p.parseAssert()
	default:
		return p.parseLogicOr()
	}
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// parseTypeAnnotation parses "name : TypeSig" at the top level.
func (p *parser) parseTypeAnnotation() (ast.Expr, error) {
	name, err := p.expectIdent()
	if err != nil {
		return nil, err
	}
	if err := p.expect(lexer.TokColon); err != nil {
		return nil, err
	}
	ty, err := p.parseTypeSig()
	if err != nil {
		return nil, err
	}
	return ast.TypeAnnotation{Name: name, Type: ty}, nil
}

// ParseTokens parses a token list into a list of top-level expressions.
func ParseTokens(tokens []lexer.Token) ([]ast.Expr, error) {
	p := &parser{tokens: tokens, pos: 0, caseArmCol: -1, letBindCol: -1}
	var exprs []ast.Expr
	for {
		if p.peek().Kind == lexer.TokEOF {
			break
		}
		p.caseArmCol = 0
		// Check for type annotation: lowercase ident followed by ':'
		if p.peek().Kind == lexer.TokIdent && !isUppercase(p.peek().Value.(string)) &&
			p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == lexer.TokColon {
			ann, err := p.parseTypeAnnotation()
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, ann)
			continue
		}
		e, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		exprs = append(exprs, e)
	}
	return exprs, nil
}

// Parse tokenizes and parses source code.
func Parse(source string) ([]ast.Expr, error) {
	tokens, err := lexer.Tokenize(source)
	if err != nil {
		return nil, err
	}
	return ParseTokens(tokens)
}
