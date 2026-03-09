package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/parser"
)

func lowerCode(t *testing.T, code string) *ir.Program {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	l := ir.NewLowerer()
	prog, err := l.LowerProgram(exprs)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	return prog
}

// hasCmd checks if a command is available on PATH.
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

	prog := lowerCode(t, code)
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT: %v\nCode: %s", err, code)
	}

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

	cmd := exec.Command("wasmtime", wasmFile)
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

func TestEmitWATMainZero(t *testing.T) {
	prog := lowerCode(t, "main _ = 0\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	if !strings.Contains(wat, "i64.const 0") {
		t.Error("expected i64.const 0")
	}
	if !strings.Contains(wat, "i32.wrap_i64") {
		t.Error("expected i32.wrap_i64 for proc_exit")
	}
}

func TestEmitWATArithmetic(t *testing.T) {
	prog := lowerCode(t, "main _ = 1 + 2\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	if !strings.Contains(wat, "i64.add") {
		t.Error("expected i64.add")
	}
}

func TestEmitWATFloat(t *testing.T) {
	prog := lowerCode(t, "main _ = 1.5 + 2.5\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	if !strings.Contains(wat, "f64.const") {
		t.Error("expected f64.const")
	}
	if !strings.Contains(wat, "f64.add") {
		t.Error("expected f64.add")
	}
	// main returns i64, so float result should be truncated
	if !strings.Contains(wat, "i64.trunc_f64_s") {
		t.Error("expected i64.trunc_f64_s conversion")
	}
}

func TestEmitWATComparison(t *testing.T) {
	prog := lowerCode(t, `
main _ =
    if 1 < 2 then
        42
    else
        0
`)
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	if !strings.Contains(wat, "i64.lt_s") {
		t.Error("expected i64.lt_s")
	}
}

func TestEmitWATBoolOps(t *testing.T) {
	prog := lowerCode(t, `
main _ =
    if true && false then
        1
    else
        0
`)
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	if !strings.Contains(wat, "i32.and") {
		t.Error("expected i32.and")
	}
}

func TestEmitWATNoMain(t *testing.T) {
	prog := lowerCode(t, "x = 42\n")
	_, err := EmitWAT(prog)
	if err == nil || !strings.Contains(err.Error(), "no main") {
		t.Fatalf("expected 'no main' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// End-to-end tests (require wasm-tools + wasmtime)
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
	// -(-42) = 42
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
		// true branch → 1, false branch → 0
		{"main _ =\n    if 1 < 2 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 2 > 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 <= 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 >= 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 42 == 42 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 != 2 then\n        1\n    else\n        0\n", 1},
		// false cases
		{"main _ =\n    if 2 < 1 then\n        1\n    else\n        0\n", 0},
		{"main _ =\n    if 1 == 2 then\n        1\n    else\n        0\n", 0},
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
		// Float results get truncated to i64 for exit code
		{"main _ = 1.5 + 2.5\n", 4},    // 4.0 → 4
		{"main _ = 10.0 - 3.0\n", 7},   // 7.0 → 7
		{"main _ = 6.0 * 7.0\n", 42},   // 42.0 → 42
		{"main _ = 84.0 / 2.0\n", 42},  // 42.0 → 42
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EFloatComparison(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ =\n    if 1.5 < 2.5 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 2.5 > 1.5 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1.0 == 1.0 then\n        1\n    else\n        0\n", 1},
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
