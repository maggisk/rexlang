package lexer

import (
	"fmt"
	"strconv"
	"unicode"
)

// LexError is returned for lexer errors.
type LexError struct {
	Msg  string
	Line int
	Col  int
}

func (e *LexError) Error() string {
	return e.Msg
}

var keywords = map[string]bool{
	"let": true, "rec": true, "and": true, "in": true,
	"if": true, "then": true, "else": true,
	"match": true, "when": true, "type": true, "import": true,
	"export": true, "as": true, "trait": true, "impl": true,
	"where": true, "test": true, "assert": true, "alias": true,
}

// Tokenize converts source code into a slice of tokens.
func Tokenize(source string) ([]Token, error) {
	runes := []rune(source)
	n := len(runes)
	pos := 0
	line := 1
	lineStart := 0
	var tokens []Token

	lexErr := func(msg string) error {
		col := pos - lineStart + 1
		return &LexError{
			Msg:  fmt.Sprintf("%s at line %d, col %d", msg, line, col),
			Line: line,
			Col:  col,
		}
	}

	skipWhitespace := func() error {
		for {
			for pos < n && (runes[pos] == ' ' || runes[pos] == '\n' || runes[pos] == '\t' || runes[pos] == '\r') {
				if runes[pos] == '\n' {
					pos++
					line++
					lineStart = pos
				} else {
					pos++
				}
			}
			if pos+1 < n && runes[pos] == '-' && runes[pos+1] == '-' {
				pos += 2
				for pos < n && runes[pos] != '\n' {
					pos++
				}
			} else {
				break
			}
		}
		return nil
	}

	for {
		if err := skipWhitespace(); err != nil {
			return nil, err
		}
		if pos >= n {
			tokens = append(tokens, Token{Kind: TokEOF, Line: line, Col: pos - lineStart})
			break
		}
		tokCol := pos - lineStart
		tokLine := line
		c := runes[pos]

		switch {
		case unicode.IsDigit(c):
			start := pos
			// Check for 0x, 0o, 0b prefixes
			if c == '0' && pos+1 < n {
				prefix := runes[pos+1]
				if prefix == 'x' || prefix == 'X' || prefix == 'o' || prefix == 'O' || prefix == 'b' || prefix == 'B' {
					pos += 2 // skip "0x" etc.
					digitStart := pos
					for pos < n && (isHexDigit(runes[pos], prefix) || runes[pos] == '_') {
						pos++
					}
					raw := string(runes[digitStart:pos])
					if err := validateUnderscores(raw); err != nil {
						return nil, lexErr(err.Error())
					}
					stripped := stripUnderscores(raw)
					if len(stripped) == 0 {
						return nil, lexErr(fmt.Sprintf("no digits after 0%c prefix", prefix))
					}
					var base int
					switch prefix {
					case 'x', 'X':
						base = 16
					case 'o', 'O':
						base = 8
					case 'b', 'B':
						base = 2
					}
					val, err := strconv.ParseInt(stripped, base, 64)
					if err != nil {
						return nil, lexErr(fmt.Sprintf("invalid number literal: %s", string(runes[start:pos])))
					}
					tokens = append(tokens, Token{Kind: TokInt, Value: int(val), Line: tokLine, Col: tokCol})
					break
				}
			}
			// Decimal integer or float
			for pos < n && (unicode.IsDigit(runes[pos]) || runes[pos] == '_') {
				pos++
			}
			if pos < n && runes[pos] == '.' {
				intPart := string(runes[start:pos])
				if err := validateUnderscores(intPart); err != nil {
					return nil, lexErr(err.Error())
				}
				if len(intPart) > 0 && intPart[len(intPart)-1] == '_' {
					return nil, lexErr("underscore before decimal point")
				}
				pos++ // skip '.'
				dotPos := pos
				for pos < n && (unicode.IsDigit(runes[pos]) || runes[pos] == '_') {
					pos++
				}
				fracPart := string(runes[dotPos:pos])
				if len(fracPart) > 0 && fracPart[0] == '_' {
					return nil, lexErr("underscore after decimal point")
				}
				if err := validateUnderscores(fracPart); err != nil {
					return nil, lexErr(err.Error())
				}
				full := stripUnderscores(string(runes[start:pos]))
				f, err := strconv.ParseFloat(full, 64)
				if err != nil {
					return nil, lexErr(fmt.Sprintf("invalid float literal: %s", string(runes[start:pos])))
				}
				tokens = append(tokens, Token{Kind: TokFloat, Value: f, Line: tokLine, Col: tokCol})
			} else {
				raw := string(runes[start:pos])
				if err := validateUnderscores(raw); err != nil {
					return nil, lexErr(err.Error())
				}
				stripped := stripUnderscores(raw)
				i, err := strconv.Atoi(stripped)
				if err != nil {
					return nil, lexErr(fmt.Sprintf("invalid integer literal: %s", string(runes[start:pos])))
				}
				tokens = append(tokens, Token{Kind: TokInt, Value: i, Line: tokLine, Col: tokCol})
			}

		case c == '"':
			pos++ // skip opening '"'
			// Check for triple-quote
			tripleQuote := pos+1 < n && runes[pos] == '"' && runes[pos+1] == '"'
			if tripleQuote {
				pos += 2 // skip the other two quotes
				// Strip first newline if present
				if pos < n && runes[pos] == '\n' {
					pos++
					line++
					lineStart = pos
				} else if pos+1 < n && runes[pos] == '\r' && runes[pos+1] == '\n' {
					pos += 2
					line++
					lineStart = pos
				}
			}
			var buf []rune
			var parts []InterpPart
			hasInterp := false
			for {
				if pos >= n {
					if tripleQuote {
						return nil, lexErr("unterminated multi-line string")
					}
					return nil, lexErr("unterminated string")
				}
				ch := runes[pos]
				pos++
				// Check for closing delimiter
				if ch == '"' {
					if tripleQuote {
						if pos+1 < n && runes[pos] == '"' && runes[pos+1] == '"' {
							pos += 2
							break
						}
						// lone " or "" inside triple-quoted string
						buf = append(buf, '"')
						continue
					}
					break
				}
				if ch == '\\' {
					if pos >= n {
						return nil, lexErr("unterminated string escape")
					}
					esc := runes[pos]
					pos++
					switch esc {
					case 'n':
						buf = append(buf, '\n')
					case 'r':
						buf = append(buf, '\r')
					case 't':
						buf = append(buf, '\t')
					case '\\':
						buf = append(buf, '\\')
					case '"':
						buf = append(buf, '"')
					case '$':
						buf = append(buf, '$')
					default:
						return nil, lexErr(fmt.Sprintf("unknown escape: \\%c", esc))
					}
				} else if ch == '$' && pos < n && runes[pos] == '{' {
					hasInterp = true
					pos++ // skip '{'
					// Save accumulated literal
					if len(buf) > 0 {
						parts = append(parts, InterpPart{Literal: true, Str: string(buf)})
						buf = nil
					}
					// Find matching '}' using mutual recursion helpers
					exprStart := pos
					if err := skipInterp(runes, &pos, n); err != nil {
						return nil, lexErr(err.Error())
					}
					exprSrc := string(runes[exprStart : pos-1]) // exclude closing '}'
					exprTokens, err := Tokenize(exprSrc)
					if err != nil {
						return nil, err
					}
					parts = append(parts, InterpPart{Literal: false, Tokens: exprTokens})
				} else if tripleQuote && ch == '\n' {
					buf = append(buf, '\n')
					line++
					lineStart = pos
				} else if tripleQuote && ch == '\r' {
					// normalize \r\n to \n; lone \r kept as-is
					if pos < n && runes[pos] == '\n' {
						continue // skip \r, \n handled next iteration
					}
					buf = append(buf, '\r')
				} else {
					buf = append(buf, ch)
				}
			}
			if hasInterp {
				// Trailing literal
				if len(buf) > 0 {
					parts = append(parts, InterpPart{Literal: true, Str: string(buf)})
				}
				tokens = append(tokens, Token{Kind: TokInterp, Value: parts, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokString, Value: string(buf), Line: tokLine, Col: tokCol})
			}

		case unicode.IsLetter(c) || c == '_':
			start := pos
			for pos < n && (unicode.IsLetter(runes[pos]) || unicode.IsDigit(runes[pos]) || runes[pos] == '_') {
				pos++
			}
			s := string(runes[start:pos])
			if keywords[s] {
				tokens = append(tokens, Token{Kind: s, Line: tokLine, Col: tokCol})
			} else if s == "true" {
				tokens = append(tokens, Token{Kind: TokBool, Value: true, Line: tokLine, Col: tokCol})
			} else if s == "false" {
				tokens = append(tokens, Token{Kind: TokBool, Value: false, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokIdent, Value: s, Line: tokLine, Col: tokCol})
			}

		case c == '+':
			pos++
			if pos < n && runes[pos] == '+' {
				pos++
				tokens = append(tokens, Token{Kind: TokPlusPlus, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokPlus, Line: tokLine, Col: tokCol})
			}

		case c == '*':
			pos++
			tokens = append(tokens, Token{Kind: TokStar, Line: tokLine, Col: tokCol})

		case c == '%':
			pos++
			tokens = append(tokens, Token{Kind: TokPercent, Line: tokLine, Col: tokCol})

		case c == '/':
			pos++
			tokens = append(tokens, Token{Kind: TokSlash, Line: tokLine, Col: tokCol})

		case c == '\\':
			pos++
			tokens = append(tokens, Token{Kind: TokBackslash, Line: tokLine, Col: tokCol})

		case c == '!':
			pos++
			if pos < n && runes[pos] == '=' {
				pos++
				tokens = append(tokens, Token{Kind: TokBangEq, Line: tokLine, Col: tokCol})
			} else {
				return nil, lexErr("unexpected character: !")
			}

		case c == '=':
			pos++
			if pos < n && runes[pos] == '=' {
				pos++
				tokens = append(tokens, Token{Kind: TokEqEq, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokEq, Line: tokLine, Col: tokCol})
			}

		case c == '(':
			pos++
			tokens = append(tokens, Token{Kind: TokLParen, Line: tokLine, Col: tokCol})

		case c == ')':
			pos++
			tokens = append(tokens, Token{Kind: TokRParen, Line: tokLine, Col: tokCol})

		case c == '|':
			pos++
			if pos < n && runes[pos] == '>' {
				pos++
				tokens = append(tokens, Token{Kind: TokPipeGt, Line: tokLine, Col: tokCol})
			} else if pos < n && runes[pos] == '|' {
				pos++
				tokens = append(tokens, Token{Kind: TokPipePipe, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokPipe, Line: tokLine, Col: tokCol})
			}

		case c == '&':
			pos++
			if pos < n && runes[pos] == '&' {
				pos++
				tokens = append(tokens, Token{Kind: TokAmpAmp, Line: tokLine, Col: tokCol})
			} else {
				return nil, lexErr("unexpected character: &")
			}

		case c == '-':
			pos++
			if pos < n && runes[pos] == '>' {
				pos++
				tokens = append(tokens, Token{Kind: TokArrow, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokMinus, Line: tokLine, Col: tokCol})
			}

		case c == '<':
			pos++
			if pos < n && runes[pos] == '=' {
				pos++
				tokens = append(tokens, Token{Kind: TokLtEq, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokLt, Line: tokLine, Col: tokCol})
			}

		case c == '>':
			pos++
			if pos < n && runes[pos] == '=' {
				pos++
				tokens = append(tokens, Token{Kind: TokGtEq, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokGt, Line: tokLine, Col: tokCol})
			}

		case c == '[':
			pos++
			tokens = append(tokens, Token{Kind: TokLBrack, Line: tokLine, Col: tokCol})

		case c == ']':
			pos++
			tokens = append(tokens, Token{Kind: TokRBrack, Line: tokLine, Col: tokCol})

		case c == '{':
			pos++
			tokens = append(tokens, Token{Kind: TokLBrace, Line: tokLine, Col: tokCol})

		case c == '}':
			pos++
			tokens = append(tokens, Token{Kind: TokRBrace, Line: tokLine, Col: tokCol})

		case c == ',':
			pos++
			tokens = append(tokens, Token{Kind: TokComma, Line: tokLine, Col: tokCol})

		case c == ':':
			pos++
			if pos < n && runes[pos] == ':' {
				pos++
				tokens = append(tokens, Token{Kind: TokColonColon, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokColon, Line: tokLine, Col: tokCol})
			}

		case c == '.':
			pos++
			tokens = append(tokens, Token{Kind: TokDot, Line: tokLine, Col: tokCol})

		case c == ';':
			return nil, lexErr("unexpected character: ;")

		default:
			return nil, lexErr(fmt.Sprintf("unexpected character: %c", c))
		}
	}

	return tokens, nil
}

// skipInterp advances pos past a ${...} expression body (pos is right after the '{').
// On return, pos points just past the closing '}'.
func skipInterp(runes []rune, pos *int, n int) error {
	depth := 1
	for depth > 0 {
		if *pos >= n {
			return fmt.Errorf("unterminated interpolation")
		}
		ch := runes[*pos]
		*pos++
		switch ch {
		case '{':
			depth++
		case '}':
			depth--
		case '"':
			if err := skipString(runes, pos, n); err != nil {
				return err
			}
		}
	}
	return nil
}

// isHexDigit checks if r is a valid digit for the given prefix (x/o/b).
func isHexDigit(r rune, prefix rune) bool {
	switch prefix {
	case 'x', 'X':
		return (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
	case 'o', 'O':
		return r >= '0' && r <= '7'
	case 'b', 'B':
		return r == '0' || r == '1'
	}
	return false
}

// stripUnderscores removes all '_' characters from s.
func stripUnderscores(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] != '_' {
			out = append(out, s[i])
		}
	}
	return string(out)
}

// validateUnderscores checks for leading, trailing, or double underscores.
func validateUnderscores(s string) error {
	if len(s) == 0 {
		return nil
	}
	if s[0] == '_' {
		return fmt.Errorf("leading underscore in number literal")
	}
	if s[len(s)-1] == '_' {
		return fmt.Errorf("trailing underscore in number literal")
	}
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '_' && s[i+1] == '_' {
			return fmt.Errorf("double underscore in number literal")
		}
	}
	return nil
}

// skipString advances pos past a string literal (pos is right after the opening '"').
func skipString(runes []rune, pos *int, n int) error {
	// Check for triple-quote
	if *pos+1 < n && runes[*pos] == '"' && runes[*pos+1] == '"' {
		*pos += 2 // skip past the opening """
		for {
			if *pos >= n {
				return fmt.Errorf("unterminated multi-line string in interpolation")
			}
			ch := runes[*pos]
			*pos++
			switch ch {
			case '"':
				if *pos+1 < n && runes[*pos] == '"' && runes[*pos+1] == '"' {
					*pos += 2
					return nil
				}
			case '\\':
				if *pos < n {
					*pos++
				}
			case '$':
				if *pos < n && runes[*pos] == '{' {
					*pos++
					if err := skipInterp(runes, pos, n); err != nil {
						return err
					}
				}
			}
		}
	}
	// Regular single-line string
	for {
		if *pos >= n {
			return fmt.Errorf("unterminated string in interpolation")
		}
		ch := runes[*pos]
		*pos++
		switch ch {
		case '"':
			return nil
		case '\\':
			if *pos < n {
				*pos++ // skip escaped char
			}
		case '$':
			if *pos < n && runes[*pos] == '{' {
				*pos++ // skip '{'
				if err := skipInterp(runes, pos, n); err != nil {
					return err
				}
			}
		}
	}
}
