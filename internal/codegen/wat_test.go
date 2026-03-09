package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/typechecker"
)

// compileCode runs the full pipeline: parse → typecheck → lower → WAT.
func compileCode(t *testing.T, code string) string {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	typeEnv, _, err := typechecker.CheckProgram(exprs)
	if err != nil {
		t.Fatalf("typecheck error: %v", err)
	}
	l := ir.NewLowerer()
	prog, err := l.LowerProgram(exprs)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	wat, err := EmitWAT(prog, typeEnv)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	return wat
}

func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runWasm compiles Rex source to .wasm and runs it, returning the exit code.
func runWasm(t *testing.T, code string) int {
	t.Helper()
	if !hasCmd("wasm-tools") {
		t.Skip("wasm-tools not found")
	}
	if !hasCmd("wasmtime") {
		t.Skip("wasmtime not found")
	}

	wat := compileCode(t, code)

	dir := t.TempDir()
	watFile := filepath.Join(dir, "main.wat")
	wasmFile := filepath.Join(dir, "main.wasm")

	if err := os.WriteFile(watFile, []byte(wat), 0644); err != nil {
		t.Fatalf("write wat: %v", err)
	}

	out, err := exec.Command("wasm-tools", "parse", watFile, "-o", wasmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("wasm-tools parse: %v\n%s\nWAT:\n%s", err, out, wat)
	}

	cmd := exec.Command("wasmtime", "--wasm", "gc", "--wasm", "function-references", wasmFile)
	out, err = cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("wasmtime: %v\n%s", err, out)
		}
	}
	return exitCode
}

// ---------------------------------------------------------------------------
// WAT output tests (no wasm-tools needed)
// ---------------------------------------------------------------------------

func TestWATMainZero(t *testing.T) {
	wat := compileCode(t, "main _ = 0\n")
	if !strings.Contains(wat, "i64.const 0") {
		t.Error("expected i64.const 0")
	}
	if !strings.Contains(wat, "i32.wrap_i64") {
		t.Error("expected i32.wrap_i64 for proc_exit")
	}
}

func TestWATArithmetic(t *testing.T) {
	wat := compileCode(t, "main _ = 1 + 2\n")
	if !strings.Contains(wat, "i64.add") {
		t.Error("expected i64.add")
	}
}

func TestWATFloat(t *testing.T) {
	wat := compileCode(t, "main _ = 1.5 + 2.5\n")
	if !strings.Contains(wat, "f64.const") {
		t.Error("expected f64.const")
	}
	if !strings.Contains(wat, "f64.add") {
		t.Error("expected f64.add")
	}
}

func TestWATFunctionCall(t *testing.T) {
	wat := compileCode(t, "double x = x + x\nmain _ = double 21\n")
	if !strings.Contains(wat, "func $double") {
		t.Error("expected func $double")
	}
	if !strings.Contains(wat, "call $double") {
		t.Error("expected call $double")
	}
}

func TestWATNoMain(t *testing.T) {
	exprs, _ := parser.Parse("x = 42\n")
	l := ir.NewLowerer()
	prog, _ := l.LowerProgram(exprs)
	_, err := EmitWAT(prog, nil)
	if err == nil || !strings.Contains(err.Error(), "no main") {
		t.Fatalf("expected 'no main' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// End-to-end tests
// ---------------------------------------------------------------------------

func TestE2EZero(t *testing.T) {
	if got := runWasm(t, "main _ = 0\n"); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}

func TestE2ELiteral(t *testing.T) {
	if got := runWasm(t, "main _ = 42\n"); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EArithmetic(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ = 10 + 32\n", 42},
		{"main _ = 100 - 58\n", 42},
		{"main _ = 6 * 7\n", 42},
		{"main _ = 84 / 2\n", 42},
		{"main _ = 85 % 43\n", 42},
		{"main _ = 1 + 2 + 3\n", 6},
		{"main _ = 10 - 3 * 2\n", 4},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EUnaryMinus(t *testing.T) {
	if got := runWasm(t, "main _ = -(-42)\n"); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EIfThenElse(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ =\n    if true then\n        42\n    else\n        0\n", 42},
		{"main _ =\n    if false then\n        0\n    else\n        42\n", 42},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EComparison(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ =\n    if 1 < 2 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 2 > 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 <= 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 >= 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 42 == 42 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 != 2 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 2 < 1 then\n        1\n    else\n        0\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EBooleanOps(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ =\n    if true && true then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if true && false then\n        1\n    else\n        0\n", 0},
		{"main _ =\n    if false || true then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if false || false then\n        1\n    else\n        0\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EFloatArithmetic(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ = 1.5 + 2.5\n", 4},
		{"main _ = 10.0 - 3.0\n", 7},
		{"main _ = 6.0 * 7.0\n", 42},
		{"main _ = 84.0 / 2.0\n", 42},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2ENestedIf(t *testing.T) {
	code := `
main _ =
    if 1 < 2 then
        if 3 > 4 then
            10
        else
            42
    else
        0
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ELetWithComparison(t *testing.T) {
	code := `
main _ =
    let x = 6 * 7
    in
    if x == 42 then
        1
    else
        0
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Function call tests
// ---------------------------------------------------------------------------

func TestE2EFunctionCall(t *testing.T) {
	code := `
double x = x + x
main _ = double 21
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EMultiArgFunction(t *testing.T) {
	code := `
add x y = x + y
main _ = add 20 22
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EThreeArgFunction(t *testing.T) {
	code := `
add3 a b c = a + b + c
main _ = add3 10 20 12
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ERecursion(t *testing.T) {
	code := `
fact n =
    if n == 0 then
        1
    else
        n * fact (n - 1)
main _ = fact 5
`
	// 5! = 120
	if got := runWasm(t, code); got != 120 {
		t.Fatalf("expected exit 120, got %d", got)
	}
}

func TestE2EMultipleFunctions(t *testing.T) {
	code := `
double x = x + x
square x = x * x
main _ = double (square 3) + square 2 + double 1
`
	// double(9) + 4 + 2 = 18 + 4 + 2 = 24
	if got := runWasm(t, code); got != 24 {
		t.Fatalf("expected exit 24, got %d", got)
	}
}

func TestE2EFunctionWithIf(t *testing.T) {
	code := `
abs x =
    if x < 0 then
        0 - x
    else
        x
main _ = abs (-42)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EClosureCapture(t *testing.T) {
	code := `
makeAdder n = \x -> n + x
main _ =
    let add5 = makeAdder 5
    in add5 37
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EClosureTwoCaptures(t *testing.T) {
	code := `
makeAdd a b = \x -> a + b + x
main _ =
    let f = makeAdd 10 20
    in f 12
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EHigherOrderFunction(t *testing.T) {
	code := `
apply f x = f x
double x = x + x
main _ = apply double 21
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EMutualFunctions(t *testing.T) {
	code := `
isEven n =
    if n == 0 then
        1
    else
        isOdd (n - 1)
isOdd n =
    if n == 0 then
        0
    else
        isEven (n - 1)
main _ = isEven 10
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}
