// Package codegen emits WebAssembly Text (WAT) from the IR.
//
// The compilation target is WasmGC via WASI. Programs export _start and
// use proc_exit for the exit code. Functions compile to wasm funcs with
// direct calls when possible. Closures use WasmGC structs (funcref + captures).
package codegen

import (
	"fmt"
	"strings"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// wasm value types
const (
	wtI32       = "i32"
	wtI64       = "i64"
	wtF64       = "f64"
	wtRef       = "(ref null $closure)" // closure reference (nullable for locals)
	wtAdtRef    = "(ref null $adt)"     // ADT value reference (nullable for locals)
	wtStringRef = "(ref null $string)" // string reference (nullable for locals)
	wtListRef   = "(ref null $list)"   // list reference (nullable for locals)
)

// EmitWAT converts an IR program to WAT text.
func EmitWAT(prog *ir.Program, typeEnv typechecker.TypeEnv) (string, error) {
	g := &watGen{
		buf:           &strings.Builder{},
		locals:        make(map[string]string),
		funcs:         make(map[string]*funcInfo),
		typeEnv:       typeEnv,
		funcRefs:      make(map[string]bool),
		funcWrappers:  make(map[string]string),
		adts:          make(map[string]*adtInfo),
		ctorToAdt:     make(map[string]*ctorInfo),
		stringDataIdx: make(map[string]int),
		usesTuples:    make(map[int]bool),
	}
	return g.emit(prog)
}

// adtInfo describes an ADT for codegen.
type adtInfo struct {
	name  string
	ctors []ctorInfo
}

type ctorInfo struct {
	name      string
	tag       int
	typeName  string   // parent ADT name
	fieldTypes []string // wasm types of constructor fields
}

// funcInfo describes a top-level function for codegen.
type funcInfo struct {
	name    string
	arity   int
	params  []paramInfo
	retType string
	body    ir.Expr // innermost body after unwrapping lambdas
}

type paramInfo struct {
	name     string
	wasmType string
}

// lambdaFunc is a lifted lambda that becomes a wasm function.
type lambdaFunc struct {
	name     string   // generated func name
	captures []string // names of captured variables
	capTypes []string // wasm types of captures
	param    string
	retType  string
	body     ir.Expr
}

// pendingCall tracks partial application chains for multi-arg call detection.
type pendingCall struct {
	funcName string
	args     []ir.Atom
}

type watGen struct {
	buf     *strings.Builder
	indent  int
	locals  map[string]string    // local name → wasm type
	funcs   map[string]*funcInfo // top-level function info
	typeEnv typechecker.TypeEnv
	lambdas []lambdaFunc // lifted lambdas
	pending map[string]*pendingCall
	maxCaps int // max captures needed (determines closure types)

	// ADT info
	adts      map[string]*adtInfo  // ADT name → info
	ctorToAdt map[string]*ctorInfo // constructor name → ctor info

	// functions referenced via ref.func (need elem declare)
	funcRefs map[string]bool

	// wrapper functions for top-level funcs used as values
	funcWrappers map[string]string // func name → wrapper name

	// string literal data segments
	stringData    []string       // ordered unique string values
	stringDataIdx map[string]int // string value → index in stringData

	// list/tuple usage tracking
	usesLists  bool
	usesTuples map[int]bool // tuple arity → true

	// current function context
	currentFunc *funcInfo
	funcParams  map[string]string // param name → wasm type for current function
}

func (g *watGen) line(format string, args ...any) {
	g.buf.WriteString(strings.Repeat("  ", g.indent))
	fmt.Fprintf(g.buf, format, args...)
	g.buf.WriteByte('\n')
}

// watStringLiteral converts a Go string to a WAT data segment string literal.
// WAT uses "..." with \xx hex escapes for non-printable bytes.
func watStringLiteral(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x20 && c < 0x7f && c != '"' && c != '\\' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "\\%02x", c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func wtTupleRef(arity int) string {
	return fmt.Sprintf("(ref null $tuple%d)", arity)
}

// internString adds a string literal to the data segment pool and returns its index.
func (g *watGen) internString(s string) int {
	if idx, ok := g.stringDataIdx[s]; ok {
		return idx
	}
	idx := len(g.stringData)
	g.stringData = append(g.stringData, s)
	g.stringDataIdx[s] = idx
	return idx
}

// ---------------------------------------------------------------------------
// Type helpers
// ---------------------------------------------------------------------------

// wasmType converts a Rex type to a wasm value type.
func (g *watGen) wasmType(t types.Type) string {
	switch ty := t.(type) {
	case types.TCon:
		switch ty.Name {
		case "Int":
			return wtI64
		case "Float":
			return wtF64
		case "Bool":
			return wtI32
		case "String":
			return wtStringRef
		case "List":
			return wtListRef
		case "Tuple":
			return wtTupleRef(len(ty.Args))
		case "Fun":
			return wtRef
		default:
			if _, ok := g.adts[ty.Name]; ok {
				return wtAdtRef
			}
		}
	}
	return wtI64 // default for type variables and unknown types
}

// decomposeFuncType breaks a function type into parameter types and return type.
func decomposeFuncType(t types.Type) ([]types.Type, types.Type) {
	var params []types.Type
	for {
		tc, ok := t.(types.TCon)
		if !ok || tc.Name != "Fun" || len(tc.Args) != 2 {
			break
		}
		params = append(params, tc.Args[0])
		t = tc.Args[1]
	}
	return params, t
}

// lookupType gets the Rex type for a top-level name from the type environment.
func (g *watGen) lookupType(name string) (types.Type, bool) {
	v, ok := g.typeEnv[name]
	if !ok {
		return nil, false
	}
	scheme, ok := v.(types.Scheme)
	if !ok {
		return nil, false
	}
	return scheme.Ty, true
}

// ---------------------------------------------------------------------------
// Phase 1: Analyze — collect function info and lambdas
// ---------------------------------------------------------------------------

func (g *watGen) analyze(prog *ir.Program) {
	// Pass 0: collect ADT info (before functions, since param types may be ADTs)
	for _, d := range prog.Decls {
		dt, ok := d.(ir.DType)
		if !ok || len(dt.Ctors) == 0 {
			continue
		}
		g.analyzeADT(dt)
	}

	// First pass: collect function info
	for _, d := range prog.Decls {
		dl, ok := d.(ir.DLet)
		if !ok {
			continue
		}
		fi := g.analyzeFunc(dl)
		if fi != nil {
			g.funcs[fi.name] = fi
			// Multi-arg functions need closure types for partial application
			if fi.arity > 1 && fi.arity-1 > g.maxCaps {
				g.maxCaps = fi.arity - 1
			}
		}
	}

	// Second pass: detect function names used as values
	for _, d := range prog.Decls {
		dl, ok := d.(ir.DLet)
		if !ok {
			continue
		}
		fi := g.funcs[dl.Name]
		if fi == nil {
			continue
		}
		g.scanFuncAsValue(fi.body)
	}

	// Third pass: collect string literals
	for _, d := range prog.Decls {
		dl, ok := d.(ir.DLet)
		if !ok {
			continue
		}
		fi := g.funcs[dl.Name]
		if fi == nil {
			continue
		}
		g.scanStrings(fi.body)
	}
}

func (g *watGen) scanStrings(expr ir.Expr) {
	switch e := expr.(type) {
	case ir.EAtom:
		g.scanAtomString(e.A)
	case ir.EComplex:
		g.scanCExprStrings(e.C)
	case ir.ELet:
		g.scanCExprStrings(e.Bind)
		g.scanStrings(e.Body)
	}
}

func (g *watGen) scanAtomString(a ir.Atom) {
	if s, ok := a.(ir.AString); ok {
		g.internString(s.Value)
	}
}

func (g *watGen) scanCExprStrings(c ir.CExpr) {
	switch e := c.(type) {
	case ir.CApp:
		g.scanAtomString(e.Func)
		g.scanAtomString(e.Arg)
	case ir.CBinop:
		g.scanAtomString(e.Left)
		g.scanAtomString(e.Right)
	case ir.CUnaryMinus:
		g.scanAtomString(e.Expr)
	case ir.CIf:
		g.scanAtomString(e.Cond)
		g.scanStrings(e.Then)
		g.scanStrings(e.Else)
	case ir.CMatch:
		g.scanAtomString(e.Scrutinee)
		for _, arm := range e.Arms {
			g.scanPatternStrings(arm.Pat)
			g.scanStrings(arm.Body)
		}
	case ir.CLambda:
		g.scanStrings(e.Body)
	case ir.CCtor:
		for _, arg := range e.Args {
			g.scanAtomString(arg)
		}
	case ir.CList:
		g.usesLists = true
		for _, item := range e.Items {
			g.scanAtomString(item)
		}
	case ir.CTuple:
		g.usesTuples[len(e.Items)] = true
		for _, item := range e.Items {
			g.scanAtomString(item)
		}
	}
}

func (g *watGen) scanPatternStrings(pat ir.Pattern) {
	switch p := pat.(type) {
	case ir.PString:
		g.internString(p.Value)
	case ir.PNil, ir.PCons:
		g.usesLists = true
	case ir.PTuple:
		g.usesTuples[len(p.Pats)] = true
	}
}

// emitStringEq emits a helper function that compares two $string arrays byte-by-byte.
func (g *watGen) emitStringEq() {
	g.line("(func $string_eq (param $a (ref null $string)) (param $b (ref null $string)) (result i32)")
	g.indent++
	g.line("(local $len i32)")
	g.line("(local $i i32)")
	// Check if lengths differ
	g.line("(local.set $len (array.len (local.get $a)))")
	g.line("(if (i32.ne (local.get $len) (array.len (local.get $b)))")
	g.indent++
	g.line("(then (return (i32.const 0)))")
	g.indent--
	g.line(")")
	// Compare byte-by-byte
	g.line("(local.set $i (i32.const 0))")
	g.line("(block $done")
	g.indent++
	g.line("(loop $loop")
	g.indent++
	g.line("(br_if $done (i32.ge_u (local.get $i) (local.get $len)))")
	g.line("(if (i32.ne (array.get_u $string (local.get $a) (local.get $i)) (array.get_u $string (local.get $b) (local.get $i)))")
	g.indent++
	g.line("(then (return (i32.const 0)))")
	g.indent--
	g.line(")")
	g.line("(local.set $i (i32.add (local.get $i) (i32.const 1)))")
	g.line("(br $loop)")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	g.line("(i32.const 1)")
	g.indent--
	g.line(")")
}

// scanFuncAsValue finds atoms that reference top-level functions in value position.
func (g *watGen) scanFuncAsValue(expr ir.Expr) {
	switch e := expr.(type) {
	case ir.EAtom:
		g.checkAtomFuncAsValue(e.A)
	case ir.EComplex:
		g.scanCExprFuncAsValue(e.C)
	case ir.ELet:
		g.scanCExprFuncAsValue(e.Bind)
		g.scanFuncAsValue(e.Body)
	case ir.ELetRec:
		for _, b := range e.Bindings {
			g.scanCExprFuncAsValue(b.Bind)
		}
		g.scanFuncAsValue(e.Body)
	}
}

func (g *watGen) scanCExprFuncAsValue(c ir.CExpr) {
	switch e := c.(type) {
	case ir.CApp:
		// The argument is in value position; the func position is not
		g.checkAtomFuncAsValue(e.Arg)
	case ir.CBinop:
		g.checkAtomFuncAsValue(e.Left)
		g.checkAtomFuncAsValue(e.Right)
	case ir.CUnaryMinus:
		g.checkAtomFuncAsValue(e.Expr)
	case ir.CIf:
		g.checkAtomFuncAsValue(e.Cond)
		g.scanFuncAsValue(e.Then)
		g.scanFuncAsValue(e.Else)
	case ir.CLambda:
		g.scanFuncAsValue(e.Body)
	case ir.CMatch:
		g.checkAtomFuncAsValue(e.Scrutinee)
		for _, arm := range e.Arms {
			g.scanFuncAsValue(arm.Body)
		}
	}
}

func (g *watGen) checkAtomFuncAsValue(a ir.Atom) {
	v, ok := a.(ir.AVar)
	if !ok {
		return
	}
	if fi, ok := g.funcs[v.Name]; ok && fi.arity == 1 {
		if _, exists := g.funcWrappers[v.Name]; !exists {
			g.funcWrappers[v.Name] = fmt.Sprintf("$%s__wrap", v.Name)
		}
	}
}

func (g *watGen) analyzeFunc(dl ir.DLet) *funcInfo {
	// Unwrap lambda chain to determine arity and params
	body := dl.Body
	var params []paramInfo

	for {
		ec, ok := body.(ir.EComplex)
		if !ok {
			break
		}
		lam, ok := ec.C.(ir.CLambda)
		if !ok {
			break
		}
		params = append(params, paramInfo{name: lam.Param})
		body = lam.Body
	}

	if len(params) == 0 {
		return nil // not a function, just a value binding
	}

	fi := &funcInfo{
		name:   dl.Name,
		arity:  len(params),
		params: params,
		body:   body,
	}

	// Use type env to determine param and return types
	if ty, ok := g.lookupType(dl.Name); ok {
		paramTypes, retType := decomposeFuncType(ty)
		for i := range fi.params {
			if i < len(paramTypes) {
				fi.params[i].wasmType = g.wasmType(paramTypes[i])
			} else {
				fi.params[i].wasmType = wtI64
			}
		}
		fi.retType = g.wasmType(retType)
	} else {
		// Default: all i64
		for i := range fi.params {
			fi.params[i].wasmType = wtI64
		}
		fi.retType = wtI64
	}

	return fi
}

func (g *watGen) analyzeADT(dt ir.DType) {
	ai := &adtInfo{name: dt.Name}
	for i, c := range dt.Ctors {
		ci := ctorInfo{
			name:     c.Name,
			tag:      i,
			typeName: dt.Name,
		}
		// Determine field types from type env
		if ty, ok := g.lookupType(c.Name); ok {
			paramTypes, _ := decomposeFuncType(ty)
			for _, pt := range paramTypes {
				ci.fieldTypes = append(ci.fieldTypes, g.wasmType(pt))
			}
		}
		ai.ctors = append(ai.ctors, ci)
		g.ctorToAdt[c.Name] = &ai.ctors[len(ai.ctors)-1]
	}
	g.adts[dt.Name] = ai
}

// ---------------------------------------------------------------------------
// Phase 2: Emit — generate WAT output
// ---------------------------------------------------------------------------

func (g *watGen) emit(prog *ir.Program) (string, error) {
	g.analyze(prog)

	// Find main
	mainFI, ok := g.funcs["main"]
	if !ok {
		return "", fmt.Errorf("codegen: no main function found")
	}
	// Override main return type to i64 (exit code)
	mainFI.retType = wtI64

	// Determine max captures from lambdas in all function bodies
	for _, fi := range g.funcs {
		g.scanForLambdas(fi.name, fi.body, fi.params)
	}

	g.line("(module")
	g.indent++

	// Emit GC type declarations (closures + ADTs in one rec group)
	if g.needsGCTypes() {
		g.emitGCTypes()
		g.line("")
	}

	// WASI import: proc_exit
	g.line("(import \"wasi_snapshot_preview1\" \"proc_exit\" (func $proc_exit (param i32)))")
	g.line("")

	// Memory (required by WASI)
	g.line("(memory (export \"memory\") 1)")
	g.line("")

	// Data segments for string literals
	for i, s := range g.stringData {
		g.line("(data $d%d %s)", i, watStringLiteral(s))
	}
	if len(g.stringData) > 0 {
		g.line("")
	}

	// Emit all top-level functions
	for _, d := range prog.Decls {
		dl, ok := d.(ir.DLet)
		if !ok {
			continue
		}
		fi, ok := g.funcs[dl.Name]
		if !ok {
			continue
		}
		if err := g.emitFunc(fi); err != nil {
			return "", err
		}
		g.line("")
	}

	// Emit lifted lambdas
	for _, lf := range g.lambdas {
		if err := g.emitLambdaFunc(&lf); err != nil {
			return "", err
		}
		g.line("")
	}

	// Emit function wrappers (for functions used as values)
	for funcName, wrapperName := range g.funcWrappers {
		fi := g.funcs[funcName]
		g.emitFuncWrapper(fi, wrapperName)
		g.line("")
	}

	// String equality helper
	if len(g.stringData) > 0 {
		g.emitStringEq()
		g.line("")
	}

	// _start function — pass dummy arg (0) for main's ignored parameter
	g.line("(func (export \"_start\")")
	g.indent++
	g.line("(call $proc_exit (i32.and (i32.wrap_i64 (call $main (i64.const 0))) (i32.const 255)))")
	g.indent--
	g.line(")")

	// Declare function references (required for ref.func)
	if len(g.funcRefs) > 0 {
		g.line("")
		refs := make([]string, 0, len(g.funcRefs))
		for r := range g.funcRefs {
			refs = append(refs, r)
		}
		sortStrings(refs)
		g.line("(elem declare func %s)", strings.Join(refs, " "))
	}

	g.indent--
	g.line(")")

	return g.buf.String(), nil
}

func (g *watGen) needsClosures() bool {
	return len(g.lambdas) > 0 || g.maxCaps > 0 || len(g.funcWrappers) > 0
}

func (g *watGen) needsGCTypes() bool {
	return g.needsClosures() || len(g.adts) > 0 || len(g.stringData) > 0 || g.usesLists || len(g.usesTuples) > 0
}

// ---------------------------------------------------------------------------
// GC type declarations (closures + ADTs)
// ---------------------------------------------------------------------------

func (g *watGen) emitGCTypes() {
	// All GC types go in one (rec ...) group for mutual references
	g.line(";; GC types")
	g.line("(rec")
	g.indent++

	// String type
	if len(g.stringData) > 0 {
		g.line("(type $string (array (mut i8)))")
	}

	// Closure types
	if g.needsClosures() {
		g.line(";; Closure types")
		g.line("(type $ft_apply (func (param (ref null $closure)) (param i64) (result i64)))")
		g.line("(type $closure (sub (struct (field $fn (ref $ft_apply)))))")
		for n := 1; n <= g.maxCaps; n++ {
			fields := "(field $fn (ref $ft_apply))"
			for i := 0; i < n; i++ {
				fields += fmt.Sprintf(" (field $c%d i64)", i)
			}
			g.line("(type $closure_%d (sub $closure (struct %s)))", n, fields)
		}
	}

	// ADT types
	if len(g.adts) > 0 {
		g.line(";; ADT types")
		g.line("(type $adt (sub (struct (field $tag i32))))")

		// Sort ADT names for deterministic output
		adtNames := make([]string, 0, len(g.adts))
		for name := range g.adts {
			adtNames = append(adtNames, name)
		}
		sortStrings(adtNames)

		for _, adtName := range adtNames {
			ai := g.adts[adtName]
			for _, ci := range ai.ctors {
				if len(ci.fieldTypes) == 0 {
					// Zero-arg constructor: just the base $adt type with tag is fine,
					// but define a named subtype for clarity
					g.line("(type $%s_%s (sub $adt (struct (field $tag i32))))",
						adtName, ci.name)
				} else {
					// Constructor with fields
					fields := "(field $tag i32)"
					for i, ft := range ci.fieldTypes {
						fields += fmt.Sprintf(" (field $f%d %s)", i, ft)
					}
					g.line("(type $%s_%s (sub $adt (struct %s)))",
						adtName, ci.name, fields)
				}
			}
		}
	}

	// List types
	if g.usesLists {
		g.line(";; List types")
		g.line("(type $list (sub (struct (field $tag i32))))")
		g.line("(type $list_cons (sub $list (struct (field $tag i32) (field $head i64) (field $tail (ref null $list)))))")
	}

	// Tuple types
	if len(g.usesTuples) > 0 {
		g.line(";; Tuple types")
		arities := make([]int, 0, len(g.usesTuples))
		for a := range g.usesTuples {
			arities = append(arities, a)
		}
		sortInts(arities)
		for _, arity := range arities {
			fields := ""
			for i := 0; i < arity; i++ {
				if fields != "" {
					fields += " "
				}
				fields += fmt.Sprintf("(field $f%d i64)", i)
			}
			g.line("(type $tuple%d (struct %s))", arity, fields)
		}
	}

	g.indent--
	g.line(")")
}

// ---------------------------------------------------------------------------
// Scan for lambdas with captures
// ---------------------------------------------------------------------------

func (g *watGen) scanForLambdas(ownerFunc string, body ir.Expr, params []paramInfo) {
	paramSet := make(map[string]bool)
	for _, p := range params {
		paramSet[p.name] = true
	}
	g.scanExprForLambdas(ownerFunc, body, paramSet, nil)
}

func (g *watGen) scanExprForLambdas(owner string, expr ir.Expr, scope map[string]bool, letBindings map[string]bool) {
	switch e := expr.(type) {
	case ir.EComplex:
		g.scanCExprForLambdas(owner, e.C, scope, letBindings)
	case ir.ELet:
		g.scanCExprForLambdas(owner, e.Bind, scope, letBindings)
		// Add let-bound name to scope
		newScope := copyScope(scope)
		newScope[e.Name] = true
		newLet := copyScope(letBindings)
		newLet[e.Name] = true
		g.scanExprForLambdas(owner, e.Body, newScope, newLet)
	}
}

func (g *watGen) scanCExprForLambdas(owner string, c ir.CExpr, scope map[string]bool, letBindings map[string]bool) {
	switch e := c.(type) {
	case ir.CLambda:
		// This is a lambda that appears as a value (not at the top of a function def)
		// Find captures: free variables that come from the enclosing scope
		captures := g.findCaptures(e, scope, letBindings)
		if len(captures) > g.maxCaps {
			g.maxCaps = len(captures)
		}
		// Also need at least 1 for partial application
		lambdaName := fmt.Sprintf("$%s__lambda_%d", owner, len(g.lambdas))
		capTypes := make([]string, len(captures))
		for i := range captures {
			capTypes[i] = wtI64 // default
		}
		g.lambdas = append(g.lambdas, lambdaFunc{
			name:     lambdaName,
			captures: captures,
			capTypes: capTypes,
			param:    e.Param,
			retType:  wtI64,
			body:     e.Body,
		})
	case ir.CIf:
		g.scanExprForLambdas(owner, e.Then, scope, letBindings)
		g.scanExprForLambdas(owner, e.Else, scope, letBindings)
	case ir.CMatch:
		for _, arm := range e.Arms {
			g.scanExprForLambdas(owner, arm.Body, scope, letBindings)
		}
	}
}

func (g *watGen) findCaptures(lam ir.CLambda, outerScope map[string]bool, letBindings map[string]bool) []string {
	free := make(map[string]bool)
	g.collectFreeVars(lam.Body, map[string]bool{lam.Param: true}, free)

	var captures []string
	for name := range free {
		// Only capture variables from let bindings and function params
		// (not top-level functions — those are called directly)
		if _, isFunc := g.funcs[name]; isFunc {
			continue
		}
		if outerScope[name] {
			captures = append(captures, name)
		}
	}
	// Sort for deterministic output
	sortStrings(captures)
	return captures
}

func (g *watGen) collectFreeVars(expr ir.Expr, bound map[string]bool, free map[string]bool) {
	switch e := expr.(type) {
	case ir.EAtom:
		g.collectFreeVarsAtom(e.A, bound, free)
	case ir.EComplex:
		g.collectFreeVarsCExpr(e.C, bound, free)
	case ir.ELet:
		g.collectFreeVarsCExpr(e.Bind, bound, free)
		newBound := copyScope(bound)
		newBound[e.Name] = true
		g.collectFreeVars(e.Body, newBound, free)
	case ir.ELetRec:
		newBound := copyScope(bound)
		for _, b := range e.Bindings {
			newBound[b.Name] = true
		}
		for _, b := range e.Bindings {
			g.collectFreeVarsCExpr(b.Bind, newBound, free)
		}
		g.collectFreeVars(e.Body, newBound, free)
	}
}

func (g *watGen) collectFreeVarsAtom(a ir.Atom, bound map[string]bool, free map[string]bool) {
	if v, ok := a.(ir.AVar); ok && !bound[v.Name] {
		free[v.Name] = true
	}
}

func (g *watGen) collectFreeVarsCExpr(c ir.CExpr, bound map[string]bool, free map[string]bool) {
	switch e := c.(type) {
	case ir.CApp:
		g.collectFreeVarsAtom(e.Func, bound, free)
		g.collectFreeVarsAtom(e.Arg, bound, free)
	case ir.CBinop:
		g.collectFreeVarsAtom(e.Left, bound, free)
		g.collectFreeVarsAtom(e.Right, bound, free)
	case ir.CUnaryMinus:
		g.collectFreeVarsAtom(e.Expr, bound, free)
	case ir.CIf:
		g.collectFreeVarsAtom(e.Cond, bound, free)
		g.collectFreeVars(e.Then, bound, free)
		g.collectFreeVars(e.Else, bound, free)
	case ir.CMatch:
		g.collectFreeVarsAtom(e.Scrutinee, bound, free)
		for _, arm := range e.Arms {
			g.collectFreeVars(arm.Body, bound, free)
		}
	case ir.CLambda:
		newBound := copyScope(bound)
		newBound[e.Param] = true
		g.collectFreeVars(e.Body, newBound, free)
	}
}

// ---------------------------------------------------------------------------
// Emit functions
// ---------------------------------------------------------------------------

func (g *watGen) emitFunc(fi *funcInfo) error {
	g.currentFunc = fi
	g.funcParams = make(map[string]string)
	g.locals = make(map[string]string)

	// Build param signature
	var paramSig string
	for _, p := range fi.params {
		paramSig += fmt.Sprintf(" (param $%s %s)", p.name, p.wasmType)
		g.funcParams[p.name] = p.wasmType
	}

	// Collect locals
	g.collectLocals(fi.body)

	g.line("(func $%s%s (result %s)", fi.name, paramSig, fi.retType)
	g.indent++

	// Declare locals
	for _, name := range g.localOrder(fi.body) {
		g.line("(local $%s %s)", name, g.locals[name])
	}

	// Emit body
	// Function body is in tail position
	if err := g.emitExprTail(fi.body, true); err != nil {
		return fmt.Errorf("codegen %s: %w", fi.name, err)
	}

	// Convert result type if needed
	bodyType := g.typeOfExpr(fi.body)
	if bodyType != fi.retType {
		g.emitConvert(bodyType, fi.retType)
	}

	g.indent--
	g.line(")")
	g.currentFunc = nil
	return nil
}

func (g *watGen) emitLambdaFunc(lf *lambdaFunc) error {
	g.currentFunc = nil
	g.funcParams = make(map[string]string)
	g.locals = make(map[string]string)

	// Lambda function signature: (self: ref $closure, param: i64) -> i64
	closureType := "$closure"
	if len(lf.captures) > 0 {
		closureType = fmt.Sprintf("$closure_%d", len(lf.captures))
	}

	// Register captures and param as available locals
	for i, cap := range lf.captures {
		g.locals[cap] = lf.capTypes[i]
	}
	g.funcParams[lf.param] = wtI64

	// Collect locals from body
	g.collectLocals(lf.body)

	g.line("(func %s (type $ft_apply) (param $self (ref null $closure)) (param $%s i64) (result i64)", lf.name, lf.param)
	g.indent++

	// Extract captures from closure struct
	for i, cap := range lf.captures {
		g.line("(local $%s i64)", cap)
		g.line("(local.set $%s (struct.get %s $c%d (ref.cast (ref %s) (local.get $self))))",
			cap, closureType, i, closureType)
	}

	// Declare other locals
	for _, name := range g.localOrder(lf.body) {
		if !containsStr(lf.captures, name) {
			g.line("(local $%s %s)", name, g.locals[name])
		}
	}

	if err := g.emitExpr(lf.body); err != nil {
		return fmt.Errorf("codegen lambda %s: %w", lf.name, err)
	}

	// Convert to i64 if needed
	bodyType := g.typeOfExpr(lf.body)
	if bodyType != wtI64 {
		g.emitConvert(bodyType, wtI64)
	}

	g.indent--
	g.line(")")
	return nil
}

// ---------------------------------------------------------------------------
// Partial application wrappers
// ---------------------------------------------------------------------------

func (g *watGen) emitPartialApplyWrappers(fi *funcInfo) {
	// For a function with arity N, generate N-1 wrapper functions.
	// Wrapper k takes a closure with k captured args + 1 new arg,
	// and either calls the next wrapper or the direct function.

	// Ensure we have enough closure types
	if fi.arity-1 > g.maxCaps {
		g.maxCaps = fi.arity - 1
	}

	// Generate one wrapper per partial application level.
	// $f__partial_k: takes a closure with k captures + 1 arg
	// k = arity-1: has all args → call $f directly
	// k < arity-1: creates a new closure with k+1 captures
	for k := 0; k < fi.arity; k++ {
		nCaps := k // number of captured args so far
		closureType := "$closure"
		if nCaps > 0 {
			closureType = fmt.Sprintf("$closure_%d", nCaps)
		}

		g.line("(func $%s__partial_%d (type $ft_apply) (param $self (ref null $closure)) (param $arg i64) (result i64)",
			fi.name, k)
		g.indent++

		if k == fi.arity-1 {
			// Saturated: extract all captured args + this arg, call $f directly
			for i := 0; i < nCaps; i++ {
				g.line("(struct.get %s $c%d (ref.cast (ref %s) (local.get $self)))",
					closureType, i, closureType)
			}
			g.line("(local.get $arg)")
			g.line("(call $%s)", fi.name)
		} else {
			// Not saturated: create a new closure with one more capture
			newCaps := nCaps + 1
			newClosureType := fmt.Sprintf("$closure_%d", newCaps)
			if newCaps > g.maxCaps {
				g.maxCaps = newCaps
			}

			// The new closure's funcref is the next wrapper
			g.line("(struct.new %s", newClosureType)
			g.indent++
			nextWrapper := fmt.Sprintf("$%s__partial_%d", fi.name, k+1)
			g.funcRefs[nextWrapper] = true
			g.line("(ref.func %s)", nextWrapper)
			// Copy existing captures
			for i := 0; i < nCaps; i++ {
				g.line("(struct.get %s $c%d (ref.cast (ref %s) (local.get $self)))",
					closureType, i, closureType)
			}
			// Add new arg as capture
			g.line("(local.get $arg)")
			g.indent--
			g.line(")")

			// The result is a closure ref, but our return type is i64.
			// We need to pass it as-is. But our uniform closure calling convention
			// uses i64 everywhere...
			// Actually, closures are ref types, not i64. We need a different approach.
			// For now, wrap the closure ref in a global table or use anyref boxing.
			// TODO: This needs a ref return type, not i64.
		}

		g.indent--
		g.line(")")
	}
}

// ---------------------------------------------------------------------------
// Type conversion helpers
// ---------------------------------------------------------------------------

func (g *watGen) emitConvert(from, to string) {
	if from == to {
		return
	}
	switch {
	case from == wtI32 && to == wtI64:
		g.line("i64.extend_i32_u")
	case from == wtF64 && to == wtI64:
		g.line("i64.trunc_f64_s")
	case from == wtI64 && to == wtI32:
		g.line("i32.wrap_i64")
	}
}

// ---------------------------------------------------------------------------
// Local collection — pre-pass to find all let bindings and their types
// ---------------------------------------------------------------------------

func (g *watGen) collectLocals(expr ir.Expr) {
	g.collectLocalsWithCtx(expr, nil)
}

// partialCtors tracks temp variables that hold partially-applied constructors.
func (g *watGen) collectLocalsWithCtx(expr ir.Expr, partialCtors map[string]*ctorInfo) {
	if partialCtors == nil {
		partialCtors = make(map[string]*ctorInfo)
	}
	switch e := expr.(type) {
	case ir.ELet:
		localType := g.typeOfCExpr(e.Bind)
		// Detect constructor application chains
		if app, ok := e.Bind.(ir.CApp); ok {
			if fvar, ok := app.Func.(ir.AVar); ok {
				// If applying a constructor with arity > 1, the temp holds a partial app
				if ci, ok := g.ctorToAdt[fvar.Name]; ok && len(ci.fieldTypes) > 1 {
					partialCtors[e.Name] = ci
					localType = wtAdtRef // partial app temp, but saturated ctor will make it an adt ref
				}
				// If applying a variable that was a partial ctor app
				if ci, ok := partialCtors[fvar.Name]; ok {
					partialCtors[e.Name] = ci
					localType = wtAdtRef
				}
			}
		}
		g.locals[e.Name] = localType
		g.collectLocalsCExpr(e.Bind)
		g.collectLocalsWithCtx(e.Body, partialCtors)
	case ir.EComplex:
		g.collectLocalsCExpr(e.C)
	}
}

func (g *watGen) collectLocalsCExpr(c ir.CExpr) {
	switch e := c.(type) {
	case ir.CIf:
		g.collectLocals(e.Then)
		g.collectLocals(e.Else)
	case ir.CMatch:
		for _, arm := range e.Arms {
			g.collectPatternLocals(arm.Pat)
			g.collectLocals(arm.Body)
		}
	}
}

func (g *watGen) collectPatternLocals(pat ir.Pattern) {
	switch p := pat.(type) {
	case ir.PVar:
		if _, ok := g.locals[p.Name]; !ok {
			// Default to i64 for pattern variables; override for ADT fields
			g.locals[p.Name] = wtI64
		}
	case ir.PCtor:
		ci, ok := g.ctorToAdt[p.Name]
		if ok {
			for i, sub := range p.Args {
				if pv, ok := sub.(ir.PVar); ok {
					ft := wtI64
					if i < len(ci.fieldTypes) {
						ft = ci.fieldTypes[i]
					}
					g.locals[pv.Name] = ft
				}
				g.collectPatternLocals(sub)
			}
		} else {
			for _, sub := range p.Args {
				g.collectPatternLocals(sub)
			}
		}
	case ir.PCons:
		// head is i64 (for List Int), tail is list ref
		if pv, ok := p.Head.(ir.PVar); ok {
			g.locals[pv.Name] = wtI64
		}
		if pv, ok := p.Tail.(ir.PVar); ok {
			g.locals[pv.Name] = wtListRef
		}
		g.collectPatternLocals(p.Head)
		g.collectPatternLocals(p.Tail)
	case ir.PTuple:
		// All tuple fields are i64 for now
		for _, sub := range p.Pats {
			if pv, ok := sub.(ir.PVar); ok {
				g.locals[pv.Name] = wtI64
			}
			g.collectPatternLocals(sub)
		}
	}
}

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
	case ir.CMatch:
		for _, arm := range e.Arms {
			g.collectPatternLocalOrder(arm.Pat, names, seen)
			g.collectLocalOrder(arm.Body, names, seen)
		}
	}
}

func (g *watGen) collectPatternLocalOrder(pat ir.Pattern, names *[]string, seen map[string]bool) {
	switch p := pat.(type) {
	case ir.PVar:
		if !seen[p.Name] {
			seen[p.Name] = true
			*names = append(*names, p.Name)
		}
	case ir.PCtor:
		for _, sub := range p.Args {
			g.collectPatternLocalOrder(sub, names, seen)
		}
	case ir.PCons:
		g.collectPatternLocalOrder(p.Head, names, seen)
		g.collectPatternLocalOrder(p.Tail, names, seen)
	case ir.PTuple:
		for _, sub := range p.Pats {
			g.collectPatternLocalOrder(sub, names, seen)
		}
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
	case ir.AString:
		return wtStringRef
	case ir.AVar:
		// Zero-arg constructor
		if ci, ok := g.ctorToAdt[v.Name]; ok && len(ci.fieldTypes) == 0 {
			return wtAdtRef
		}
		if t, ok := g.funcParams[v.Name]; ok {
			return t
		}
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
	case ir.CApp:
		// Check if calling a constructor
		if v, ok := e.Func.(ir.AVar); ok {
			if _, ok := g.ctorToAdt[v.Name]; ok {
				return wtAdtRef
			}
			if fi, ok := g.funcs[v.Name]; ok {
				if fi.arity == 1 {
					return fi.retType
				}
				// Partial application returns a closure
				return wtRef
			}
		}
		return wtI64
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
	case ir.CMatch:
		if len(e.Arms) > 0 {
			return g.typeOfExpr(e.Arms[0].Body)
		}
	case ir.CLambda:
		return wtRef
	case ir.CList:
		return wtListRef
	case ir.CTuple:
		return wtTupleRef(len(e.Items))
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
// Emit expressions
// ---------------------------------------------------------------------------

func (g *watGen) emitExpr(expr ir.Expr) error {
	return g.emitExprTail(expr, false)
}

func (g *watGen) emitExprTail(expr ir.Expr, tail bool) error {
	switch e := expr.(type) {
	case ir.EAtom:
		return g.emitAtom(e.A)
	case ir.EComplex:
		return g.emitCExprTail(e.C, tail)
	case ir.ELet:
		return g.emitLetTail(e, tail)
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
		// Check if this is a zero-arg constructor
		if ci, ok := g.ctorToAdt[v.Name]; ok && len(ci.fieldTypes) == 0 {
			_, isParam := g.funcParams[v.Name]
			_, isLocal := g.locals[v.Name]
			if !isParam && !isLocal {
				g.line("(struct.new $%s_%s (i32.const %d))", ci.typeName, ci.name, ci.tag)
				return nil
			}
		}
		// Check if this is a top-level function used as a value
		if fi, ok := g.funcs[v.Name]; ok {
			// Only wrap as closure if it's not a param or local (shadowing)
			_, isParam := g.funcParams[v.Name]
			_, isLocal := g.locals[v.Name]
			if !isParam && !isLocal {
				return g.emitFuncAsValue(fi)
			}
		}
		g.line("local.get $%s", v.Name)
		return nil
	case ir.ABool:
		if v.Value {
			g.line("i32.const 1")
		} else {
			g.line("i32.const 0")
		}
		return nil
	case ir.AString:
		idx := g.internString(v.Value)
		g.line("(array.new_data $string $d%d (i32.const 0) (i32.const %d))", idx, len(v.Value))
		return nil
	default:
		return fmt.Errorf("unsupported atom: %T", a)
	}
}

// emitFuncAsValue wraps a top-level function in a closure struct so it can
// be passed as a value (e.g., as an argument to a higher-order function).
func (g *watGen) emitFuncAsValue(fi *funcInfo) error {
	if fi.arity > 1 {
		return fmt.Errorf("codegen: function %s (arity %d) used as value — partial application not yet supported", fi.name, fi.arity)
	}

	wrapperName, exists := g.funcWrappers[fi.name]
	if !exists {
		wrapperName = fmt.Sprintf("$%s__wrap", fi.name)
		g.funcWrappers[fi.name] = wrapperName
	}

	g.funcRefs[wrapperName] = true
	g.line("(struct.new $closure (ref.func %s))", wrapperName)
	return nil
}

func (g *watGen) emitCExpr(c ir.CExpr) error {
	return g.emitCExprTail(c, false)
}

func (g *watGen) emitCExprTail(c ir.CExpr, tail bool) error {
	switch e := c.(type) {
	case ir.CApp:
		return g.emitAppTail(e, tail)
	case ir.CBinop:
		return g.emitBinop(e)
	case ir.CUnaryMinus:
		return g.emitUnaryMinus(e)
	case ir.CIf:
		return g.emitIfTail(e, tail)
	case ir.CLambda:
		return g.emitClosureCreate(e)
	case ir.CMatch:
		return g.emitMatchTail(e, tail)
	case ir.CList:
		return g.emitList(e)
	case ir.CTuple:
		return g.emitTuple(e)
	default:
		return fmt.Errorf("unsupported cexpr: %T", c)
	}
}

// ---------------------------------------------------------------------------
// Function calls
// ---------------------------------------------------------------------------

func (g *watGen) emitApp(e ir.CApp) error {
	return g.emitAppTail(e, false)
}

func (g *watGen) emitAppTail(e ir.CApp, tail bool) error {
	v, isVar := e.Func.(ir.AVar)
	if !isVar {
		return fmt.Errorf("codegen: non-variable function application not supported")
	}

	// Constructor application (single-arg constructors)
	if ci, ok := g.ctorToAdt[v.Name]; ok {
		if len(ci.fieldTypes) == 1 {
			// Single-arg constructor: struct.new with tag + field
			g.line("(struct.new $%s_%s", ci.typeName, ci.name)
			g.indent++
			g.line("(i32.const %d)", ci.tag)
			if err := g.emitAtom(e.Arg); err != nil {
				return err
			}
			g.indent--
			g.line(")")
			return nil
		}
		// Multi-arg constructor: handled via saturated call detection in emitLet
		return fmt.Errorf("codegen: multi-arg constructor %s applied with single arg — use saturated application", ci.name)
	}

	// Direct call to known function
	if fi, ok := g.funcs[v.Name]; ok {
		if fi.arity == 1 {
			// Saturated call: emit arg, call
			if err := g.emitAtom(e.Arg); err != nil {
				return err
			}
			if tail {
				g.line("return_call $%s", fi.name)
			} else {
				g.line("call $%s", fi.name)
			}
			return nil
		}
		// Partial application of multi-arg function — the saturated call
		// detector handles the common case. If we get here, it means
		// this single CApp wasn't part of a detected chain.
		return fmt.Errorf("codegen: partial application of %s (arity %d) not yet supported; provide all arguments", fi.name, fi.arity)
	}

	// Call through closure reference (the variable holds a ref $closure)
	if err := g.emitCallRef(v.Name, e.Arg); err != nil {
		return err
	}
	return nil
}

func (g *watGen) emitCallRef(closureVar string, arg ir.Atom) error {
	// call_ref $ft_apply: stack = [self, arg, funcref]
	g.line("(local.get $%s)", closureVar) // self
	if err := g.emitAtom(arg); err != nil {
		return err
	}
	g.line("(struct.get $closure $fn (local.get $%s))", closureVar) // funcref
	g.line("(call_ref $ft_apply)")
	return nil
}

// ---------------------------------------------------------------------------
// Let bindings with saturated call detection
// ---------------------------------------------------------------------------

func (g *watGen) emitLet(e ir.ELet) error {
	return g.emitLetTail(e, false)
}

func (g *watGen) emitLetTail(e ir.ELet, tail bool) error {
	// Detect saturated multi-arg calls:
	// let _t = f arg1 in _t arg2  (where f has arity 2)
	if app, ok := e.Bind.(ir.CApp); ok {
		if fvar, ok := app.Func.(ir.AVar); ok {
			if fi, ok := g.funcs[fvar.Name]; ok && fi.arity > 1 {
				// Check if the body immediately uses _t in another application
				if result, ok := g.trySaturatedCallTail(fi, []ir.Atom{app.Arg}, e.Name, e.Body, tail); ok {
					return result
				}
			}
			// Detect saturated multi-arg constructor applications
			if ci, ok := g.ctorToAdt[fvar.Name]; ok && len(ci.fieldTypes) > 1 {
				if result, ok := g.trySaturatedCtor(ci, []ir.Atom{app.Arg}, e.Name, e.Body); ok {
					return result
				}
			}
		}
	}

	// Normal let: emit bind, set local, emit body
	if err := g.emitCExpr(e.Bind); err != nil {
		return err
	}
	g.line("local.set $%s", e.Name)
	return g.emitExprTail(e.Body, tail)
}

// trySaturatedCallTail detects chains of let-bound partial applications that
// resolve to a saturated direct call. Uses return_call in tail position.
func (g *watGen) trySaturatedCallTail(fi *funcInfo, args []ir.Atom, tempName string, body ir.Expr, tail bool) (error, bool) {
	if len(args) == fi.arity {
		// Saturated! Emit direct call
		for _, arg := range args {
			if err := g.emitAtom(arg); err != nil {
				return err, true
			}
		}
		if tail {
			g.line("return_call $%s", fi.name)
		} else {
			g.line("call $%s", fi.name)
		}
		return nil, true
	}

	// Check if body is: EComplex{CApp{tempName, nextArg}}
	if ec, ok := body.(ir.EComplex); ok {
		if app, ok := ec.C.(ir.CApp); ok {
			if v, ok := app.Func.(ir.AVar); ok && v.Name == tempName {
				return g.trySaturatedCallTail(fi, append(args, app.Arg), tempName, nil, tail)
			}
		}
	}

	// Check if body is: ELet{newTemp, CApp{tempName, nextArg}, restBody}
	if let, ok := body.(ir.ELet); ok {
		if app, ok := let.Bind.(ir.CApp); ok {
			if v, ok := app.Func.(ir.AVar); ok && v.Name == tempName {
				newArgs := append(args, app.Arg)
				if len(newArgs) == fi.arity {
					// Saturated! Emit call, then set temp and continue
					for _, arg := range newArgs {
						if err := g.emitAtom(arg); err != nil {
							return err, true
						}
					}
					g.line("call $%s", fi.name)
					// The call result replaces the let binding
					g.line("local.set $%s", let.Name)
					err := g.emitExprTail(let.Body, tail)
					return err, true
				}
				// Continue chaining
				return g.trySaturatedCallTail(fi, newArgs, let.Name, let.Body, tail)
			}
		}
		// Unrelated let binding — emit it and continue looking for the
		// application chain in the body.
		if !g.letBindUsesVar(let.Bind, tempName) {
			if err := g.emitCExpr(let.Bind); err != nil {
				return err, true
			}
			g.line("local.set $%s", let.Name)
			return g.trySaturatedCallTail(fi, args, tempName, let.Body, tail)
		}
	}

	return nil, false
}

// letBindUsesVar checks if a CExpr references a variable name.
func (g *watGen) letBindUsesVar(c ir.CExpr, name string) bool {
	switch e := c.(type) {
	case ir.CApp:
		return g.atomIsVar(e.Func, name) || g.atomIsVar(e.Arg, name)
	case ir.CBinop:
		return g.atomIsVar(e.Left, name) || g.atomIsVar(e.Right, name)
	case ir.CUnaryMinus:
		return g.atomIsVar(e.Expr, name)
	}
	return false
}

func (g *watGen) atomIsVar(a ir.Atom, name string) bool {
	if v, ok := a.(ir.AVar); ok {
		return v.Name == name
	}
	return false
}

// trySaturatedCtor detects chains of let-bound partial constructor applications.
func (g *watGen) trySaturatedCtor(ci *ctorInfo, args []ir.Atom, tempName string, body ir.Expr) (error, bool) {
	nFields := len(ci.fieldTypes)
	if len(args) == nFields {
		// Saturated! Emit struct.new
		g.line("(struct.new $%s_%s", ci.typeName, ci.name)
		g.indent++
		g.line("(i32.const %d)", ci.tag)
		for _, arg := range args {
			if err := g.emitAtom(arg); err != nil {
				return err, true
			}
		}
		g.indent--
		g.line(")")
		return nil, true
	}

	// Check if body is: EComplex{CApp{tempName, nextArg}}
	if ec, ok := body.(ir.EComplex); ok {
		if app, ok := ec.C.(ir.CApp); ok {
			if v, ok := app.Func.(ir.AVar); ok && v.Name == tempName {
				return g.trySaturatedCtor(ci, append(args, app.Arg), tempName, nil)
			}
		}
	}

	// Check if body is: ELet{newTemp, CApp{tempName, nextArg}, restBody}
	if let, ok := body.(ir.ELet); ok {
		if app, ok := let.Bind.(ir.CApp); ok {
			if v, ok := app.Func.(ir.AVar); ok && v.Name == tempName {
				newArgs := append(args, app.Arg)
				if len(newArgs) == nFields {
					// Saturated! Emit struct.new, then set local and continue
					g.line("(struct.new $%s_%s", ci.typeName, ci.name)
					g.indent++
					g.line("(i32.const %d)", ci.tag)
					for _, arg := range newArgs {
						if err := g.emitAtom(arg); err != nil {
							return err, true
						}
					}
					g.indent--
					g.line(")")
					// Override local type — collectLocals may have guessed wrong
					g.locals[let.Name] = wtAdtRef
					g.line("local.set $%s", let.Name)
					err := g.emitExpr(let.Body)
					return err, true
				}
				return g.trySaturatedCtor(ci, newArgs, let.Name, let.Body)
			}
		}
	}

	return nil, false
}

// ---------------------------------------------------------------------------
// Pattern matching
// ---------------------------------------------------------------------------

func (g *watGen) emitMatch(e ir.CMatch) error {
	return g.emitMatchTail(e, false)
}

func (g *watGen) emitMatchTail(e ir.CMatch, tail bool) error {
	resultType := g.typeOfCExpr(e)

	// For now, handle constructor patterns and literal patterns
	// using a chain of if/else blocks based on the tag field

	// Get the tag of the scrutinee
	scrut := e.Scrutinee
	scrutName := ""
	if v, ok := scrut.(ir.AVar); ok {
		scrutName = v.Name
	}

	// Determine the number of arms
	numArms := len(e.Arms)
	if numArms == 0 {
		return fmt.Errorf("codegen: empty match")
	}

	// Emit a block-based pattern match:
	// Get tag, then use nested if/else
	for i, arm := range e.Arms {
		isLast := i == numArms-1
		switch pat := arm.Pat.(type) {
		case ir.PCtor:
			ci, ok := g.ctorToAdt[pat.Name]
			if !ok {
				return fmt.Errorf("codegen: unknown constructor %s in pattern", pat.Name)
			}

			if !isLast {
				// Emit: if (tag == ci.tag) then ... else ...
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("(struct.get $adt $tag (ref.cast (ref $adt)))")
				g.line("(i32.const %d)", ci.tag)
				g.line("i32.eq")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++

				// Bind pattern variables by extracting fields
				if err := g.emitPatternBindings(scrutName, ci, pat.Args); err != nil {
					return err
				}
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}

				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				// Last arm: no condition check (catch-all or final branch)
				if err := g.emitPatternBindings(scrutName, ci, pat.Args); err != nil {
					return err
				}
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
			}

		case ir.PWild, ir.PVar:
			// Wildcard or variable pattern — always matches
			if pv, ok := pat.(ir.PVar); ok && scrutName != "" {
				// Bind the variable to the scrutinee value
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("local.set $%s", pv.Name)
			}
			if err := g.emitExprTail(arm.Body, tail); err != nil {
				return err
			}

		case ir.PInt:
			if !isLast {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("i64.const %d", pat.Value)
				g.line("i64.eq")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
			}

		case ir.PBool:
			if !isLast {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				if pat.Value {
					g.line("i32.const 1")
				} else {
					g.line("i32.const 0")
				}
				g.line("i32.eq")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
			}

		case ir.PString:
			if !isLast {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				idx := g.internString(pat.Value)
				g.line("(array.new_data $string $d%d (i32.const 0) (i32.const %d))", idx, len(pat.Value))
				g.line("call $string_eq")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
			}

		case ir.PNil:
			// Empty list: tag == 0
			if !isLast {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("(struct.get $list $tag)")
				g.line("i32.eqz")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitExprTail(arm.Body, tail); err != nil {
					return err
				}
			}

		case ir.PCons:
			// Cons cell: tag == 1, bind head and tail
			if !isLast {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("(struct.get $list $tag)")
				g.line("(i32.const 1)")
				g.line("i32.eq")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
			}
			// Bind head if PVar
			if pv, ok := pat.Head.(ir.PVar); ok {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("(struct.get $list_cons $head (ref.cast (ref $list_cons)))")
				g.line("local.set $%s", pv.Name)
			}
			// Bind tail if PVar
			if pv, ok := pat.Tail.(ir.PVar); ok {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("(struct.get $list_cons $tail (ref.cast (ref $list_cons)))")
				g.line("local.set $%s", pv.Name)
			}
			if err := g.emitExprTail(arm.Body, tail); err != nil {
				return err
			}
			if !isLast {
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			}

		case ir.PTuple:
			// Tuple pattern: extract each field
			for fi, subPat := range pat.Pats {
				if pv, ok := subPat.(ir.PVar); ok {
					arity := len(pat.Pats)
					if err := g.emitAtom(scrut); err != nil {
						return err
					}
					g.line("(struct.get $tuple%d $f%d)", arity, fi)
					g.line("local.set $%s", pv.Name)
				}
			}
			if err := g.emitExprTail(arm.Body, tail); err != nil {
				return err
			}

		default:
			return fmt.Errorf("codegen: unsupported pattern type %T", pat)
		}
	}

	// Close all the if/else blocks (one for each non-last arm)
	for i := 0; i < numArms-1; i++ {
		g.indent--
		g.line(")")
		g.indent--
		g.line(")")
	}

	return nil
}

// emitPatternBindings extracts fields from an ADT struct and binds them
// to the pattern variables.
func (g *watGen) emitPatternBindings(scrutName string, ci *ctorInfo, pats []ir.Pattern) error {
	for i, pat := range pats {
		switch p := pat.(type) {
		case ir.PVar:
			// Extract field i from the constructor struct
			ctorType := fmt.Sprintf("$%s_%s", ci.typeName, ci.name)
			g.line("(local.set $%s (struct.get %s $f%d (ref.cast (ref %s) (local.get $%s))))",
				p.Name, ctorType, i, ctorType, scrutName)
		case ir.PWild:
			// Skip
		default:
			return fmt.Errorf("codegen: nested pattern in constructor not yet supported: %T", pat)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Lists and tuples
// ---------------------------------------------------------------------------

// emitList creates a cons-cell chain from a CList literal.
// [1, 2, 3] → cons(1, cons(2, cons(3, nil)))
func (g *watGen) emitList(e ir.CList) error {
	if len(e.Items) == 0 {
		g.line("(struct.new $list (i32.const 0))")
		return nil
	}

	// Build nested s-expressions from left to right.
	// Each cons cell wraps the tail which is the next cons (or nil).
	for i := 0; i < len(e.Items); i++ {
		g.line("(struct.new $list_cons (i32.const 1)")
		g.indent++
		if err := g.emitAtom(e.Items[i]); err != nil {
			return err
		}
	}
	// Innermost tail is nil
	g.line("(struct.new $list (i32.const 0))")
	// Close all the struct.new parens
	for i := 0; i < len(e.Items); i++ {
		g.indent--
		g.line(")")
	}
	return nil
}

// emitTuple creates a tuple struct.
func (g *watGen) emitTuple(e ir.CTuple) error {
	arity := len(e.Items)
	g.line("(struct.new $tuple%d", arity)
	g.indent++
	for _, item := range e.Items {
		if err := g.emitAtom(item); err != nil {
			return err
		}
	}
	g.indent--
	g.line(")")
	return nil
}

// ---------------------------------------------------------------------------
// Closures
// ---------------------------------------------------------------------------

func (g *watGen) emitClosureCreate(lam ir.CLambda) error {
	// Find the corresponding lifted lambda
	for i := range g.lambdas {
		lf := &g.lambdas[i]
		// Match by param name and body (simple heuristic)
		if lf.param == lam.Param {
			closureType := "$closure"
			nCaps := len(lf.captures)
			if nCaps > 0 {
				closureType = fmt.Sprintf("$closure_%d", nCaps)
			}

			g.line("(struct.new %s", closureType)
			g.indent++
			g.funcRefs[lf.name] = true
			g.line("(ref.func %s)", lf.name)
			for _, cap := range lf.captures {
				g.line("(local.get $%s)", cap)
			}
			g.indent--
			g.line(")")
			return nil
		}
	}

	return fmt.Errorf("codegen: lambda not found in lifted lambdas (param=%s)", lam.Param)
}

// emitFuncWrapper generates a wrapper function that allows a top-level function
// to be called through the closure calling convention ($ft_apply).
func (g *watGen) emitFuncWrapper(fi *funcInfo, wrapperName string) {
	// Wrapper must match $ft_apply exactly (same type index from rec group)
	g.line("(func %s (type $ft_apply) (param $self (ref null $closure)) (param $arg i64) (result i64)",
		wrapperName)
	g.indent++
	// Convert arg from i64 to the function's expected param type if needed
	g.line("local.get $arg")
	if fi.params[0].wasmType != wtI64 {
		g.emitConvert(wtI64, fi.params[0].wasmType)
	}
	g.line("call $%s", fi.name)
	// Convert return to i64 if needed
	if fi.retType != wtI64 {
		g.emitConvert(fi.retType, wtI64)
	}
	g.indent--
	g.line(")")
}

// ---------------------------------------------------------------------------
// Binary operators
// ---------------------------------------------------------------------------

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
		if opType == wtStringRef {
			g.line("call $string_eq")
		} else {
			g.line("%s.eq", opType)
		}
	case "Neq":
		if opType == wtStringRef {
			g.line("call $string_eq")
			g.line("i32.eqz")
		} else {
			g.line("%s.ne", opType)
		}
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
	return g.emitIfTail(e, false)
}

func (g *watGen) emitIfTail(e ir.CIf, tail bool) error {
	resultType := g.typeOfExpr(e.Then)
	if err := g.emitAtom(e.Cond); err != nil {
		return err
	}
	g.line("(if (result %s)", resultType)
	g.indent++
	g.line("(then")
	g.indent++
	if err := g.emitExprTail(e.Then, tail); err != nil {
		return err
	}
	g.indent--
	g.line(")")
	g.line("(else")
	g.indent++
	if err := g.emitExprTail(e.Else, tail); err != nil {
		return err
	}
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func copyScope(m map[string]bool) map[string]bool {
	c := make(map[string]bool, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

func containsStr(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func sortInts(s []int) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
