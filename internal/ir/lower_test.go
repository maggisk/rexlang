package ir

import (
	"fmt"
	"strings"
	"testing"

	"github.com/maggisk/rexlang/internal/parser"
)

func lowerCode(t *testing.T, code string) *Program {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	l := NewLowerer()
	prog, err := l.LowerProgram(exprs)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return prog
}

func TestLowerLiteral(t *testing.T) {
	prog := lowerCode(t, "x = 42\n")
	if len(prog.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(prog.Decls))
	}
	s := DeclToString(prog.Decls[0])
	if !strings.Contains(s, "42") {
		t.Fatalf("expected 42, got: %s", s)
	}
}

func TestLowerBinop(t *testing.T) {
	prog := lowerCode(t, "x = 1 + 2\n")
	s := ProgramToString(prog)
	// ANF: x = 1 Add 2
	if !strings.Contains(s, "1 Add 2") {
		t.Fatalf("expected binop in ANF, got:\n%s", s)
	}
}

func TestLowerNestedBinop(t *testing.T) {
	prog := lowerCode(t, "x = 1 + 2 + 3\n")
	s := ProgramToString(prog)
	// Should have a let binding for the intermediate result
	if !strings.Contains(s, "let _t") {
		t.Fatalf("expected let binding for intermediate, got:\n%s", s)
	}
}

func TestLowerFunctionDef(t *testing.T) {
	prog := lowerCode(t, "f x = x + 1\n")
	s := ProgramToString(prog)
	// Should contain a lambda
	if !strings.Contains(s, "\\x ->") || !strings.Contains(s, "\\") {
		t.Fatalf("expected lambda, got:\n%s", s)
	}
}

func TestLowerApp(t *testing.T) {
	prog := lowerCode(t, "f x = x\ny = f 42\n")
	s := ProgramToString(prog)
	if !strings.Contains(s, "f 42") {
		t.Fatalf("expected application f 42, got:\n%s", s)
	}
}

func TestLowerIf(t *testing.T) {
	prog := lowerCode(t, `
f x =
    if x then
        1
    else
        2
`)
	s := ProgramToString(prog)
	if !strings.Contains(s, "if") && !strings.Contains(s, "then") {
		t.Fatalf("expected if/then, got:\n%s", s)
	}
}

func TestLowerMatch(t *testing.T) {
	prog := lowerCode(t, `
f x =
    match x
        when 0 ->
            1
        when _ ->
            2
`)
	s := ProgramToString(prog)
	if !strings.Contains(s, "match") {
		t.Fatalf("expected match, got:\n%s", s)
	}
	if !strings.Contains(s, "when") {
		t.Fatalf("expected when arms, got:\n%s", s)
	}
}

func TestLowerList(t *testing.T) {
	prog := lowerCode(t, "xs = [1, 2, 3]\n")
	s := ProgramToString(prog)
	if !strings.Contains(s, "[1, 2, 3]") {
		t.Fatalf("expected [1, 2, 3], got:\n%s", s)
	}
}

func TestLowerTuple(t *testing.T) {
	prog := lowerCode(t, "p = (1, 2)\n")
	s := ProgramToString(prog)
	if !strings.Contains(s, "(1, 2)") {
		t.Fatalf("expected (1, 2), got:\n%s", s)
	}
}

func TestLowerLambda(t *testing.T) {
	prog := lowerCode(t, `f = \x -> x + 1`+"\n")
	s := ProgramToString(prog)
	if !strings.Contains(s, "\\x ->") {
		t.Fatalf("expected lambda, got:\n%s", s)
	}
}

func TestLowerLetIn(t *testing.T) {
	prog := lowerCode(t, `
f x =
    let y = x + 1
    in y + 2
`)
	s := ProgramToString(prog)
	if !strings.Contains(s, "let y") {
		t.Fatalf("expected let y binding, got:\n%s", s)
	}
}

func TestLowerNestedApp(t *testing.T) {
	// f (g x) should become: let t = g x in f t
	prog := lowerCode(t, `
f x = x
g x = x
y = f (g 1)
`)
	s := ProgramToString(prog)
	// Should have a temporary for the inner application
	if !strings.Contains(s, "let _t") {
		t.Fatalf("expected temp binding for nested app, got:\n%s", s)
	}
}

func TestLowerTypeDecl(t *testing.T) {
	prog := lowerCode(t, "type Color = Red | Blue\n")
	s := ProgramToString(prog)
	if !strings.Contains(s, "type Color") {
		t.Fatalf("expected type decl, got:\n%s", s)
	}
}

func TestLowerImport(t *testing.T) {
	prog := lowerCode(t, `import Std:List (map, filter)`+"\n")
	s := ProgramToString(prog)
	if !strings.Contains(s, "import Std:List") {
		t.Fatalf("expected import, got:\n%s", s)
	}
}

func TestLowerDump(t *testing.T) {
	// Visual check — not a real assertion, just prints the IR
	prog := lowerCode(t, `
type Shape = Circle Float | Rect Float Float

area s =
    match s
        when Circle r ->
            3.14 * r * r
        when Rect w h ->
            w * h

main _ = 0
`)
	s := ProgramToString(prog)
	fmt.Println("=== IR dump ===")
	fmt.Println(s)
	fmt.Println("===============")
	// Just verify it doesn't crash and produces output
	if len(s) == 0 {
		t.Fatal("expected non-empty IR dump")
	}
}
