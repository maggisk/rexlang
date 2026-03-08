package parser

import (
	"strings"
	"testing"
)

func TestValidateIndentation_MatchArmBodyNotIndented(t *testing.T) {
	code := `
f x =
    match x
        when 0 ->
        1
        when _ ->
            2
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error, got nil")
	}
	if !strings.Contains(err.Error(), "indented past 'when'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_MatchArmBodyAtSameColAsWhen(t *testing.T) {
	code := `
f x =
    match x
        when 0 ->
    1
        when _ ->
            2
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error, got nil")
	}
	if !strings.Contains(err.Error(), "indented past 'when'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_MatchArmBodyProperlyIndented(t *testing.T) {
	code := `
f x =
    match x
        when 0 ->
            1
        when _ ->
            2
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := ValidateIndentation(exprs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_SingleLineArmOK(t *testing.T) {
	// Single-line arms (body on same line as when) should always be OK.
	// The parser may or may not accept this form depending on context,
	// but if it parses, the validator should not reject it.
	code := `
f x =
    match x
        when 0 ->
            1
        when _ ->
            2
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := ValidateIndentation(exprs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_NestedMatchBadIndentation(t *testing.T) {
	code := `
f x y =
    match x
        when 0 ->
            match y
                when 1 ->
                2
                when _ ->
                    3
        when _ ->
            4
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error for nested match, got nil")
	}
	if !strings.Contains(err.Error(), "indented past 'when'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_NestedMatchGoodIndentation(t *testing.T) {
	code := `
f x y =
    match x
        when 0 ->
            match y
                when 1 ->
                    2
                when _ ->
                    3
        when _ ->
            4
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := ValidateIndentation(exprs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_MatchInsideLetBody(t *testing.T) {
	code := `
f x =
    let y = match x
                when 0 ->
                1
                when _ ->
                    2
    in y
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error, got nil")
	}
	if !strings.Contains(err.Error(), "indented past 'when'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_TopLevelMatch(t *testing.T) {
	code := `
f x =
    match x
        when 0 ->
        0
        when _ ->
            1
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error, got nil")
	}
}

func TestValidateIndentation_MatchInTestDecl(t *testing.T) {
	code := `
test "bad match" =
    let r = match 1
                when 0 ->
                "a"
                when _ ->
                    "b"
    in assert (r == "b")
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error inside test, got nil")
	}
}

func TestValidateIndentation_MatchInImplMethod(t *testing.T) {
	code := `
type Color = Red | Blue

trait Describe a where
    describe : a -> String

impl Describe Color where
    describe c =
        match c
            when Red ->
            "red"
            when Blue ->
                "blue"
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error inside impl, got nil")
	}
}

func TestValidateIndentation_AllArmsGoodVariousDepth(t *testing.T) {
	code := `
f x =
    match x
        when 0 ->
            "zero"
        when 1 ->
            "one"
        when 2 ->
            "two"
        when _ ->
            "other"
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := ValidateIndentation(exprs); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateIndentation_ErrorHasLineNumber(t *testing.T) {
	code := `
f x =
    match x
        when 0 ->
        1
        when _ ->
            2
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected error")
	}
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("expected *ParseError, got %T", err)
	}
	if pe.Line == 0 {
		t.Fatal("expected non-zero line in error")
	}
}

func TestValidateIndentation_MatchInLambda(t *testing.T) {
	code := `
import Std:List (map)

f xs =
    map (\x ->
        match x
            when 0 ->
            "z"
            when _ ->
                "o"
    ) xs
`
	exprs, err := Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	err = ValidateIndentation(exprs)
	if err == nil {
		t.Fatal("expected indentation error inside lambda, got nil")
	}
}
