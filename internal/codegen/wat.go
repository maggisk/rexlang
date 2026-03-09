// Package codegen emits WebAssembly Text (WAT) from the IR.
//
// The initial target is WasmGC via WASI — programs export _start,
// and the exit code comes from main's return value via proc_exit.
package codegen

import (
	"fmt"
	"strings"

	"github.com/maggisk/rexlang/internal/ir"
)

// EmitWAT converts an IR program to WAT text.
// For now, only handles programs whose main returns an Int literal.
func EmitWAT(prog *ir.Program) (string, error) {
	g := &watGen{buf: &strings.Builder{}}
	return g.emit(prog)
}

type watGen struct {
	buf    *strings.Builder
	indent int
}

func (g *watGen) line(format string, args ...any) {
	g.buf.WriteString(strings.Repeat("  ", g.indent))
	fmt.Fprintf(g.buf, format, args...)
	g.buf.WriteByte('\n')
}

func (g *watGen) emit(prog *ir.Program) (string, error) {
	// Find main declaration
	var mainDecl *ir.DLet
	for _, d := range prog.Decls {
		if dl, ok := d.(ir.DLet); ok && dl.Name == "main" {
			mainDecl = &dl
			break
		}
	}
	if mainDecl == nil {
		return "", fmt.Errorf("codegen: no main function found")
	}

	g.line("(module")
	g.indent++

	// WASI import: proc_exit
	g.line("(import \"wasi_snapshot_preview1\" \"proc_exit\" (func $proc_exit (param i32)))")
	g.line("")

	// Memory (required by WASI)
	g.line("(memory (export \"memory\") 1)")
	g.line("")

	// _start function — calls $main, then proc_exit with the result
	g.line("(func (export \"_start\")")
	g.indent++
	g.line("(call $proc_exit (call $main))")
	g.indent--
	g.line(")")
	g.line("")

	// $main function — returns i32
	if err := g.emitMain(mainDecl); err != nil {
		return "", err
	}

	g.indent--
	g.line(")")

	return g.buf.String(), nil
}

func (g *watGen) emitMain(decl *ir.DLet) error {
	// main is `\_ -> body` — unwrap the lambda
	body := decl.Body
	if ec, ok := body.(ir.EComplex); ok {
		if lam, ok := ec.C.(ir.CLambda); ok {
			body = lam.Body
		}
	}

	g.line("(func $main (result i32)")
	g.indent++

	if err := g.emitExpr(body); err != nil {
		return fmt.Errorf("codegen main: %w", err)
	}

	g.indent--
	g.line(")")
	return nil
}

func (g *watGen) emitExpr(expr ir.Expr) error {
	switch e := expr.(type) {
	case ir.EAtom:
		return g.emitAtom(e.A)
	case ir.EComplex:
		return g.emitCExpr(e.C)
	case ir.ELet:
		return g.emitLet(e)
	default:
		return fmt.Errorf("unsupported expr: %T", expr)
	}
}

func (g *watGen) emitAtom(a ir.Atom) error {
	switch v := a.(type) {
	case ir.AInt:
		g.line("i32.const %d", v.Value)
		return nil
	case ir.AVar:
		g.line("local.get $%s", v.Name)
		return nil
	case ir.ABool:
		if v.Value {
			g.line("i32.const 1")
		} else {
			g.line("i32.const 0")
		}
		return nil
	default:
		return fmt.Errorf("unsupported atom: %T", a)
	}
}

func (g *watGen) emitCExpr(c ir.CExpr) error {
	switch e := c.(type) {
	case ir.CBinop:
		if err := g.emitAtom(e.Left); err != nil {
			return err
		}
		if err := g.emitAtom(e.Right); err != nil {
			return err
		}
		switch e.Op {
		case "Add":
			g.line("i32.add")
		case "Sub":
			g.line("i32.sub")
		case "Mul":
			g.line("i32.mul")
		case "Div":
			g.line("i32.div_s")
		case "Mod":
			g.line("i32.rem_s")
		default:
			return fmt.Errorf("unsupported binop: %s", e.Op)
		}
		return nil
	case ir.CUnaryMinus:
		g.line("i32.const 0")
		if err := g.emitAtom(e.Expr); err != nil {
			return err
		}
		g.line("i32.sub")
		return nil
	case ir.CIf:
		// (if (result i32) <cond> (then <then>) (else <else>))
		if err := g.emitAtom(e.Cond); err != nil {
			return err
		}
		g.line("(if (result i32)")
		g.indent++
		g.line("(then")
		g.indent++
		if err := g.emitExpr(e.Then); err != nil {
			return err
		}
		g.indent--
		g.line(")")
		g.line("(else")
		g.indent++
		if err := g.emitExpr(e.Else); err != nil {
			return err
		}
		g.indent--
		g.line(")")
		g.indent--
		g.line(")")
		return nil
	default:
		return fmt.Errorf("unsupported cexpr: %T", c)
	}
}

func (g *watGen) emitLet(e ir.ELet) error {
	// Declare the local, compute the value, set it, then emit body
	g.line("(local $%s i32)", e.Name)
	if err := g.emitCExpr(e.Bind); err != nil {
		return err
	}
	g.line("local.set $%s", e.Name)
	return g.emitExpr(e.Body)
}
