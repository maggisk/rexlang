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

// wasm value types
const (
	wtI32 = "i32"
	wtI64 = "i64"
	wtF64 = "f64"
)

// EmitWAT converts an IR program to WAT text.
func EmitWAT(prog *ir.Program) (string, error) {
	g := &watGen{
		buf:    &strings.Builder{},
		locals: make(map[string]string),
	}
	return g.emit(prog)
}

type watGen struct {
	buf    *strings.Builder
	indent int
	locals map[string]string // local name → wasm type
}

func (g *watGen) line(format string, args ...any) {
	g.buf.WriteString(strings.Repeat("  ", g.indent))
	fmt.Fprintf(g.buf, format, args...)
	g.buf.WriteByte('\n')
}

func (g *watGen) emit(prog *ir.Program) (string, error) {
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

	// _start function — calls $main, wraps i64 result to i32 for proc_exit
	g.line("(func (export \"_start\")")
	g.indent++
	g.line("(call $proc_exit (i32.wrap_i64 (call $main)))")
	g.indent--
	g.line(")")
	g.line("")

	// $main function — returns i64 (Rex Int)
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

	// Pre-pass: collect all locals and their types
	g.locals = make(map[string]string)
	g.collectLocals(body)

	// Emit function header with all locals declared up front
	g.line("(func $main (result i64)")
	g.indent++

	for _, name := range g.localOrder(body) {
		g.line("(local $%s %s)", name, g.locals[name])
	}

	if err := g.emitExpr(body); err != nil {
		return fmt.Errorf("codegen main: %w", err)
	}

	// main returns i64; if the body produced i32 (bool) or f64, convert
	switch g.typeOfExpr(body) {
	case wtI32:
		g.line("i64.extend_i32_u")
	case wtF64:
		g.line("i64.trunc_f64_s")
	}

	g.indent--
	g.line(")")
	return nil
}

// ---------------------------------------------------------------------------
// Local collection — pre-pass to find all let bindings and their types
// ---------------------------------------------------------------------------

func (g *watGen) collectLocals(expr ir.Expr) {
	switch e := expr.(type) {
	case ir.ELet:
		localType := g.typeOfCExpr(e.Bind)
		g.locals[e.Name] = localType
		g.collectLocals(e.Body)
	case ir.EComplex:
		g.collectLocalsCExpr(e.C)
	}
}

func (g *watGen) collectLocalsCExpr(c ir.CExpr) {
	switch e := c.(type) {
	case ir.CIf:
		g.collectLocals(e.Then)
		g.collectLocals(e.Else)
	}
}

// localOrder returns local names in the order they appear (depth-first).
func (g *watGen) localOrder(expr ir.Expr) []string {
	var names []string
	seen := make(map[string]bool)
	g.collectLocalOrder(expr, &names, seen)
	return names
}

func (g *watGen) collectLocalOrder(expr ir.Expr, names *[]string, seen map[string]bool) {
	switch e := expr.(type) {
	case ir.ELet:
		if !seen[e.Name] {
			seen[e.Name] = true
			*names = append(*names, e.Name)
		}
		g.collectLocalOrderCExpr(e.Bind, names, seen)
		g.collectLocalOrder(e.Body, names, seen)
	case ir.EComplex:
		g.collectLocalOrderCExpr(e.C, names, seen)
	}
}

func (g *watGen) collectLocalOrderCExpr(c ir.CExpr, names *[]string, seen map[string]bool) {
	switch e := c.(type) {
	case ir.CIf:
		g.collectLocalOrder(e.Then, names, seen)
		g.collectLocalOrder(e.Else, names, seen)
	}
}

// ---------------------------------------------------------------------------
// Type inference — determine wasm type of IR expressions
// ---------------------------------------------------------------------------

func (g *watGen) typeOfAtom(a ir.Atom) string {
	switch v := a.(type) {
	case ir.AInt:
		return wtI64
	case ir.AFloat:
		return wtF64
	case ir.ABool:
		return wtI32
	case ir.AVar:
		if t, ok := g.locals[v.Name]; ok {
			return t
		}
		return wtI64
	default:
		return wtI64
	}
}

func (g *watGen) typeOfCExpr(c ir.CExpr) string {
	switch e := c.(type) {
	case ir.CBinop:
		switch e.Op {
		case "Add", "Sub", "Mul", "Div", "Mod":
			return g.typeOfAtom(e.Left)
		case "Eq", "Neq", "Lt", "Gt", "Leq", "Geq", "And", "Or":
			return wtI32
		}
	case ir.CUnaryMinus:
		return g.typeOfAtom(e.Expr)
	case ir.CIf:
		return g.typeOfExpr(e.Then)
	}
	return wtI64
}

func (g *watGen) typeOfExpr(expr ir.Expr) string {
	switch e := expr.(type) {
	case ir.EAtom:
		return g.typeOfAtom(e.A)
	case ir.EComplex:
		return g.typeOfCExpr(e.C)
	case ir.ELet:
		return g.typeOfExpr(e.Body)
	}
	return wtI64
}

// ---------------------------------------------------------------------------
// Emission
// ---------------------------------------------------------------------------

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
		g.line("i64.const %d", v.Value)
		return nil
	case ir.AFloat:
		g.line("f64.const %g", v.Value)
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
		return g.emitBinop(e)
	case ir.CUnaryMinus:
		return g.emitUnaryMinus(e)
	case ir.CIf:
		return g.emitIf(e)
	default:
		return fmt.Errorf("unsupported cexpr: %T", c)
	}
}

func (g *watGen) emitBinop(e ir.CBinop) error {
	opType := g.typeOfAtom(e.Left)

	if err := g.emitAtom(e.Left); err != nil {
		return err
	}
	if err := g.emitAtom(e.Right); err != nil {
		return err
	}

	switch e.Op {
	case "Add":
		g.line("%s.add", opType)
	case "Sub":
		g.line("%s.sub", opType)
	case "Mul":
		g.line("%s.mul", opType)
	case "Div":
		if opType == wtF64 {
			g.line("f64.div")
		} else {
			g.line("i64.div_s")
		}
	case "Mod":
		g.line("i64.rem_s")
	case "Eq":
		g.line("%s.eq", opType)
	case "Neq":
		g.line("%s.ne", opType)
	case "Lt":
		if opType == wtF64 {
			g.line("f64.lt")
		} else {
			g.line("i64.lt_s")
		}
	case "Gt":
		if opType == wtF64 {
			g.line("f64.gt")
		} else {
			g.line("i64.gt_s")
		}
	case "Leq":
		if opType == wtF64 {
			g.line("f64.le")
		} else {
			g.line("i64.le_s")
		}
	case "Geq":
		if opType == wtF64 {
			g.line("f64.ge")
		} else {
			g.line("i64.ge_s")
		}
	case "And":
		g.line("i32.and")
	case "Or":
		g.line("i32.or")
	default:
		return fmt.Errorf("unsupported binop: %s", e.Op)
	}
	return nil
}

func (g *watGen) emitUnaryMinus(e ir.CUnaryMinus) error {
	opType := g.typeOfAtom(e.Expr)
	if opType == wtF64 {
		if err := g.emitAtom(e.Expr); err != nil {
			return err
		}
		g.line("f64.neg")
	} else {
		g.line("i64.const 0")
		if err := g.emitAtom(e.Expr); err != nil {
			return err
		}
		g.line("i64.sub")
	}
	return nil
}

func (g *watGen) emitIf(e ir.CIf) error {
	resultType := g.typeOfExpr(e.Then)
	if err := g.emitAtom(e.Cond); err != nil {
		return err
	}
	g.line("(if (result %s)", resultType)
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
}

func (g *watGen) emitLet(e ir.ELet) error {
	// Locals already declared at function top
	if err := g.emitCExpr(e.Bind); err != nil {
		return err
	}
	g.line("local.set $%s", e.Name)
	return g.emitExpr(e.Body)
}
