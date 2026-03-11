// Package codegen — Go backend.
//
// EmitGo converts an IR program into a Go source file. The output is a
// standalone main package that can be compiled with `go build`.
package codegen

import (
	"fmt"
	"strings"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// EmitGo converts an IR program to Go source code.
func EmitGo(prog *ir.Program, typeEnv typechecker.TypeEnv) (string, error) {
	g := &goGen{
		buf:          &strings.Builder{},
		typeEnv:      typeEnv,
		funcs:        make(map[string]*goFuncInfo),
		adts:         make(map[string]*goAdtInfo),
		ctorToAdt:    make(map[string]*goCtorInfo),
		records:      make(map[string]*goRecordInfo),
		traitImpls:   make(map[string][]goImplCase), // "Trait:Method" -> cases
		usedBuiltins: make(map[string]bool),
		locals:       make(map[string]bool),
	}
	return g.emit(prog)
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type goFuncInfo struct {
	name    string
	arity   int
	params  []goParamInfo
	retType types.Type
	body    ir.Expr
}

type goParamInfo struct {
	name string
	ty   types.Type
}

type goAdtInfo struct {
	name  string
	ctors []goCtorInfo
}

type goCtorInfo struct {
	name       string
	tag        int
	typeName   string
	fieldTypes []types.Type
}

type goRecordInfo struct {
	name       string
	fieldNames []string
	fieldTypes []types.Type
}

type goImplCase struct {
	typeName string
	funcName string
}

// ---------------------------------------------------------------------------
// Generator state
// ---------------------------------------------------------------------------

type goGen struct {
	buf          *strings.Builder
	indent       int
	typeEnv      typechecker.TypeEnv
	funcs        map[string]*goFuncInfo
	adts         map[string]*goAdtInfo
	ctorToAdt    map[string]*goCtorInfo
	records      map[string]*goRecordInfo
	traitImpls   map[string][]goImplCase
	usedBuiltins map[string]bool
	locals       map[string]bool
	tempCounter  int

	// trait method names → dispatch function names
	traitMethodNames map[string]string // "myShow" → "dispatch_myshow_myShow"

	// track what features are used so we can emit the right imports/helpers
	usesLists        bool
	usesTuples       map[int]bool
	usesStringConcat bool
	usesShow         bool
	usesStringInterp bool
	usesConcurrency  bool
}

func (g *goGen) fresh() string {
	g.tempCounter++
	return fmt.Sprintf("_t%d", g.tempCounter)
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func (g *goGen) w(format string, args ...any) {
	for i := 0; i < g.indent; i++ {
		g.buf.WriteByte('\t')
	}
	fmt.Fprintf(g.buf, format, args...)
	g.buf.WriteByte('\n')
}

func (g *goGen) wn(format string, args ...any) {
	// write without newline
	for i := 0; i < g.indent; i++ {
		g.buf.WriteByte('\t')
	}
	fmt.Fprintf(g.buf, format, args...)
}

func (g *goGen) raw(s string) {
	g.buf.WriteString(s)
	g.buf.WriteByte('\n')
}

// ---------------------------------------------------------------------------
// Main emit
// ---------------------------------------------------------------------------

func (g *goGen) emit(prog *ir.Program) (string, error) {
	// Phase 1: analyze
	g.analyze(prog)

	// Phase 2: emit
	out := &strings.Builder{}

	// Emit package + imports
	out.WriteString("package main\n\n")
	out.WriteString(g.emitImports())

	// Emit type definitions (ADTs, records, tuples)
	out.WriteString(g.emitTypeDefinitions())

	// Emit runtime helpers
	out.WriteString(g.emitRuntimeHelpers())

	// Emit trait dispatch functions
	out.WriteString(g.emitTraitDispatchers())

	// Emit top-level functions
	g.buf = out
	for _, d := range prog.Decls {
		if err := g.emitDecl(d); err != nil {
			return "", err
		}
	}

	// Emit Go main()
	out.WriteString("\nfunc main() {\n")
	out.WriteString("\tos.Exit(int(rex_main(nil).(int64)))\n")
	out.WriteString("}\n")

	return out.String(), nil
}

// ---------------------------------------------------------------------------
// Phase 1: Analyze
// ---------------------------------------------------------------------------

func (g *goGen) analyze(prog *ir.Program) {
	g.usesTuples = make(map[int]bool)
	g.traitMethodNames = make(map[string]string)

	// First pass: collect trait method names from DImpl declarations
	for _, d := range prog.Decls {
		if di, ok := d.(ir.DImpl); ok {
			for _, m := range di.Methods {
				dispatchName := fmt.Sprintf("dispatch_%s_%s", strings.ToLower(di.TraitName), m.Name)
				g.traitMethodNames[m.Name] = dispatchName
			}
		}
	}

	for _, d := range prog.Decls {
		switch d := d.(type) {
		case ir.DLet:
			fi := g.analyzeFunc(d)
			g.funcs[d.Name] = fi
			g.scanExpr(d.Body)

		case ir.DLetRec:
			for _, b := range d.Bindings {
				fi := g.analyzeFuncFromBinding(b)
				g.funcs[b.Name] = fi
				g.scanExpr(b.Bind.(ir.CLambda).Body)
			}

		case ir.DType:
			if len(d.Fields) > 0 {
				// Record type
				ri := &goRecordInfo{name: d.Name}
				for _, f := range d.Fields {
					ri.fieldNames = append(ri.fieldNames, f.Name)
					ri.fieldTypes = append(ri.fieldTypes, f.Ty)
				}
				g.records[d.Name] = ri
			} else if len(d.Ctors) > 0 {
				// ADT
				ai := &goAdtInfo{name: d.Name}
				for i, c := range d.Ctors {
					ci := goCtorInfo{
						name:     c.Name,
						tag:      i,
						typeName: d.Name,
					}
					for _, t := range c.ArgTypes {
						ci.fieldTypes = append(ci.fieldTypes, t)
					}
					ai.ctors = append(ai.ctors, ci)
					g.ctorToAdt[c.Name] = &ai.ctors[len(ai.ctors)-1]
				}
				g.adts[d.Name] = ai
			}

		case ir.DImpl:
			for _, m := range d.Methods {
				key := d.TraitName + ":" + m.Name
				funcName := fmt.Sprintf("impl_%s_%s_%s", d.TraitName, d.TargetTypeName, m.Name)
				g.traitImpls[key] = append(g.traitImpls[key], goImplCase{
					typeName: d.TargetTypeName,
					funcName: funcName,
				})
				g.scanExpr(m.Body)
			}
		}
	}
}

func (g *goGen) analyzeFunc(d ir.DLet) *goFuncInfo {
	fi := &goFuncInfo{name: d.Name}
	body := d.Body
	for {
		switch e := body.(type) {
		case ir.EComplex:
			if lam, ok := e.C.(ir.CLambda); ok {
				fi.params = append(fi.params, goParamInfo{name: lam.Param, ty: paramType(lam.Ty)})
				fi.arity++
				body = lam.Body
				continue
			}
		}
		break
	}
	fi.body = body
	fi.retType = g.inferReturnType(d.Name, fi.arity)
	return fi
}

func (g *goGen) analyzeFuncFromBinding(b ir.RecBinding) *goFuncInfo {
	fi := &goFuncInfo{name: b.Name}
	lam, ok := b.Bind.(ir.CLambda)
	if !ok {
		fi.body = ir.EComplex{C: b.Bind}
		return fi
	}
	body := ir.Expr(ir.EComplex{C: lam})
	for {
		switch e := body.(type) {
		case ir.EComplex:
			if l, ok := e.C.(ir.CLambda); ok {
				fi.params = append(fi.params, goParamInfo{name: l.Param, ty: paramType(l.Ty)})
				fi.arity++
				body = l.Body
				continue
			}
		}
		break
	}
	fi.body = body
	return fi
}

// paramType extracts the parameter type from a function type.
func paramType(ty types.Type) types.Type {
	if tc, ok := ty.(types.TCon); ok && tc.Name == "Fun" && len(tc.Args) == 2 {
		return tc.Args[0]
	}
	return nil
}

func (g *goGen) inferReturnType(name string, arity int) types.Type {
	s, ok := g.typeEnv[name]
	if !ok {
		return nil
	}
	scheme, ok := s.(types.Scheme)
	if !ok {
		return nil
	}
	ty := scheme.Ty
	for i := 0; i < arity; i++ {
		if tc, ok := ty.(types.TCon); ok && tc.Name == "Fun" && len(tc.Args) == 2 {
			ty = tc.Args[1]
		} else {
			return nil
		}
	}
	return ty
}

// scanExpr scans an expression to detect feature usage.
func (g *goGen) scanExpr(expr ir.Expr) {
	switch e := expr.(type) {
	case ir.EAtom:
		g.scanAtom(e.A)
	case ir.EComplex:
		g.scanCExpr(e.C)
	case ir.ELet:
		g.scanCExpr(e.Bind)
		g.scanExpr(e.Body)
	case ir.ELetRec:
		for _, b := range e.Bindings {
			g.scanCExpr(b.Bind)
		}
		g.scanExpr(e.Body)
	}
}

func (g *goGen) scanAtom(a ir.Atom) {
	if v, ok := a.(ir.AVar); ok {
		switch v.Name {
		case "println", "print":
			g.usedBuiltins[v.Name] = true
		case "showInt", "showFloat":
			g.usedBuiltins[v.Name] = true
			g.usesShow = true
		case "not", "error", "todo":
			g.usedBuiltins[v.Name] = true
		}
	}
}

func (g *goGen) scanCExpr(c ir.CExpr) {
	switch c := c.(type) {
	case ir.CApp:
		g.scanAtom(c.Func)
		g.scanAtom(c.Arg)
	case ir.CBinop:
		if c.Op == "Cons" {
			g.usesLists = true
		}
		if c.Op == "Concat" {
			g.usesStringConcat = true
		}
	case ir.CIf:
		g.scanExpr(c.Then)
		g.scanExpr(c.Else)
	case ir.CMatch:
		for _, arm := range c.Arms {
			g.scanExpr(arm.Body)
		}
	case ir.CLambda:
		g.scanExpr(c.Body)
	case ir.CList:
		g.usesLists = true
	case ir.CTuple:
		g.usesTuples[len(c.Items)] = true
	case ir.CStringInterp:
		g.usesStringInterp = true
		g.usesShow = true
	}
}

// ---------------------------------------------------------------------------
// Imports
// ---------------------------------------------------------------------------

func (g *goGen) emitImports() string {
	// Always need os (for os.Exit), fmt and strconv (for rex_display)
	imports := []string{`"fmt"`, `"os"`, `"strconv"`}

	// Only import "strings" when list display uses strings.Join
	if g.usesLists {
		imports = append(imports, `"strings"`)
	}

	var b strings.Builder
	b.WriteString("import (\n")
	for _, imp := range imports {
		fmt.Fprintf(&b, "\t%s\n", imp)
	}
	b.WriteString(")\n\n")
	return b.String()
}

// ---------------------------------------------------------------------------
// Type definitions
// ---------------------------------------------------------------------------

func (g *goGen) emitTypeDefinitions() string {
	var b strings.Builder

	// ADT interface + structs
	for _, adt := range g.adts {
		ifaceName := goTypeName(adt.name)
		fmt.Fprintf(&b, "type %s interface{ tag%s() int }\n\n", ifaceName, ifaceName)

		for _, ctor := range adt.ctors {
			structName := goCtorStructName(ctor.typeName, ctor.name)
			if len(ctor.fieldTypes) == 0 {
				fmt.Fprintf(&b, "type %s struct{}\n", structName)
			} else {
				fmt.Fprintf(&b, "type %s struct {\n", structName)
				for i, ft := range ctor.fieldTypes {
					fmt.Fprintf(&b, "\tF%d %s\n", i, g.goType(ft))
				}
				b.WriteString("}\n")
			}
			fmt.Fprintf(&b, "func (%s) tag%s() int { return %d }\n\n",
				structName, ifaceName, ctor.tag)
		}
	}

	// Record structs
	for _, rec := range g.records {
		structName := goTypeName(rec.name)
		fmt.Fprintf(&b, "type %s struct {\n", structName)
		for i, fname := range rec.fieldNames {
			fmt.Fprintf(&b, "\t%s %s\n", goExportedField(fname), g.goType(rec.fieldTypes[i]))
		}
		b.WriteString("}\n\n")
	}

	// Tuple structs
	for arity := range g.usesTuples {
		if arity < 2 {
			continue
		}
		name := fmt.Sprintf("Tuple%d", arity)
		fmt.Fprintf(&b, "type %s struct {\n", name)
		for i := 0; i < arity; i++ {
			fmt.Fprintf(&b, "\tF%d any\n", i)
		}
		b.WriteString("}\n\n")
	}

	// List type
	if g.usesLists {
		b.WriteString("type RexList struct {\n")
		b.WriteString("\tHead any\n")
		b.WriteString("\tTail *RexList\n")
		b.WriteString("}\n\n")
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Runtime helpers
// ---------------------------------------------------------------------------

func (g *goGen) emitRuntimeHelpers() string {
	var b strings.Builder

	// Structural equality
	b.WriteString("func rex_eq(a, b any) bool {\n")
	b.WriteString("\tswitch av := a.(type) {\n")
	b.WriteString("\tcase int64:\n")
	b.WriteString("\t\tbv, ok := b.(int64); return ok && av == bv\n")
	b.WriteString("\tcase float64:\n")
	b.WriteString("\t\tbv, ok := b.(float64); return ok && av == bv\n")
	b.WriteString("\tcase string:\n")
	b.WriteString("\t\tbv, ok := b.(string); return ok && av == bv\n")
	b.WriteString("\tcase bool:\n")
	b.WriteString("\t\tbv, ok := b.(bool); return ok && av == bv\n")
	b.WriteString("\tcase nil:\n")
	b.WriteString("\t\treturn b == nil\n")
	if g.usesLists {
		b.WriteString("\tcase *RexList:\n")
		b.WriteString("\t\tbv, ok := b.(*RexList)\n")
		b.WriteString("\t\tif !ok { return false }\n")
		b.WriteString("\t\tif av == nil && bv == nil { return true }\n")
		b.WriteString("\t\tif av == nil || bv == nil { return false }\n")
		b.WriteString("\t\treturn rex_eq(av.Head, bv.Head) && rex_eq(av.Tail, bv.Tail)\n")
	}
	b.WriteString("\tdefault:\n")
	b.WriteString("\t\treturn false\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n\n")

	// Structural comparison (for <, >, <=, >=)
	b.WriteString("func rex_compare(a, b any) int {\n")
	b.WriteString("\tswitch av := a.(type) {\n")
	b.WriteString("\tcase int64:\n")
	b.WriteString("\t\tbv := b.(int64)\n")
	b.WriteString("\t\tif av < bv { return -1 }\n")
	b.WriteString("\t\tif av > bv { return 1 }\n")
	b.WriteString("\t\treturn 0\n")
	b.WriteString("\tcase float64:\n")
	b.WriteString("\t\tbv := b.(float64)\n")
	b.WriteString("\t\tif av < bv { return -1 }\n")
	b.WriteString("\t\tif av > bv { return 1 }\n")
	b.WriteString("\t\treturn 0\n")
	b.WriteString("\tcase string:\n")
	b.WriteString("\t\tbv := b.(string)\n")
	b.WriteString("\t\tif av < bv { return -1 }\n")
	b.WriteString("\t\tif av > bv { return 1 }\n")
	b.WriteString("\t\treturn 0\n")
	b.WriteString("\tcase bool:\n")
	b.WriteString("\t\tbv := b.(bool)\n")
	b.WriteString("\t\tai, bi := 0, 0\n")
	b.WriteString("\t\tif av { ai = 1 }\n")
	b.WriteString("\t\tif bv { bi = 1 }\n")
	b.WriteString("\t\tif ai < bi { return -1 }\n")
	b.WriteString("\t\tif ai > bi { return 1 }\n")
	b.WriteString("\t\treturn 0\n")
	b.WriteString("\t}\n")
	b.WriteString("\treturn 0\n")
	b.WriteString("}\n\n")

	// println / print builtins
	if g.usedBuiltins["println"] {
		b.WriteString("func rex_println(v any) any {\n")
		b.WriteString("\tfmt.Println(rex_display(v))\n")
		b.WriteString("\treturn nil\n")
		b.WriteString("}\n\n")
	}
	if g.usedBuiltins["print"] {
		b.WriteString("func rex_print(v any) any {\n")
		b.WriteString("\tfmt.Print(rex_display(v))\n")
		b.WriteString("\treturn nil\n")
		b.WriteString("}\n\n")
	}

	// display helper (always emitted since rex_eq might need it for debug)
	b.WriteString("func rex_display(v any) string {\n")
	b.WriteString("\tswitch val := v.(type) {\n")
	b.WriteString("\tcase nil:\n")
	b.WriteString("\t\treturn \"()\"\n")
	b.WriteString("\tcase int64:\n")
	b.WriteString("\t\treturn strconv.FormatInt(val, 10)\n")
	b.WriteString("\tcase float64:\n")
	b.WriteString("\t\treturn strconv.FormatFloat(val, 'g', -1, 64)\n")
	b.WriteString("\tcase string:\n")
	b.WriteString("\t\treturn val\n")
	b.WriteString("\tcase bool:\n")
	b.WriteString("\t\tif val { return \"true\" }\n")
	b.WriteString("\t\treturn \"false\"\n")
	if g.usesLists {
		b.WriteString("\tcase *RexList:\n")
		b.WriteString("\t\tif val == nil { return \"[]\" }\n")
		b.WriteString("\t\tvar parts []string\n")
		b.WriteString("\t\tfor l := val; l != nil; l = l.Tail {\n")
		b.WriteString("\t\t\tparts = append(parts, rex_display(l.Head))\n")
		b.WriteString("\t\t}\n")
		b.WriteString("\t\treturn \"[\" + strings.Join(parts, \", \") + \"]\"\n")
	}
	b.WriteString("\tdefault:\n")
	b.WriteString("\t\treturn fmt.Sprintf(\"%v\", val)\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n\n")

	// showInt / showFloat builtins
	if g.usedBuiltins["showInt"] {
		b.WriteString("func rex_showInt(v any) any {\n")
		b.WriteString("\treturn strconv.FormatInt(v.(int64), 10)\n")
		b.WriteString("}\n\n")
	}
	if g.usedBuiltins["showFloat"] {
		b.WriteString("func rex_showFloat(v any) any {\n")
		b.WriteString("\treturn strconv.FormatFloat(v.(float64), 'g', -1, 64)\n")
		b.WriteString("}\n\n")
	}

	// error / todo builtins
	if g.usedBuiltins["error"] {
		b.WriteString("func rex_error(msg any) any {\n")
		b.WriteString("\tpanic(fmt.Sprintf(\"error: %s\", msg.(string)))\n")
		b.WriteString("}\n\n")
	}
	if g.usedBuiltins["todo"] {
		b.WriteString("func rex_todo(msg any) any {\n")
		b.WriteString("\tpanic(fmt.Sprintf(\"TODO: %s\", msg.(string)))\n")
		b.WriteString("}\n\n")
	}

	// not builtin
	if g.usedBuiltins["not"] {
		b.WriteString("func rex_not(v any) any {\n")
		b.WriteString("\treturn !v.(bool)\n")
		b.WriteString("}\n\n")
	}

	// Partial application helpers
	b.WriteString("func rex__apply(f any, arg any) any {\n")
	b.WriteString("\tswitch fn := f.(type) {\n")
	b.WriteString("\tcase func(any) any:\n")
	b.WriteString("\t\treturn fn(arg)\n")
	b.WriteString("\tdefault:\n")
	b.WriteString("\t\tpanic(\"rex__apply: not a function\")\n")
	b.WriteString("\t}\n")
	b.WriteString("}\n\n")

	return b.String()
}

// ---------------------------------------------------------------------------
// Trait dispatch
// ---------------------------------------------------------------------------

func (g *goGen) emitTraitDispatchers() string {
	var b strings.Builder

	for key, cases := range g.traitImpls {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		traitName, methodName := parts[0], parts[1]
		dispatchName := fmt.Sprintf("dispatch_%s_%s", strings.ToLower(traitName), methodName)

		fmt.Fprintf(&b, "func %s(args ...any) any {\n", dispatchName)
		b.WriteString("\tv := args[0]\n")
		b.WriteString("\tswitch v.(type) {\n")
		for _, c := range cases {
			fmt.Fprintf(&b, "\tcase %s:\n", g.goTypeForDispatch(c.typeName))
			fmt.Fprintf(&b, "\t\treturn %s(args...)\n", c.funcName)
		}
		b.WriteString("\tdefault:\n")
		fmt.Fprintf(&b, "\t\tpanic(\"no %s.%s instance\")\n", traitName, methodName)
		b.WriteString("\t}\n")
		b.WriteString("}\n\n")
	}

	return b.String()
}

func (g *goGen) goTypeForDispatch(typeName string) string {
	switch typeName {
	case "Int":
		return "int64"
	case "Float":
		return "float64"
	case "String":
		return "string"
	case "Bool":
		return "bool"
	case "List":
		return "*RexList"
	default:
		return goTypeName(typeName)
	}
}

// ---------------------------------------------------------------------------
// Emit declarations
// ---------------------------------------------------------------------------

func (g *goGen) emitDecl(d ir.Decl) error {
	switch d := d.(type) {
	case ir.DLet:
		return g.emitDLet(d)
	case ir.DLetRec:
		return g.emitDLetRec(d)
	case ir.DImpl:
		return g.emitDImpl(d)
	case ir.DType, ir.DTrait, ir.DImport, ir.DTest:
		// handled in analyze or skipped
		return nil
	default:
		return nil
	}
}

func (g *goGen) emitDLet(d ir.DLet) error {
	fi := g.funcs[d.Name]
	if fi == nil {
		return nil
	}

	goName := goFuncName(d.Name)

	if fi.arity == 0 {
		// Top-level value (not a function) — emit as var
		g.buf.WriteString(fmt.Sprintf("var %s = ", goName))
		g.locals = make(map[string]bool)
		if err := g.emitExprInline(fi.body); err != nil {
			return err
		}
		g.buf.WriteString("\n\n")
		return nil
	}

	// Function
	g.buf.WriteString(fmt.Sprintf("func %s(", goName))
	for i, p := range fi.params {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		fmt.Fprintf(g.buf, "%s any", goVarName(p.name))
	}
	g.buf.WriteString(") any {\n")

	g.indent = 1
	g.locals = make(map[string]bool)
	for _, p := range fi.params {
		g.locals[p.name] = true
	}

	if err := g.emitExprStmt(fi.body, true); err != nil {
		return err
	}

	g.buf.WriteString("}\n\n")
	g.indent = 0
	return nil
}

func (g *goGen) emitDLetRec(d ir.DLetRec) error {
	for _, b := range d.Bindings {
		fi := g.funcs[b.Name]
		if fi == nil {
			continue
		}
		goName := goFuncName(b.Name)
		g.buf.WriteString(fmt.Sprintf("func %s(", goName))
		for i, p := range fi.params {
			if i > 0 {
				g.buf.WriteString(", ")
			}
			fmt.Fprintf(g.buf, "%s any", goVarName(p.name))
		}
		g.buf.WriteString(") any {\n")

		g.indent = 1
		g.locals = make(map[string]bool)
		for _, p := range fi.params {
			g.locals[p.name] = true
		}

		if err := g.emitExprStmt(fi.body, true); err != nil {
			return err
		}

		g.buf.WriteString("}\n\n")
		g.indent = 0
	}
	return nil
}

func (g *goGen) emitDImpl(d ir.DImpl) error {
	for _, m := range d.Methods {
		funcName := fmt.Sprintf("impl_%s_%s_%s", d.TraitName, d.TargetTypeName, m.Name)

		// Unwrap lambda chain
		var params []string
		body := m.Body
		for {
			if ec, ok := body.(ir.EComplex); ok {
				if lam, ok := ec.C.(ir.CLambda); ok {
					params = append(params, lam.Param)
					body = lam.Body
					continue
				}
			}
			break
		}

		g.buf.WriteString(fmt.Sprintf("func %s(args ...any) any {\n", funcName))
		g.indent = 1
		g.locals = make(map[string]bool)
		for i, p := range params {
			vn := goVarName(p)
			if p == "_" {
				g.w("_ = args[%d]", i)
			} else {
				g.w("%s := args[%d]", vn, i)
			}
			g.locals[p] = true
		}

		if err := g.emitExprStmt(body, true); err != nil {
			return err
		}

		g.buf.WriteString("}\n\n")
		g.indent = 0
	}
	return nil
}

// ---------------------------------------------------------------------------
// Emit expressions
// ---------------------------------------------------------------------------

// emitExprStmt emits an expression as a statement. If isReturn is true,
// the result is returned.
func (g *goGen) emitExprStmt(expr ir.Expr, isReturn bool) error {
	switch e := expr.(type) {
	case ir.EAtom:
		if isReturn {
			g.wn("return ")
			g.emitAtom(e.A)
			g.buf.WriteByte('\n')
		} else {
			g.wn("")
			g.emitAtom(e.A)
			g.buf.WriteByte('\n')
		}
		return nil

	case ir.EComplex:
		return g.emitCExprStmt(e.C, isReturn)

	case ir.ELet:
		varName := goVarName(e.Name)
		// Use explicit any type to avoid Go type issues when value is used polymorphically
		g.wn("var %s any = ", varName)
		if err := g.emitCExprInline(e.Bind); err != nil {
			return err
		}
		g.buf.WriteByte('\n')
		g.locals[e.Name] = true
		return g.emitExprStmt(e.Body, isReturn)

	case ir.ELetRec:
		// Declare variables first, then assign (for mutual recursion)
		for _, b := range e.Bindings {
			g.w("var %s any", goVarName(b.Name))
			g.locals[b.Name] = true
		}
		for _, b := range e.Bindings {
			g.wn("%s = ", goVarName(b.Name))
			if err := g.emitCExprInline(b.Bind); err != nil {
				return err
			}
			g.buf.WriteByte('\n')
		}
		return g.emitExprStmt(e.Body, isReturn)
	}
	return fmt.Errorf("unknown expr type: %T", expr)
}

// emitExprInline emits an expression as an inline value (for var initializers etc).
func (g *goGen) emitExprInline(expr ir.Expr) error {
	switch e := expr.(type) {
	case ir.EAtom:
		g.emitAtom(e.A)
		return nil
	case ir.EComplex:
		return g.emitCExprInline(e.C)
	default:
		// Complex expressions that need statements: wrap in IIFE
		g.buf.WriteString("func() any {\n")
		g.indent++
		if err := g.emitExprStmt(expr, true); err != nil {
			return err
		}
		g.indent--
		for i := 0; i < g.indent; i++ {
			g.buf.WriteByte('\t')
		}
		g.buf.WriteString("}()")
		return nil
	}
}

// emitCExprStmt emits a complex expression as a statement.
func (g *goGen) emitCExprStmt(c ir.CExpr, isReturn bool) error {
	switch c := c.(type) {
	case ir.CIf:
		g.w("if %s {", g.atomToBool(c.Cond))
		g.indent++
		if err := g.emitExprStmt(c.Then, isReturn); err != nil {
			return err
		}
		g.indent--
		g.w("} else {")
		g.indent++
		if err := g.emitExprStmt(c.Else, isReturn); err != nil {
			return err
		}
		g.indent--
		g.w("}")
		return nil

	case ir.CMatch:
		return g.emitMatch(c, isReturn)

	default:
		if isReturn {
			g.wn("return ")
		} else {
			g.wn("")
		}
		if err := g.emitCExprInline(c); err != nil {
			return err
		}
		g.buf.WriteByte('\n')
		return nil
	}
}

// emitCExprInline emits a complex expression as an inline value.
func (g *goGen) emitCExprInline(c ir.CExpr) error {
	switch c := c.(type) {
	case ir.CApp:
		return g.emitApp(c)

	case ir.CBinop:
		return g.emitBinop(c)

	case ir.CUnaryMinus:
		g.buf.WriteString("(-")
		g.emitAtomTyped(c.Expr, c.Ty)
		g.buf.WriteString(")")
		return nil

	case ir.CIf:
		// Inline if → Go ternary via IIFE
		g.buf.WriteString("func() any {\n")
		g.indent++
		g.w("if %s {", g.atomToBool(c.Cond))
		g.indent++
		if err := g.emitExprStmt(c.Then, true); err != nil {
			return err
		}
		g.indent--
		g.w("} else {")
		g.indent++
		if err := g.emitExprStmt(c.Else, true); err != nil {
			return err
		}
		g.indent--
		g.w("}")
		g.indent--
		for i := 0; i < g.indent; i++ {
			g.buf.WriteByte('\t')
		}
		g.buf.WriteString("}()")
		return nil

	case ir.CMatch:
		// Inline match → IIFE
		g.buf.WriteString("func() any {\n")
		g.indent++
		if err := g.emitMatch(c, true); err != nil {
			return err
		}
		g.indent--
		for i := 0; i < g.indent; i++ {
			g.buf.WriteByte('\t')
		}
		g.buf.WriteString("}()")
		return nil

	case ir.CLambda:
		return g.emitLambda(c)

	case ir.CCtor:
		return g.emitCtor(c)

	case ir.CRecord:
		return g.emitRecord(c)

	case ir.CFieldAccess:
		return g.emitFieldAccess(c)

	case ir.CRecordUpdate:
		return g.emitRecordUpdate(c)

	case ir.CList:
		return g.emitList(c)

	case ir.CTuple:
		return g.emitTuple(c)

	case ir.CStringInterp:
		return g.emitStringInterp(c)
	}
	return fmt.Errorf("unknown cexpr type: %T", c)
}

// ---------------------------------------------------------------------------
// Application
// ---------------------------------------------------------------------------

func (g *goGen) emitApp(c ir.CApp) error {
	funcName := ""
	if v, ok := c.Func.(ir.AVar); ok {
		funcName = v.Name
	}

	// Known builtins
	switch funcName {
	case "__id":
		// Identity function — just return the argument
		g.emitAtom(c.Arg)
		return nil
	case "println":
		g.buf.WriteString("rex_println(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "print":
		g.buf.WriteString("rex_print(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "showInt":
		g.buf.WriteString("rex_showInt(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "showFloat":
		g.buf.WriteString("rex_showFloat(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "not":
		g.buf.WriteString("rex_not(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "error":
		g.buf.WriteString("rex_error(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "todo":
		g.buf.WriteString("rex_todo(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	}

	// Trait method dispatch
	if dispatchName, ok := g.traitMethodNames[funcName]; ok {
		fmt.Fprintf(g.buf, "%s(", dispatchName)
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	}

	// Known top-level function: direct call or partial application
	if fi, ok := g.funcs[funcName]; ok && fi.arity > 0 {
		goName := goFuncName(funcName)
		if fi.arity == 1 {
			// Full application — direct call
			fmt.Fprintf(g.buf, "%s(", goName)
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
		} else {
			// Partial application — return a closure that collects remaining args
			g.emitPartialApp(goName, fi.arity, c.Arg)
		}
		return nil
	}

	// Unknown / variable function: use rex__apply
	g.buf.WriteString("rex__apply(")
	g.emitAtom(c.Func)
	g.buf.WriteString(", ")
	g.emitAtom(c.Arg)
	g.buf.WriteString(")")
	return nil
}

// emitPartialApp emits a closure that captures the first arg and waits for remaining args.
// For a 2-arg function f, emitPartialApp("f", 2, arg) emits:
//
//	func(a1 any) any { return f(arg, a1) }
//
// For a 3-arg function, it nests further.
func (g *goGen) emitPartialApp(goName string, arity int, firstArg ir.Atom) {
	remaining := arity - 1
	// Build nested closures from outside in
	var params []string
	for i := 0; i < remaining; i++ {
		param := fmt.Sprintf("_pa%d", i)
		params = append(params, param)
		fmt.Fprintf(g.buf, "func(%s any) any { return ", param)
	}
	// Innermost: call the function with all args
	fmt.Fprintf(g.buf, "%s(", goName)
	g.emitAtom(firstArg)
	for _, p := range params {
		fmt.Fprintf(g.buf, ", %s", p)
	}
	g.buf.WriteString(")")
	// Close all the nested functions
	for range params {
		g.buf.WriteString(" }")
	}
}

// ---------------------------------------------------------------------------
// Binary operators
// ---------------------------------------------------------------------------

func (g *goGen) emitBinop(c ir.CBinop) error {
	switch c.Op {
	case "Add":
		return g.emitArithBinop(c, "+")
	case "Sub":
		return g.emitArithBinop(c, "-")
	case "Mul":
		return g.emitArithBinop(c, "*")
	case "Div":
		return g.emitArithBinop(c, "/")
	case "Mod":
		g.buf.WriteString("(")
		g.emitAtomTyped(c.Left, c.Ty)
		g.buf.WriteString(" % ")
		g.emitAtomTyped(c.Right, c.Ty)
		g.buf.WriteString(")")
		return nil
	case "Eq":
		g.buf.WriteString("rex_eq(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Neq":
		g.buf.WriteString("!rex_eq(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Lt":
		g.buf.WriteString("(rex_compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") < 0)")
		return nil
	case "Gt":
		g.buf.WriteString("(rex_compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") > 0)")
		return nil
	case "Leq":
		g.buf.WriteString("(rex_compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") <= 0)")
		return nil
	case "Geq":
		g.buf.WriteString("(rex_compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") >= 0)")
		return nil
	case "And":
		g.buf.WriteString("(")
		g.emitAtomAsBool(c.Left)
		g.buf.WriteString(" && ")
		g.emitAtomAsBool(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Or":
		g.buf.WriteString("(")
		g.emitAtomAsBool(c.Left)
		g.buf.WriteString(" || ")
		g.emitAtomAsBool(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Concat":
		g.buf.WriteString("(")
		g.emitAtomAsString(c.Left)
		g.buf.WriteString(" + ")
		g.emitAtomAsString(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Cons":
		g.buf.WriteString("&RexList{Head: ")
		g.emitAtom(c.Left)
		g.buf.WriteString(", Tail: ")
		g.emitAtomAsList(c.Right)
		g.buf.WriteString("}")
		return nil
	}
	return fmt.Errorf("unknown binop: %s", c.Op)
}

func (g *goGen) emitArithBinop(c ir.CBinop, op string) error {
	isFloat := isFloatType(c.Ty)
	g.buf.WriteString("(")
	if isFloat {
		g.emitAtomAsFloat(c.Left)
	} else {
		g.emitAtomAsInt(c.Left)
	}
	fmt.Fprintf(g.buf, " %s ", op)
	if isFloat {
		g.emitAtomAsFloat(c.Right)
	} else {
		g.emitAtomAsInt(c.Right)
	}
	g.buf.WriteString(")")
	return nil
}

// ---------------------------------------------------------------------------
// Lambda / closure
// ---------------------------------------------------------------------------

func (g *goGen) emitLambda(c ir.CLambda) error {
	param := goVarName(c.Param)
	g.buf.WriteString(fmt.Sprintf("func(%s any) any {\n", param))
	oldLocals := g.locals
	g.locals = make(map[string]bool)
	for k := range oldLocals {
		g.locals[k] = true
	}
	g.locals[c.Param] = true
	g.indent++
	if err := g.emitExprStmt(c.Body, true); err != nil {
		return err
	}
	g.indent--
	for i := 0; i < g.indent; i++ {
		g.buf.WriteByte('\t')
	}
	g.buf.WriteString("}")
	g.locals = oldLocals
	return nil
}

// ---------------------------------------------------------------------------
// ADT constructors
// ---------------------------------------------------------------------------

func (g *goGen) emitCtor(c ir.CCtor) error {
	ci, ok := g.ctorToAdt[c.Name]
	if !ok {
		return fmt.Errorf("unknown constructor: %s", c.Name)
	}
	structName := goCtorStructName(ci.typeName, c.Name)
	if len(c.Args) == 0 {
		fmt.Fprintf(g.buf, "%s{}", structName)
		return nil
	}
	fmt.Fprintf(g.buf, "%s{", structName)
	for i, a := range c.Args {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitAtom(a)
	}
	g.buf.WriteString("}")
	return nil
}

// ---------------------------------------------------------------------------
// Records
// ---------------------------------------------------------------------------

func (g *goGen) emitRecord(c ir.CRecord) error {
	structName := goTypeName(c.TypeName)
	fmt.Fprintf(g.buf, "%s{", structName)
	for i, f := range c.Fields {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		fmt.Fprintf(g.buf, "%s: ", goExportedField(f.Name))
		g.emitAtom(f.Value)
	}
	g.buf.WriteString("}")
	return nil
}

func (g *goGen) emitFieldAccess(c ir.CFieldAccess) error {
	g.emitAtom(c.Record)
	ri := g.findRecordForField(c.Field)
	if ri != nil {
		fmt.Fprintf(g.buf, ".(%s).%s", goTypeName(ri.name), goExportedField(c.Field))
	} else {
		// Fallback: try generic field access
		fmt.Fprintf(g.buf, ".(%s)", goExportedField(c.Field))
	}
	return nil
}

func (g *goGen) findRecordForField(field string) *goRecordInfo {
	for _, ri := range g.records {
		for _, fn := range ri.fieldNames {
			if fn == field {
				return ri
			}
		}
	}
	return nil
}

func (g *goGen) emitRecordUpdate(c ir.CRecordUpdate) error {
	// Find record type
	var recTypeName string
	for _, ri := range g.records {
		// Try to figure out the type from the record atom
		recTypeName = ri.name
		break // TODO: improve type resolution
	}
	if recTypeName == "" {
		return fmt.Errorf("cannot determine record type for update")
	}

	structName := goTypeName(recTypeName)
	g.buf.WriteString("func() any {\n")
	g.indent++
	g.w("r := %s", g.atomStr(c.Record))
	g.w("copy := r.(%s)", structName)
	for _, u := range c.Updates {
		if len(u.Path) == 1 {
			g.wn("copy.%s = ", goExportedField(u.Path[0]))
			g.emitAtom(u.Value)
			g.buf.WriteByte('\n')
		}
		// TODO: nested paths
	}
	g.w("return copy")
	g.indent--
	for i := 0; i < g.indent; i++ {
		g.buf.WriteByte('\t')
	}
	g.buf.WriteString("}()")
	return nil
}

// ---------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------

func (g *goGen) emitList(c ir.CList) error {
	if len(c.Items) == 0 {
		g.buf.WriteString("(*RexList)(nil)")
		return nil
	}
	// Build cons list from right to left
	g.buf.WriteString("&RexList{Head: ")
	g.emitAtom(c.Items[0])
	for i := 1; i < len(c.Items); i++ {
		g.buf.WriteString(", Tail: &RexList{Head: ")
		g.emitAtom(c.Items[i])
	}
	// Close with nil tails
	g.buf.WriteString(", Tail: (*RexList)(nil)")
	for i := 1; i < len(c.Items); i++ {
		g.buf.WriteString("}")
	}
	g.buf.WriteString("}")
	return nil
}

// ---------------------------------------------------------------------------
// Tuples
// ---------------------------------------------------------------------------

func (g *goGen) emitTuple(c ir.CTuple) error {
	if len(c.Items) == 0 {
		g.buf.WriteString("nil") // unit
		return nil
	}
	fmt.Fprintf(g.buf, "Tuple%d{", len(c.Items))
	for i, item := range c.Items {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitAtom(item)
	}
	g.buf.WriteString("}")
	return nil
}

// ---------------------------------------------------------------------------
// String interpolation
// ---------------------------------------------------------------------------

func (g *goGen) emitStringInterp(c ir.CStringInterp) error {
	if len(c.Parts) == 0 {
		g.buf.WriteString(`""`)
		return nil
	}
	if len(c.Parts) == 1 {
		g.buf.WriteString("rex_display(")
		g.emitAtom(c.Parts[0])
		g.buf.WriteString(")")
		return nil
	}
	// Use a builder pattern for multiple parts
	parts := make([]string, 0, len(c.Parts))
	for _, p := range c.Parts {
		if s, ok := p.(ir.AString); ok {
			parts = append(parts, fmt.Sprintf("%q", s.Value))
		} else {
			parts = append(parts, fmt.Sprintf("rex_display(%s)", g.atomStr(p)))
		}
	}
	g.buf.WriteString(strings.Join(parts, " + "))
	return nil
}

// ---------------------------------------------------------------------------
// Pattern matching
// ---------------------------------------------------------------------------

func (g *goGen) emitMatch(c ir.CMatch, isReturn bool) error {
	scrutVar := g.atomStr(c.Scrutinee)

	for i, arm := range c.Arms {
		if i == 0 {
			// First arm
		}
		cond, bindings := g.patternCondition(scrutVar, arm.Pat)

		if cond == "" || cond == "true" {
			// Unconditional (wildcard, variable)
			if i > 0 {
				g.w("} else {")
			}
			g.indent++
			for _, b := range bindings {
				g.w("%s := %s", goVarName(b.name), b.expr)
				g.locals[b.name] = true
			}
			if err := g.emitExprStmt(arm.Body, isReturn); err != nil {
				return err
			}
			g.indent--
			if i > 0 {
				g.w("}")
			}
			return nil
		}

		if i == 0 {
			g.w("if %s {", cond)
		} else {
			g.w("} else if %s {", cond)
		}
		g.indent++
		for _, b := range bindings {
			g.w("%s := %s", goVarName(b.name), b.expr)
			g.locals[b.name] = true
		}
		if err := g.emitExprStmt(arm.Body, isReturn); err != nil {
			return err
		}
		g.indent--
	}

	if len(c.Arms) > 0 {
		g.w("} else {")
		g.indent++
		g.w(`panic("non-exhaustive match")`)
		g.indent--
		g.w("}")
	}
	return nil
}

type patBinding struct {
	name string
	expr string
}

func (g *goGen) patternCondition(scrutExpr string, pat ir.Pattern) (string, []patBinding) {
	switch p := pat.(type) {
	case ir.PWild:
		return "true", nil

	case ir.PVar:
		return "true", []patBinding{{name: p.Name, expr: scrutExpr}}

	case ir.PInt:
		return fmt.Sprintf("rex_eq(%s, int64(%d))", scrutExpr, p.Value), nil

	case ir.PFloat:
		return fmt.Sprintf("rex_eq(%s, float64(%g))", scrutExpr, p.Value), nil

	case ir.PString:
		return fmt.Sprintf("rex_eq(%s, %q)", scrutExpr, p.Value), nil

	case ir.PBool:
		if p.Value {
			return fmt.Sprintf("rex_eq(%s, true)", scrutExpr), nil
		}
		return fmt.Sprintf("rex_eq(%s, false)", scrutExpr), nil

	case ir.PUnit:
		return "true", nil

	case ir.PNil:
		return fmt.Sprintf("%s.(*RexList) == nil || %s == nil", scrutExpr, scrutExpr), nil

	case ir.PCons:
		listVar := g.fresh()
		headCond, headBindings := g.patternCondition(fmt.Sprintf("%s.Head", listVar), p.Head)
		tailCond, tailBindings := g.patternCondition(fmt.Sprintf("any(%s.Tail)", listVar), p.Tail)

		cond := fmt.Sprintf("func() bool { if l, ok := %s.(*RexList); ok && l != nil { %s = l; return true }; return false }()", scrutExpr, listVar)

		// We need to declare the list var
		var bindings []patBinding
		bindings = append(bindings, patBinding{name: listVar[1:], expr: fmt.Sprintf("func() *RexList { if l, ok := %s.(*RexList); ok && l != nil { return l }; return nil }()", scrutExpr)})
		bindings = append(bindings, headBindings...)
		bindings = append(bindings, tailBindings...)

		// Hmm, this is getting complex. Let me simplify.
		// Actually, for pattern matching on cons lists, let's use a different approach.
		// Use a helper variable approach.

		// Simplify: emit the match as a type check + field access
		_ = headCond
		_ = tailCond
		_ = cond
		_ = bindings

		return g.emitConsPattern(scrutExpr, p)

	case ir.PTuple:
		var conds []string
		var bindings []patBinding
		for i, subPat := range p.Pats {
			fieldExpr := fmt.Sprintf("%s.(Tuple%d).F%d", scrutExpr, len(p.Pats), i)
			c, b := g.patternCondition(fieldExpr, subPat)
			if c != "" && c != "true" {
				conds = append(conds, c)
			}
			bindings = append(bindings, b...)
		}
		cond := "true"
		if len(conds) > 0 {
			cond = strings.Join(conds, " && ")
		}
		return cond, bindings

	case ir.PCtor:
		ci, ok := g.ctorToAdt[p.Name]
		if !ok {
			return "true", nil
		}
		structName := goCtorStructName(ci.typeName, p.Name)
		castExpr := fmt.Sprintf("%s.(%s)", scrutExpr, structName)

		var conds []string
		var bindings []patBinding
		conds = append(conds, fmt.Sprintf("func() bool { _, ok := %s; return ok }()", castExpr))

		for i, subPat := range p.Args {
			fieldExpr := fmt.Sprintf("%s.F%d", castExpr, i)
			c, b := g.patternCondition(fieldExpr, subPat)
			if c != "" && c != "true" {
				conds = append(conds, c)
			}
			bindings = append(bindings, b...)
		}

		return strings.Join(conds, " && "), bindings

	case ir.PRecord:
		ri := g.records[p.TypeName]
		if ri == nil {
			return "true", nil
		}
		structName := goTypeName(p.TypeName)
		var bindings []patBinding
		var conds []string
		for _, f := range p.Fields {
			fieldExpr := fmt.Sprintf("%s.(%s).%s", scrutExpr, structName, goExportedField(f.Name))
			c, b := g.patternCondition(fieldExpr, f.Pat)
			if c != "" && c != "true" {
				conds = append(conds, c)
			}
			bindings = append(bindings, b...)
		}
		cond := "true"
		if len(conds) > 0 {
			cond = strings.Join(conds, " && ")
		}
		return cond, bindings
	}
	return "true", nil
}

func (g *goGen) emitConsPattern(scrutExpr string, p ir.PCons) (string, []patBinding) {
	tmpName := g.fresh()
	goTmp := goVarName(tmpName)

	// Condition: scrutinee is a non-nil *RexList
	cond := fmt.Sprintf("func() bool { %s, _ := %s.(*RexList); return %s != nil }()",
		goTmp, scrutExpr, goTmp)

	// Actually, since we can't easily use temp vars in conditions,
	// let's use a simpler approach: check the type and extract in bindings.

	// Instead: use a single condition check, and extract fields in bindings
	cond = fmt.Sprintf("func() bool { l, ok := %s.(*RexList); return ok && l != nil }()", scrutExpr)

	var bindings []patBinding

	// Get head and tail
	headExpr := fmt.Sprintf("(%s.(*RexList)).Head", scrutExpr)
	tailExpr := fmt.Sprintf("any((%s.(*RexList)).Tail)", scrutExpr)

	headCond, headBindings := g.patternCondition(headExpr, p.Head)
	tailCond, tailBindings := g.patternCondition(tailExpr, p.Tail)

	bindings = append(bindings, headBindings...)
	bindings = append(bindings, tailBindings...)

	var allConds []string
	allConds = append(allConds, cond)
	if headCond != "" && headCond != "true" {
		allConds = append(allConds, headCond)
	}
	if tailCond != "" && tailCond != "true" {
		allConds = append(allConds, tailCond)
	}

	return strings.Join(allConds, " && "), bindings
}

// ---------------------------------------------------------------------------
// Atoms
// ---------------------------------------------------------------------------

func (g *goGen) emitAtom(a ir.Atom) {
	g.buf.WriteString(g.atomStr(a))
}

func (g *goGen) atomStr(a ir.Atom) string {
	switch a := a.(type) {
	case ir.AInt:
		return fmt.Sprintf("int64(%d)", a.Value)
	case ir.AFloat:
		return fmt.Sprintf("float64(%g)", a.Value)
	case ir.AString:
		return fmt.Sprintf("%q", a.Value)
	case ir.ABool:
		if a.Value {
			return "true"
		}
		return "false"
	case ir.AUnit:
		return "nil"
	case ir.AVar:
		name := a.Name
		// Check if it's a trait method — wrap as a closure calling the dispatch function
		if dispatchName, ok := g.traitMethodNames[name]; ok {
			return fmt.Sprintf("func(a any) any { return %s(a) }", dispatchName)
		}
		// Check if it's a known ADT constructor
		if ci, ok := g.ctorToAdt[name]; ok {
			if len(ci.fieldTypes) == 0 {
				// Nullary constructor — return as a value
				return goCtorStructName(ci.typeName, name) + "{}"
			}
			// Constructor with fields — return as a curried function
			return g.ctorAsClosure(ci)
		}
		// Check if it's a known top-level function — if so, and if arity > 0,
		// we need to wrap it as a closure when used as a value
		if fi, ok := g.funcs[name]; ok && fi.arity > 0 {
			if !g.locals[name] {
				return g.funcAsClosure(name, fi)
			}
		}
		return goVarName(name)
	}
	return "nil"
}

func (g *goGen) ctorAsClosure(ci *goCtorInfo) string {
	structName := goCtorStructName(ci.typeName, ci.name)
	n := len(ci.fieldTypes)
	if n == 1 {
		return fmt.Sprintf("func(a0 any) any { return %s{F0: a0} }", structName)
	}
	// Multi-field: curry
	var params []string
	for i := 0; i < n; i++ {
		params = append(params, fmt.Sprintf("a%d", i))
	}
	// Build struct creation
	var fields []string
	for i, p := range params {
		fields = append(fields, fmt.Sprintf("F%d: %s", i, p))
	}
	result := fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
	// Wrap from inside out
	for i := n - 1; i >= 0; i-- {
		result = fmt.Sprintf("func(%s any) any { return %s }", params[i], result)
	}
	return result
}

func (g *goGen) funcAsClosure(name string, fi *goFuncInfo) string {
	goName := goFuncName(name)
	if fi.arity == 1 {
		return fmt.Sprintf("func(a any) any { return %s(a) }", goName)
	}
	// Multi-arg: curry
	return g.buildCurriedClosure(goName, fi.arity)
}

func (g *goGen) buildCurriedClosure(goName string, arity int) string {
	if arity == 1 {
		return fmt.Sprintf("func(a any) any { return %s(a) }", goName)
	}
	// Build nested closures
	var params []string
	for i := 0; i < arity; i++ {
		params = append(params, fmt.Sprintf("a%d", i))
	}

	// Innermost call
	callArgs := strings.Join(params, ", ")
	result := fmt.Sprintf("%s(%s)", goName, callArgs)

	// Wrap from inside out
	for i := arity - 1; i >= 0; i-- {
		result = fmt.Sprintf("func(%s any) any { return %s }", params[i], result)
	}
	return result
}

// ---------------------------------------------------------------------------
// Typed atom accessors
// ---------------------------------------------------------------------------

func (g *goGen) emitAtomTyped(a ir.Atom, ty types.Type) {
	if isFloatType(ty) {
		g.emitAtomAsFloat(a)
	} else {
		g.emitAtomAsInt(a)
	}
}

func (g *goGen) emitAtomAsInt(a ir.Atom) {
	if _, ok := a.(ir.AInt); ok {
		g.emitAtom(a)
		return
	}
	g.buf.WriteString(g.atomStr(a))
	g.buf.WriteString(".(int64)")
}

func (g *goGen) emitAtomAsFloat(a ir.Atom) {
	if _, ok := a.(ir.AFloat); ok {
		g.emitAtom(a)
		return
	}
	g.buf.WriteString(g.atomStr(a))
	g.buf.WriteString(".(float64)")
}

func (g *goGen) emitAtomAsBool(a ir.Atom) {
	if _, ok := a.(ir.ABool); ok {
		g.emitAtom(a)
		return
	}
	g.buf.WriteString(g.atomStr(a))
	g.buf.WriteString(".(bool)")
}

func (g *goGen) emitAtomAsString(a ir.Atom) {
	if _, ok := a.(ir.AString); ok {
		g.emitAtom(a)
		return
	}
	g.buf.WriteString(g.atomStr(a))
	g.buf.WriteString(".(string)")
}

func (g *goGen) emitAtomAsList(a ir.Atom) {
	s := g.atomStr(a)
	// If it's already a *RexList literal, don't cast
	if strings.HasPrefix(s, "&RexList") || strings.HasPrefix(s, "(*RexList)(nil)") {
		g.buf.WriteString(s)
		return
	}
	fmt.Fprintf(g.buf, "func() *RexList { if v, ok := %s.(*RexList); ok { return v }; return nil }()", s)
}

func (g *goGen) atomToBool(a ir.Atom) string {
	if b, ok := a.(ir.ABool); ok {
		if b.Value {
			return "true"
		}
		return "false"
	}
	return fmt.Sprintf("%s.(bool)", g.atomStr(a))
}

// ---------------------------------------------------------------------------
// Go type helpers
// ---------------------------------------------------------------------------

func (g *goGen) goType(ty types.Type) string {
	if ty == nil {
		return "any"
	}
	tc, ok := ty.(types.TCon)
	if !ok {
		return "any" // type variable → any
	}
	switch tc.Name {
	case "Int":
		return "int64"
	case "Float":
		return "float64"
	case "String":
		return "string"
	case "Bool":
		return "bool"
	case "Unit":
		return "any"
	case "Fun":
		return "func(any) any"
	case "List":
		return "*RexList"
	case "Tuple":
		return fmt.Sprintf("Tuple%d", len(tc.Args))
	default:
		return "any"
	}
}

func isFloatType(ty types.Type) bool {
	if ty == nil {
		return false
	}
	tc, ok := ty.(types.TCon)
	return ok && tc.Name == "Float"
}

// ---------------------------------------------------------------------------
// Name mangling
// ---------------------------------------------------------------------------

func goFuncName(name string) string {
	if name == "main" {
		return "rex_main"
	}
	return "rex_" + goSanitize(name)
}

func goVarName(name string) string {
	// Avoid Go keywords
	switch name {
	case "type", "func", "var", "const", "map", "range", "select", "case", "default",
		"go", "chan", "defer", "interface", "struct", "switch", "break", "continue",
		"fallthrough", "return", "import", "package", "for", "if", "else":
		return "r_" + name
	}
	return goSanitize(name)
}

func goSanitize(name string) string {
	// Replace characters that aren't valid in Go identifiers
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_':
			b.WriteRune(c)
		case c == '.':
			b.WriteString("__")
		case c == ':':
			b.WriteString("_")
		default:
			fmt.Fprintf(&b, "_%d_", c)
		}
	}
	return b.String()
}

func goTypeName(name string) string {
	return "Rex_" + goSanitize(name)
}

func goCtorStructName(typeName, ctorName string) string {
	return "Rex_" + goSanitize(typeName) + "_" + goSanitize(ctorName)
}

func goExportedField(name string) string {
	if len(name) == 0 {
		return name
	}
	return strings.ToUpper(name[:1]) + name[1:]
}
