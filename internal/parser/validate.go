package parser

import (
	"fmt"

	"github.com/maggisk/rexlang/internal/ast"
)

// ValidateIndentation walks the AST and checks that bodies are indented
// relative to their headers (match arms, if/then/else, let bindings).
// Returns the first error found, or nil.
func ValidateIndentation(exprs []ast.Expr) error {
	for _, e := range exprs {
		if err := validateExpr(e); err != nil {
			return err
		}
	}
	return nil
}

func validateExpr(e ast.Expr) error {
	if e == nil {
		return nil
	}
	switch x := e.(type) {
	case ast.Match:
		for _, arm := range x.Arms {
			if err := checkArmBody(arm); err != nil {
				return err
			}
			if err := validateExpr(arm.Body); err != nil {
				return err
			}
		}
		return validateExpr(x.Scrutinee)

	case ast.If:
		if err := validateExpr(x.Cond); err != nil {
			return err
		}
		if err := validateExpr(x.ThenExpr); err != nil {
			return err
		}
		return validateExpr(x.ElseExpr)

	case ast.Let:
		if err := validateExpr(x.Body); err != nil {
			return err
		}
		return validateExpr(x.InExpr)

	case ast.LetPat:
		if err := validateExpr(x.Body); err != nil {
			return err
		}
		return validateExpr(x.InExpr)

	case ast.LetRec:
		for _, b := range x.Bindings {
			if err := validateExpr(b.Body); err != nil {
				return err
			}
		}
		return validateExpr(x.InExpr)

	case ast.Fun:
		return validateExpr(x.Body)

	case ast.App:
		if err := validateExpr(x.Func); err != nil {
			return err
		}
		return validateExpr(x.Arg)

	case ast.Binop:
		if err := validateExpr(x.Left); err != nil {
			return err
		}
		return validateExpr(x.Right)

	case ast.UnaryMinus:
		return validateExpr(x.Expr)

	case ast.ListLit:
		for _, item := range x.Items {
			if err := validateExpr(item); err != nil {
				return err
			}
		}

	case ast.TupleLit:
		for _, item := range x.Items {
			if err := validateExpr(item); err != nil {
				return err
			}
		}

	case ast.StringInterp:
		for _, part := range x.Parts {
			if err := validateExpr(part); err != nil {
				return err
			}
		}

	case ast.RecordCreate:
		for _, f := range x.Fields {
			if err := validateExpr(f.Value); err != nil {
				return err
			}
		}

	case ast.RecordUpdate:
		if err := validateExpr(x.Record); err != nil {
			return err
		}
		for _, u := range x.Updates {
			if err := validateExpr(u.Value); err != nil {
				return err
			}
		}

	case ast.FieldAccess:
		return validateExpr(x.Record)

	case ast.Assert:
		return validateExpr(x.Expr)

	case ast.TestDecl:
		for _, body := range x.Body {
			if err := validateExpr(body); err != nil {
				return err
			}
		}

	case ast.ImplDecl:
		for _, m := range x.Methods {
			if err := validateExpr(m.Body); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkArmBody validates that a match arm's body is indented past the 'when' keyword.
// Single-line arms (body on same line as 'when') are always OK.
func checkArmBody(arm ast.MatchArm) error {
	if arm.BodyLine <= 0 || arm.Line <= 0 {
		return nil
	}
	// Same line as 'when' — always fine (e.g. `when 0 -> 1`)
	if arm.BodyLine == arm.Line {
		return nil
	}
	// Body on subsequent line must be indented past the 'when' column
	if arm.BodyCol <= arm.Col {
		return &ParseError{
			Msg:  fmt.Sprintf("match arm body must be indented past 'when' (line %d, col %d), but body is at col %d", arm.Line, arm.Col+1, arm.BodyCol+1),
			Line: arm.BodyLine,
			Col:  arm.BodyCol,
		}
	}
	return nil
}
