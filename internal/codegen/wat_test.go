package codegen

import (
	"fmt"
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

func TestEmitWATMainZero(t *testing.T) {
	prog := lowerCode(t, "main _ = 0\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	t.Logf("WAT output:\n%s", wat)

	if !strings.Contains(wat, "proc_exit") {
		t.Error("expected proc_exit import")
	}
	if !strings.Contains(wat, "_start") {
		t.Error("expected _start export")
	}
	if !strings.Contains(wat, "i32.const 0") {
		t.Error("expected i32.const 0")
	}
}

func TestEmitWATMainArithmetic(t *testing.T) {
	prog := lowerCode(t, "main _ = 1 + 2\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	t.Logf("WAT output:\n%s", wat)

	if !strings.Contains(wat, "i32.add") {
		t.Error("expected i32.add")
	}
}

func TestEmitWATMainNestedArith(t *testing.T) {
	prog := lowerCode(t, "main _ = 10 - 3 * 2\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	t.Logf("WAT output:\n%s", wat)

	if !strings.Contains(wat, "i32.mul") {
		t.Error("expected i32.mul")
	}
	if !strings.Contains(wat, "i32.sub") {
		t.Error("expected i32.sub")
	}
}

func TestEmitWATMainIf(t *testing.T) {
	prog := lowerCode(t, `
main _ =
    if true then
        42
    else
        0
`)
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	t.Logf("WAT output:\n%s", wat)

	if !strings.Contains(wat, "if (result i32)") {
		t.Error("expected if expression")
	}
}

// hasCmd checks if a command is available on PATH.
func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// TestEndToEnd compiles main _ = 0 all the way to .wasm and runs it.
func TestEndToEnd(t *testing.T) {
	if !hasCmd("wasm-tools") {
		t.Skip("wasm-tools not found")
	}
	if !hasCmd("wasmtime") {
		t.Skip("wasmtime not found")
	}

	prog := lowerCode(t, "main _ = 0\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT: %v", err)
	}

	dir := t.TempDir()
	watFile := filepath.Join(dir, "main.wat")
	wasmFile := filepath.Join(dir, "main.wasm")

	if err := os.WriteFile(watFile, []byte(wat), 0644); err != nil {
		t.Fatalf("write wat: %v", err)
	}

	// WAT → .wasm
	out, err := exec.Command("wasm-tools", "parse", watFile, "-o", wasmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("wasm-tools parse failed: %v\n%s\nWAT:\n%s", err, out, wat)
	}

	// Run with wasmtime
	cmd := exec.Command("wasmtime", wasmFile)
	out, err = cmd.CombinedOutput()
	// Exit code 0 → success (err == nil)
	if err != nil {
		t.Fatalf("wasmtime failed: %v\n%s", err, out)
	}
	t.Logf("wasmtime ran successfully (exit 0)")
}

// TestEndToEndExitCode tests that main _ = 42 produces exit code 42.
func TestEndToEndExitCode(t *testing.T) {
	if !hasCmd("wasm-tools") {
		t.Skip("wasm-tools not found")
	}
	if !hasCmd("wasmtime") {
		t.Skip("wasmtime not found")
	}

	prog := lowerCode(t, "main _ = 42\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT: %v", err)
	}

	dir := t.TempDir()
	watFile := filepath.Join(dir, "main.wat")
	wasmFile := filepath.Join(dir, "main.wasm")

	if err := os.WriteFile(watFile, []byte(wat), 0644); err != nil {
		t.Fatalf("write wat: %v", err)
	}

	out, err := exec.Command("wasm-tools", "parse", watFile, "-o", wasmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("wasm-tools parse failed: %v\n%s\nWAT:\n%s", err, out, wat)
	}

	cmd := exec.Command("wasmtime", wasmFile)
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code")
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		if exitErr.ExitCode() != 42 {
			t.Fatalf("expected exit code 42, got %d", exitErr.ExitCode())
		}
		t.Logf("wasmtime exit code: 42 (correct)")
	} else {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestEndToEndArithmetic tests main _ = 10 + 32 → exit code 42.
func TestEndToEndArithmetic(t *testing.T) {
	if !hasCmd("wasm-tools") {
		t.Skip("wasm-tools not found")
	}
	if !hasCmd("wasmtime") {
		t.Skip("wasmtime not found")
	}

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
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			prog := lowerCode(t, tt.code)
			wat, err := EmitWAT(prog)
			if err != nil {
				t.Fatalf("EmitWAT: %v", err)
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

			if exitCode != tt.exitCode {
				t.Fatalf("expected exit code %d, got %d\nWAT:\n%s", tt.exitCode, exitCode, wat)
			}
		})
	}
}

func TestEmitWATNoMain(t *testing.T) {
	prog := lowerCode(t, "x = 42\n")
	_, err := EmitWAT(prog)
	if err == nil {
		t.Fatal("expected error for missing main")
	}
	if !strings.Contains(err.Error(), "no main") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWATDump(t *testing.T) {
	prog := lowerCode(t, "main _ = 1 + 2 * 3\n")
	wat, err := EmitWAT(prog)
	if err != nil {
		t.Fatalf("EmitWAT: %v", err)
	}
	fmt.Println("=== WAT dump ===")
	fmt.Println(wat)
	fmt.Println("================")
}
