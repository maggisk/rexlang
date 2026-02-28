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
	"if": true, "then": true, "else": true, "fun": true,
	"case": true, "type": true, "of": true, "import": true,
	"export": true, "as": true, "trait": true, "impl": true,
	"where": true, "test": true, "assert": true,
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

	skipBlockComment := func() error {
		pos += 2 // skip '(' and '*'
		depth := 1
		for depth > 0 {
			if pos >= n {
				return lexErr("unterminated comment")
			}
			c := runes[pos]
			pos++
			if c == '\n' {
				line++
				lineStart = pos
			} else if c == '(' && pos < n && runes[pos] == '*' {
				pos++
				depth++
			} else if c == '*' && pos < n && runes[pos] == ')' {
				pos++
				depth--
			}
		}
		return nil
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
			} else if pos+1 < n && runes[pos] == '(' && runes[pos+1] == '*' {
				if err := skipBlockComment(); err != nil {
					return err
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
			for pos < n && unicode.IsDigit(runes[pos]) {
				pos++
			}
			if pos < n && runes[pos] == '.' {
				pos++
				for pos < n && unicode.IsDigit(runes[pos]) {
					pos++
				}
				f, _ := strconv.ParseFloat(string(runes[start:pos]), 64)
				tokens = append(tokens, Token{Kind: TokFloat, Value: f, Line: tokLine, Col: tokCol})
			} else {
				i, _ := strconv.Atoi(string(runes[start:pos]))
				tokens = append(tokens, Token{Kind: TokInt, Value: i, Line: tokLine, Col: tokCol})
			}

		case c == '"':
			pos++ // skip opening '"'
			var buf []rune
			for {
				if pos >= n {
					return nil, lexErr("unterminated string")
				}
				ch := runes[pos]
				pos++
				if ch == '"' {
					break
				} else if ch == '\\' {
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
					default:
						return nil, lexErr(fmt.Sprintf("unknown escape: \\%c", esc))
					}
				} else {
					buf = append(buf, ch)
				}
			}
			tokens = append(tokens, Token{Kind: TokString, Value: string(buf), Line: tokLine, Col: tokCol})

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
			if pos < n && runes[pos] == '=' {
				pos++
				tokens = append(tokens, Token{Kind: TokSlashEq, Line: tokLine, Col: tokCol})
			} else {
				tokens = append(tokens, Token{Kind: TokSlash, Line: tokLine, Col: tokCol})
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
