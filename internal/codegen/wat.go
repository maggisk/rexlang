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
	wtStringRef = "(ref null $string)"  // string reference (nullable for locals)
	wtListRef   = "(ref null $list)"    // list reference (nullable for locals)
	wtAnyRef    = "(ref null any)"      // universal boxed type for polymorphic storage
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
		partialApps:   make(map[string]bool),
		adts:          make(map[string]*adtInfo),
		ctorToAdt:     make(map[string]*ctorInfo),
		records:       make(map[string]*recordInfo),
		stringDataIdx: make(map[string]int),
		usesTuples:    make(map[int]bool),
		traitMethods:  make(map[string]traitMethodInfo),
		implFuncs:     make(map[string]*funcInfo),
		implBodies:    make(map[string]ir.Expr),
		dispatchFuncs: make(map[string]*dispatchFuncDef),
		usedBuiltins:  make(map[string]bool),
	}
	return g.emit(prog)
}

// adtInfo describes an ADT for codegen.
type adtInfo struct {
	name  string
	ctors []ctorInfo
}

type ctorInfo struct {
	name       string
	tag        int
	typeName   string   // parent ADT name
	fieldTypes []string // wasm types of constructor fields
}

// recordInfo describes a record type for codegen.
type recordInfo struct {
	name       string
	fieldNames []string // ordered field names
	fieldTypes []string // wasm types matching fieldNames
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
	name        string   // generated func name
	captures    []string // names of captured variables
	capTypes    []string // wasm types of captures
	param       string
	paramType   string // concrete wasm type of the parameter
	retType     string
	body        ir.Expr
	used        bool   // true after closure creation emitted
	selfCapture string // non-empty if one capture is the closure itself (self-recursion)
}

// pendingCall tracks partial application chains for multi-arg call detection.
type pendingCall struct {
	funcName string
	args     []ir.Atom
}

// traitMethodInfo maps a method name back to its trait.
type traitMethodInfo struct {
	traitName  string
	methodName string
}

// dispatchFuncDef defines runtime dispatch for a trait method.
// Generated as: resolve function (br_on_cast → funcref) + dispatch function (resolve + call_ref).
type dispatchFuncDef struct {
	name       string // e.g. "dispatch_show"
	methodName string // e.g. "show"
	arity      int    // number of args the trait method takes
	implCases  []dispatchCase
}

type dispatchCase struct {
	typeName string // Rex type name ("Int", "Float", "List", etc.)
	implName string // impl function name ("Show_Int_show")
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

	// Record info
	records map[string]*recordInfo // record type name → info

	// functions referenced via ref.func (need elem declare)
	funcRefs map[string]bool

	// wrapper functions for top-level funcs used as values
	funcWrappers map[string]string // func name → wrapper name

	// partial application wrappers: "funcName_pa1" → funcInfo
	partialApps map[string]bool

	// string literal data segments
	stringData    []string       // ordered unique string values
	stringDataIdx map[string]int // string value → index in stringData

	// list/tuple usage tracking
	usesLists        bool
	usesStringConcat bool
	usesTuples       map[int]bool // tuple arity → true

	// trait dispatch
	traitMethods  map[string]traitMethodInfo  // method name → trait info ("show" → Show)
	implFuncs     map[string]*funcInfo        // "Show_Int_show" → generated func info
	implBodies    map[string]ir.Expr          // "Show_Int_show" → impl method body
	dispatchFuncs map[string]*dispatchFuncDef // "dispatch_show" → dispatch function def

	// builtin tracking
	usedBuiltins map[string]bool // builtin names referenced in code

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
			if _, ok := g.records[ty.Name]; ok {
				return fmt.Sprintf("(ref null $rec_%s)", ty.Name)
			}
		}
	}
	return wtAnyRef // type variables and unknown types use boxed representation
}

// isRefType returns true for all wasm reference types (already subtypes of any).
func isRefType(wt string) bool {
	switch wt {
	case wtRef, wtAdtRef, wtStringRef, wtListRef, wtAnyRef:
		return true
	}
	// Also handle tuple refs like "(ref null $tuple2)"
	if len(wt) > 4 && wt[:4] == "(ref" {
		return true
	}
	return false
}

// emitBox boxes a concrete wasm value (on stack) to anyref.
// For ref types this is a no-op (implicit upcast to any).
func (g *watGen) emitBox(fromType string) {
	switch fromType {
	case wtI64:
		g.line("(struct.new $box_i64)")
	case wtF64:
		g.line("(struct.new $box_f64)")
	case wtI32:
		g.line("(ref.i31)")
	case wtAnyRef:
		// already anyref, no-op
	default:
		if isRefType(fromType) {
			// ref types are subtypes of any — no-op
		} else {
			// Unknown type, treat as i64
			g.line("(struct.new $box_i64)")
		}
	}
}

// emitUnbox unboxes anyref (on stack) to a concrete wasm type.
func (g *watGen) emitUnbox(toType string) {
	switch toType {
	case wtI64:
		g.line("(ref.cast (ref $box_i64))")
		g.line("(struct.get $box_i64 $val)")
	case wtF64:
		g.line("(ref.cast (ref $box_f64))")
		g.line("(struct.get $box_f64 $val)")
	case wtI32:
		g.line("(ref.cast (ref i31))")
		g.line("(i31.get_s)")
	case wtAnyRef:
		// no-op
	default:
		if isRefType(toType) {
			g.line("(ref.cast %s)", toType)
		}
		// else no-op
	}
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
	// Pass 0: collect ADT and record info (before functions, since param types may reference them)
	for _, d := range prog.Decls {
		dt, ok := d.(ir.DType)
		if !ok {
			continue
		}
		if len(dt.Fields) > 0 {
			g.analyzeRecord(dt)
		} else if len(dt.Ctors) > 0 {
			g.analyzeADT(dt)
		}
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

	// Trait pass: collect trait methods and impl functions
	g.analyzeTraits(prog)

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

	// Third pass: collect string literals (including impl bodies)
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
	for _, body := range g.implBodies {
		g.scanStrings(body)
	}

	// Fourth pass: detect builtin usage
	for _, d := range prog.Decls {
		dl, ok := d.(ir.DLet)
		if !ok {
			continue
		}
		fi := g.funcs[dl.Name]
		if fi == nil {
			continue
		}
		g.scanBuiltins(fi.body)
	}
	for _, body := range g.implBodies {
		g.scanBuiltins(body)
	}

	// Resolve transitive builtin dependencies
	if g.usedBuiltins["println"] || g.usedBuiltins["print"] {
		// println/print for Int calls showInt
		g.usedBuiltins["showInt"] = true
		// println/print for Float calls showFloat (which calls showInt)
		g.usedBuiltins["showFloat"] = true
		// println/print for Bool needs "true"/"false" string literals
		g.internString("true")
		g.internString("false")
	}
	if g.usedBuiltins["showFloat"] {
		g.usedBuiltins["showInt"] = true
	}
	// String interpolation may need showInt/showFloat for non-string parts
	if g.usesStringConcat {
		g.usedBuiltins["showInt"] = true
		g.usedBuiltins["showFloat"] = true
		g.internString("true")
		g.internString("false")
	}
}

// knownBuiltins is the set of builtin names that the codegen can emit inline.
var knownBuiltins = map[string]bool{
	"println":   true,
	"print":     true,
	"showInt":   true,
	"showFloat": true,
	"not":       true,
}

func (g *watGen) scanBuiltins(expr ir.Expr) {
	switch e := expr.(type) {
	case ir.EAtom:
		g.checkAtomBuiltin(e.A)
	case ir.EComplex:
		g.scanCExprBuiltins(e.C)
	case ir.ELet:
		g.scanCExprBuiltins(e.Bind)
		g.scanBuiltins(e.Body)
	case ir.ELetRec:
		for _, b := range e.Bindings {
			g.scanCExprBuiltins(b.Bind)
		}
		g.scanBuiltins(e.Body)
	}
}

func (g *watGen) scanCExprBuiltins(c ir.CExpr) {
	switch e := c.(type) {
	case ir.CApp:
		if v, ok := e.Func.(ir.AVar); ok && knownBuiltins[v.Name] {
			g.usedBuiltins[v.Name] = true
		}
		g.checkAtomBuiltin(e.Arg)
	case ir.CIf:
		g.scanBuiltins(e.Then)
		g.scanBuiltins(e.Else)
	case ir.CMatch:
		for _, arm := range e.Arms {
			g.scanBuiltins(arm.Body)
		}
	case ir.CLambda:
		g.scanBuiltins(e.Body)
	}
}

func (g *watGen) checkAtomBuiltin(a ir.Atom) {
	if v, ok := a.(ir.AVar); ok && knownBuiltins[v.Name] {
		g.usedBuiltins[v.Name] = true
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
	case ir.ELetRec:
		for _, b := range e.Bindings {
			g.scanCExprStrings(b.Bind)
		}
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
		if e.Op == "Concat" {
			g.usesStringConcat = true
		}
		if e.Op == "Cons" {
			g.usesLists = true
		}
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
	case ir.CStringInterp:
		g.usesStringConcat = true
		for _, part := range e.Parts {
			g.scanAtomString(part)
		}
	case ir.CRecord:
		for _, fi := range e.Fields {
			g.scanAtomString(fi.Value)
		}
	case ir.CFieldAccess:
		g.scanAtomString(e.Record)
	case ir.CRecordUpdate:
		g.scanAtomString(e.Record)
		for _, u := range e.Updates {
			g.scanAtomString(u.Value)
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

// emitStringConcat emits a helper that concatenates two $string arrays into a new one.
func (g *watGen) emitStringConcat() {
	g.line("(func $string_concat (param $a (ref null $string)) (param $b (ref null $string)) (result (ref null $string))")
	g.indent++
	g.line("(local $len_a i32)")
	g.line("(local $len_b i32)")
	g.line("(local $result (ref null $string))")
	g.line("(local $i i32)")
	// Get lengths
	g.line("(local.set $len_a (array.len (local.get $a)))")
	g.line("(local.set $len_b (array.len (local.get $b)))")
	// Create new array of combined length, initialized to 0
	g.line("(local.set $result (array.new $string (i32.const 0) (i32.add (local.get $len_a) (local.get $len_b))))")
	// Copy bytes from $a
	g.line("(local.set $i (i32.const 0))")
	g.line("(block $done_a")
	g.indent++
	g.line("(loop $loop_a")
	g.indent++
	g.line("(br_if $done_a (i32.ge_u (local.get $i) (local.get $len_a)))")
	g.line("(array.set $string (local.get $result) (local.get $i) (array.get_u $string (local.get $a) (local.get $i)))")
	g.line("(local.set $i (i32.add (local.get $i) (i32.const 1)))")
	g.line("(br $loop_a)")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	// Copy bytes from $b
	g.line("(local.set $i (i32.const 0))")
	g.line("(block $done_b")
	g.indent++
	g.line("(loop $loop_b")
	g.indent++
	g.line("(br_if $done_b (i32.ge_u (local.get $i) (local.get $len_b)))")
	g.line("(array.set $string (local.get $result) (i32.add (local.get $len_a) (local.get $i)) (array.get_u $string (local.get $b) (local.get $i)))")
	g.line("(local.set $i (i32.add (local.get $i) (i32.const 1)))")
	g.line("(br $loop_b)")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	// Return result
	g.line("(local.get $result)")
	g.indent--
	g.line(")")
}

// emitStringInterp emits code for string interpolation.
// Each part is converted to a string (if not already) and concatenated.
func (g *watGen) emitStringInterp(e ir.CStringInterp) error {
	if len(e.Parts) == 0 {
		// Empty interpolation → empty string
		idx := g.internString("")
		g.line("(array.new_data $string $d%d (i32.const 0) (i32.const 0))", idx)
		return nil
	}

	// Emit first part as a string
	if err := g.emitAtomAsString(e.Parts[0]); err != nil {
		return err
	}

	// For each subsequent part, emit as string and concat
	for _, part := range e.Parts[1:] {
		if err := g.emitAtomAsString(part); err != nil {
			return err
		}
		g.line("call $string_concat")
	}

	return nil
}

// emitAtomAsString emits an atom, converting it to a $string if needed.
func (g *watGen) emitAtomAsString(a ir.Atom) error {
	atomType := g.typeOfAtom(a)
	switch atomType {
	case wtStringRef:
		return g.emitAtom(a)
	case wtI64:
		if err := g.emitAtom(a); err != nil {
			return err
		}
		g.line("call $showInt")
		return nil
	case wtF64:
		if err := g.emitAtom(a); err != nil {
			return err
		}
		g.line("call $showFloat")
		return nil
	case wtI32:
		// Bool → "true" or "false"
		if err := g.emitAtom(a); err != nil {
			return err
		}
		idxTrue := g.internString("true")
		idxFalse := g.internString("false")
		g.line("(if (result (ref null $string))")
		g.indent++
		g.line("(then (array.new_data $string $d%d (i32.const 0) (i32.const 4)))", idxTrue)
		g.line("(else (array.new_data $string $d%d (i32.const 0) (i32.const 5)))", idxFalse)
		g.indent--
		g.line(")")
		return nil
	default:
		// anyref — try to unbox and convert
		// For now, just emit as-is (will need show trait dispatch later)
		return g.emitAtom(a)
	}
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
				fi.params[i].wasmType = wtAnyRef
			}
		}
		fi.retType = g.wasmType(retType)
	} else {
		// Default: all anyref for unknown types
		for i := range fi.params {
			fi.params[i].wasmType = wtAnyRef
		}
		fi.retType = wtAnyRef
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
		// Determine field types from type env, fall back to IR arg count
		if ty, ok := g.lookupType(c.Name); ok {
			paramTypes, _ := decomposeFuncType(ty)
			for _, pt := range paramTypes {
				ci.fieldTypes = append(ci.fieldTypes, g.wasmType(pt))
			}
		} else {
			// Type env doesn't have this constructor (e.g., from imported module).
			// Use IR's arg count with anyref as field type.
			for range c.ArgTypes {
				ci.fieldTypes = append(ci.fieldTypes, wtAnyRef)
			}
		}
		ai.ctors = append(ai.ctors, ci)
		g.ctorToAdt[c.Name] = &ai.ctors[len(ai.ctors)-1]
	}
	g.adts[dt.Name] = ai
}

func (g *watGen) analyzeRecord(dt ir.DType) {
	ri := &recordInfo{name: dt.Name}
	// Try to get field types from __record_fields__ in typeEnv (IR may have nil types)
	var rfMap map[string]types.RecordInfo
	if rf, ok := g.typeEnv["__record_fields__"]; ok {
		rfMap, _ = rf.(map[string]types.RecordInfo)
	}
	for _, f := range dt.Fields {
		ri.fieldNames = append(ri.fieldNames, f.Name)
		wt := wtAnyRef
		if rfMap != nil {
			if rfi, ok := rfMap[dt.Name]; ok {
				for _, fi := range rfi.Fields {
					if fi.Name == f.Name {
						wt = g.wasmType(fi.Type)
						break
					}
				}
			}
		}
		ri.fieldTypes = append(ri.fieldTypes, wt)
	}
	g.records[dt.Name] = ri
}

// analyzeTraits collects trait method info and generates impl function entries.
func (g *watGen) analyzeTraits(prog *ir.Program) {
	// Collect trait methods from typeEnv (DTrait.Methods may be empty in IR)
	if t, ok := g.typeEnv["__traits__"]; ok {
		if tm, ok := t.(map[string]typechecker.TraitInfo); ok {
			for traitName, ti := range tm {
				for methodName := range ti.Methods {
					g.traitMethods[methodName] = traitMethodInfo{
						traitName:  traitName,
						methodName: methodName,
					}
				}
			}
		}
	}

	// Process impls — generate impl function entries
	// Also track which impls exist per (traitName, methodName) for dispatch
	type methodKey struct{ trait, method string }
	implsByMethod := map[methodKey][]dispatchCase{}

	for _, d := range prog.Decls {
		di, ok := d.(ir.DImpl)
		if !ok {
			continue
		}
		for _, m := range di.Methods {
			implName := di.TraitName + "_" + di.TargetTypeName + "_" + m.Name
			// Skip duplicate impls (e.g. Prelude impls included via ResolveImports)
			if _, exists := g.implFuncs[implName]; exists {
				continue
			}
			fi := g.analyzeImplMethod(implName, di.TraitName, di.TargetTypeName, m)
			if fi != nil {
				g.implFuncs[implName] = fi
				g.funcs[implName] = fi
				g.implBodies[implName] = fi.body
				if fi.arity > 1 && fi.arity-1 > g.maxCaps {
					g.maxCaps = fi.arity - 1
				}
				key := methodKey{di.TraitName, m.Name}
				implsByMethod[key] = append(implsByMethod[key], dispatchCase{
					typeName: di.TargetTypeName,
					implName: implName,
				})
			}
		}
	}

	// Build dispatch functions for each trait method
	for methodName, tmInfo := range g.traitMethods {
		key := methodKey{tmInfo.traitName, tmInfo.methodName}
		cases := implsByMethod[key]
		if len(cases) == 0 {
			continue
		}
		// Use the first impl's funcInfo to determine arity and return type
		firstImpl := g.implFuncs[cases[0].implName]
		dispName := "dispatch_" + methodName
		g.dispatchFuncs[dispName] = &dispatchFuncDef{
			name:       dispName,
			methodName: methodName,
			arity:      firstImpl.arity,
			implCases:  cases,
		}
		// Register a funcInfo so trySaturatedCallTail can use it
		dispParams := make([]paramInfo, firstImpl.arity)
		for i := range dispParams {
			dispParams[i] = paramInfo{name: fmt.Sprintf("a%d", i), wasmType: wtAnyRef}
		}
		g.funcs[dispName] = &funcInfo{
			name:    dispName,
			arity:   firstImpl.arity,
			params:  dispParams,
			retType: wtAnyRef,
		}
	}
}

// analyzeImplMethod creates a funcInfo for an impl method by looking up
// the trait method's type scheme and substituting the concrete target type.
func (g *watGen) analyzeImplMethod(implName, traitName, targetTypeName string, m ir.ImplMethodDef) *funcInfo {
	// Look up trait info from typeEnv
	var traitInfo *typechecker.TraitInfo
	if t, ok := g.typeEnv["__traits__"]; ok {
		if tm, ok := t.(map[string]typechecker.TraitInfo); ok {
			if ti, ok := tm[traitName]; ok {
				traitInfo = &ti
			}
		}
	}
	if traitInfo == nil {
		return nil
	}

	methodScheme, ok := traitInfo.Methods[m.Name]
	if !ok {
		return nil
	}

	// Substitute the trait param with the target concrete type
	targetType := g.rexNameToType(targetTypeName)
	paramSubst := types.Subst{traitInfo.Param: targetType}
	concreteType := types.ApplySubst(paramSubst, methodScheme.Ty)

	// Unwrap lambda chain from the body to get params
	body := m.Body
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
		return nil
	}

	fi := &funcInfo{
		name:   implName,
		arity:  len(params),
		params: params,
		body:   body,
	}

	// Use the concrete type to determine param and return types
	paramTypes, retType := decomposeFuncType(concreteType)
	for i := range fi.params {
		if i < len(paramTypes) {
			fi.params[i].wasmType = g.wasmType(paramTypes[i])
		} else {
			fi.params[i].wasmType = wtAnyRef
		}
	}
	fi.retType = g.wasmType(retType)

	return fi
}

// castRefType returns the WAT ref type for br_on_cast targeting a Rex type name.
// Returns "" if the type cannot be tested at runtime.
func (g *watGen) castRefType(typeName string) string {
	switch typeName {
	case "Int":
		return "(ref $box_i64)"
	case "Float":
		return "(ref $box_f64)"
	case "Bool":
		return "(ref i31)"
	case "String":
		return "(ref $string)"
	case "List":
		return "(ref $list)"
	case "Unit":
		// Unit is encoded as i31ref with value 0 — same as Bool false
		// Can't distinguish at runtime from Bool, skip for now
		return ""
	default:
		// Check if it's an ADT with a supertype
		if _, ok := g.adts[typeName]; ok {
			return fmt.Sprintf("(ref $%s)", typeName)
		}
		return ""
	}
}

type dispatchImplCase struct {
	dc dispatchCase
	fi *funcInfo
}

// emitDispatchFuncs emits resolve, dispatch, and wrapper functions for trait methods.
//
// For each trait method (e.g. "show") with impls, we generate:
//   - $Show_Int_show__wrap: wrapper that takes/returns anyref, unboxes, calls real impl, boxes
//   - $resolve_show: uses br_on_cast to return the right wrapper funcref
//   - $dispatch_show: calls resolve then call_ref (thin wrapper)
func (g *watGen) emitDispatchFuncs() error {
	dispNames := make([]string, 0, len(g.dispatchFuncs))
	for name := range g.dispatchFuncs {
		dispNames = append(dispNames, name)
	}
	sortStrings(dispNames)

	for _, name := range dispNames {
		df := g.dispatchFuncs[name]
		cases := g.filterDispatchCases(df)
		if len(cases) == 0 {
			continue
		}

		// Emit wrapper functions for each impl
		for _, c := range cases {
			g.emitImplWrapper(c, df)
			g.line("")
		}

		// Emit resolve function (br_on_cast → funcref)
		g.emitResolveFunc(df, cases)
		g.line("")

		// Emit dispatch function (resolve + call_ref)
		g.emitDispatchFunc(df)
		g.line("")
	}
	return nil
}

func (g *watGen) filterDispatchCases(df *dispatchFuncDef) []dispatchImplCase {
	var cases []dispatchImplCase
	for _, dc := range df.implCases {
		castType := g.castRefType(dc.typeName)
		if castType == "" {
			continue
		}
		fi := g.implFuncs[dc.implName]
		if fi == nil {
			continue
		}
		cases = append(cases, dispatchImplCase{dc, fi})
	}
	return cases
}

// emitImplWrapper emits a wrapper function that takes/returns anyref,
// unboxes args, calls the real impl, and boxes the result.
func (g *watGen) emitImplWrapper(c dispatchImplCase, df *dispatchFuncDef) {
	wrapName := c.dc.implName + "__wrap"
	ftName := fmt.Sprintf("$ft_trait_%s", df.methodName)
	g.line("(func $%s (type %s)", wrapName, ftName)
	g.indent++

	// Unbox each param and call the real impl
	for j := 0; j < c.fi.arity; j++ {
		g.line("(local.get %d)", j)
		g.emitUnbox(c.fi.params[j].wasmType)
	}
	g.line("(call $%s)", c.fi.name)
	g.emitBox(c.fi.retType)

	g.indent--
	g.line(")")

	// Register funcref for elem declare (with $ prefix)
	g.funcRefs["$"+wrapName] = true
}

// emitResolveFunc emits a function that uses br_on_cast to determine the
// concrete type and returns the corresponding wrapper funcref.
func (g *watGen) emitResolveFunc(df *dispatchFuncDef, cases []dispatchImplCase) {
	ftName := fmt.Sprintf("$ft_trait_%s", df.methodName)
	g.line("(func $resolve_%s (param $obj (ref null any)) (result (ref %s))", df.methodName, ftName)
	g.indent++

	// Outer block catches the funcref from inner handlers
	g.line("(block $done (result (ref %s))", ftName)
	g.indent++

	// Emit nested blocks: one per case (outermost = last case)
	// The innermost block contains the br_on_cast instructions
	for i := len(cases) - 1; i >= 0; i-- {
		castType := g.castRefType(cases[i].dc.typeName)
		g.line("(block $case_%d (result %s)", i, castType)
		g.indent++
	}

	// The innermost code: load obj and try each cast
	g.line("(local.get $obj)")
	for i := range cases {
		castType := g.castRefType(cases[i].dc.typeName)
		g.line("(br_on_cast $case_%d anyref %s)", i, castType)
	}
	g.line("(unreachable)")

	// Close blocks and emit handlers (innermost first)
	for i := range cases {
		g.indent--
		g.line(") ;; end $case_%d", i)
		// The casted value is on the stack — drop it, push funcref
		g.line("(drop)")
		wrapName := cases[i].dc.implName + "__wrap"
		g.line("(ref.func $%s)", wrapName)
		g.line("(br $done)")
	}

	g.indent--
	g.line(") ;; end $done")

	g.indent--
	g.line(")")
}

// emitDispatchFunc emits a thin dispatch function that calls resolve then call_ref.
func (g *watGen) emitDispatchFunc(df *dispatchFuncDef) error {
	params := ""
	for i := 0; i < df.arity; i++ {
		params += fmt.Sprintf(" (param $a%d (ref null any))", i)
	}
	ftName := fmt.Sprintf("$ft_trait_%s", df.methodName)
	g.line("(func $%s%s (result (ref null any))", df.name, params)
	g.indent++

	// Push args for call_ref
	for i := 0; i < df.arity; i++ {
		g.line("(local.get $a%d)", i)
	}
	// Resolve funcref based on first arg's type
	g.line("(local.get $a0)")
	g.line("(call $resolve_%s)", df.methodName)
	// Call the resolved funcref
	g.line("(call_ref %s)", ftName)

	g.indent--
	g.line(")")
	return nil
}

// rexNameToType converts a Rex type name (e.g. "Int", "String") to a types.Type.
func (g *watGen) rexNameToType(name string) types.Type {
	return types.TCon{Name: name}
}

// isLocalOrParam returns true if the name is a local variable or function parameter
// (i.e. it shadows a global name like a trait method or top-level function).
func (g *watGen) isLocalOrParam(name string) bool {
	_, isParam := g.funcParams[name]
	_, isLocal := g.locals[name]
	return isParam || isLocal
}

// rexTypeName determines the Rex type name of an atom for trait dispatch.
// Returns "" if the type cannot be statically determined.
func (g *watGen) rexTypeName(a ir.Atom) string {
	switch a.(type) {
	case ir.AInt:
		return "Int"
	case ir.AFloat:
		return "Float"
	case ir.AString:
		return "String"
	case ir.ABool:
		return "Bool"
	case ir.AUnit:
		return "Unit"
	}
	if v, ok := a.(ir.AVar); ok {
		wt := g.typeOfAtom(a)
		switch wt {
		case wtI64:
			return "Int"
		case wtF64:
			return "Float"
		case wtI32:
			return "Bool"
		case wtStringRef:
			return "String"
		case wtListRef:
			return "List"
		}
		// Check tuple refs
		if len(wt) > 15 && wt[:15] == "(ref null $tupl" {
			return "Tuple" // TODO: extract arity for TupleN
		}
		// Check if it's a zero-arg constructor
		if ci, ok := g.ctorToAdt[v.Name]; ok && len(ci.fieldTypes) == 0 {
			return ci.typeName
		}
		// For ADT refs, try to look up the type from the type env
		if wt == wtAdtRef {
			if ty, ok := g.lookupType(v.Name); ok {
				if tc, ok := ty.(types.TCon); ok {
					return tc.Name
				}
			}
		}
	}
	return ""
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

	// Determine max captures from lambdas in all function bodies.
	// Pre-populate locals and funcParams so capture types are correct.
	for _, fi := range g.funcs {
		g.funcParams = make(map[string]string)
		g.locals = make(map[string]string)
		for _, p := range fi.params {
			g.funcParams[p.name] = p.wasmType
		}
		g.collectLocals(fi.body)
		g.scanForLambdas(fi.name, fi.body, fi.params)
	}
	g.funcParams = make(map[string]string)
	g.locals = make(map[string]string)

	g.line("(module")
	g.indent++

	// Emit GC type declarations (closures + ADTs in one rec group)
	if g.needsGCTypes() {
		g.emitGCTypes()
		g.line("")
	}

	// WASI imports
	g.line("(import \"wasi_snapshot_preview1\" \"proc_exit\" (func $proc_exit (param i32)))")
	if g.needsWASIIO() {
		g.line("(import \"wasi_snapshot_preview1\" \"fd_write\" (func $fd_write (param i32 i32 i32 i32) (result i32)))")
	}
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

	// Emit trait impl functions
	implNames := make([]string, 0, len(g.implFuncs))
	for name := range g.implFuncs {
		implNames = append(implNames, name)
	}
	sortStrings(implNames)
	for _, name := range implNames {
		fi := g.implFuncs[name]
		if err := g.emitFunc(fi); err != nil {
			return "", err
		}
		g.line("")
	}

	// Emit runtime dispatch functions
	if err := g.emitDispatchFuncs(); err != nil {
		return "", err
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

	// Emit partial application wrappers
	// Note: emitting wrappers may register new deeper wrappers, so loop until stable
	for len(g.partialApps) > 0 {
		current := make(map[string]bool)
		for k, v := range g.partialApps {
			current[k] = v
		}
		g.partialApps = make(map[string]bool)
		keys := make([]string, 0, len(current))
		for k := range current {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, paKey := range keys {
			parts := strings.SplitN(paKey, "__pa", 2)
			funcName := parts[0]
			depth := 0
			fmt.Sscanf(parts[1], "%d", &depth)
			fi := g.funcs[funcName]
			if fi == nil {
				continue
			}
			g.emitPartialAppWrapper(fi, depth, paKey)
			g.line("")
		}
	}

	// String helpers
	if g.needsStrings() {
		g.emitStringEq()
		g.line("")
	}
	if g.usesStringConcat {
		g.emitStringConcat()
		g.line("")
	}

	// WASI IO helper functions
	if g.needsWASIIO() {
		g.emitWASIHelpers()
		g.line("")
	}

	// Builtin functions (showInt, showFloat)
	if g.usedBuiltins["showInt"] {
		g.emitShowInt()
		g.line("")
	}
	if g.usedBuiltins["showFloat"] {
		g.emitShowFloat()
		g.line("")
	}

	// _start function — pass dummy arg for main's ignored parameter
	g.line("(func (export \"_start\")")
	g.indent++
	mainParamType := mainFI.params[0].wasmType
	var mainArgStr string
	switch mainParamType {
	case wtI64:
		mainArgStr = "(i64.const 0)"
	case wtListRef:
		if g.usesLists {
			mainArgStr = "(struct.new $list (i32.const 0))"
		} else {
			mainArgStr = "(ref.null $list)"
		}
	case wtAnyRef:
		mainArgStr = "(ref.null any)"
	default:
		mainArgStr = "(ref.null any)"
	}
	g.line("(call $proc_exit (i32.and (i32.wrap_i64 (call $main %s)) (i32.const 255)))", mainArgStr)
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
	return g.needsClosures() || len(g.adts) > 0 || len(g.records) > 0 || g.needsStrings() || g.usesLists || len(g.usesTuples) > 0 || len(g.dispatchFuncs) > 0
}

func (g *watGen) needsStrings() bool {
	return len(g.stringData) > 0 || g.usesStringConcat || g.usedBuiltins["showInt"] || g.usedBuiltins["showFloat"] || g.needsWASIIO()
}

func (g *watGen) needsWASIIO() bool {
	return g.usedBuiltins["println"] || g.usedBuiltins["print"]
}

// ---------------------------------------------------------------------------
// WASI IO helpers
// ---------------------------------------------------------------------------

// emitWASIHelpers emits $__print_str (write GC string to stdout) used by
// println and print builtins. Memory layout:
//
//	offset 0-3:  iovec.buf (i32) = 16
//	offset 4-7:  iovec.len (i32)
//	offset 8-11: nwritten  (i32, fd_write output)
//	offset 16+:  string data buffer
func (g *watGen) emitWASIHelpers() {
	// $__print_str: copy GC string to linear memory, call fd_write
	g.line(";; WASI IO: print GC string to stdout")
	g.line("(func $__print_str (param $s (ref null $string))")
	g.indent++
	g.line("(local $len i32)")
	g.line("(local $i i32)")
	g.line("(local.set $len (array.len (ref.as_non_null (local.get $s))))")
	// Copy bytes to linear memory at offset 16
	g.line("(local.set $i (i32.const 0))")
	g.line("(block $done")
	g.indent++
	g.line("(loop $copy")
	g.indent++
	g.line("(br_if $done (i32.ge_u (local.get $i) (local.get $len)))")
	g.line("(i32.store8 (i32.add (i32.const 16) (local.get $i))")
	g.indent++
	g.line("(array.get_u $string (ref.as_non_null (local.get $s)) (local.get $i)))")
	g.indent--
	g.line("(local.set $i (i32.add (local.get $i) (i32.const 1)))")
	g.line("(br $copy)")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	// Set up iovec: buf=16, len=len
	g.line("(i32.store (i32.const 0) (i32.const 16))")
	g.line("(i32.store (i32.const 4) (local.get $len))")
	// fd_write(stdout=1, iovs=0, iovs_len=1, nwritten=8)
	g.line("(drop (call $fd_write (i32.const 1) (i32.const 0) (i32.const 1) (i32.const 8)))")
	g.indent--
	g.line(")")
	g.line("")

	// $__print_newline: write a single newline to stdout
	g.line("(func $__print_newline")
	g.indent++
	g.line("(i32.store8 (i32.const 16) (i32.const 10))")
	g.line("(i32.store (i32.const 0) (i32.const 16))")
	g.line("(i32.store (i32.const 4) (i32.const 1))")
	g.line("(drop (call $fd_write (i32.const 1) (i32.const 0) (i32.const 1) (i32.const 8)))")
	g.indent--
	g.line(")")
}

// ---------------------------------------------------------------------------
// Builtin conversion functions
// ---------------------------------------------------------------------------

// emitShowInt emits $showInt: i64 → (ref null $string)
// Converts an integer to its decimal string representation.
func (g *watGen) emitShowInt() {
	g.line(";; showInt: i64 → string")
	g.line("(func $showInt (param $n i64) (result (ref null $string))")
	g.indent++
	g.line("(local $neg i32)")
	g.line("(local $pos i32)")
	g.line("(local $len i32)")
	g.line("(local $i i32)")
	g.line("(local $result (ref null $string))")
	// Use linear memory at offset 1024-1044 as temp buffer (max 20 digits + sign)
	g.line("(local.set $pos (i32.const 1044))")
	// Handle negative
	g.line("(if (i64.lt_s (local.get $n) (i64.const 0))")
	g.indent++
	g.line("(then")
	g.indent++
	g.line("(local.set $neg (i32.const 1))")
	g.line("(local.set $n (i64.sub (i64.const 0) (local.get $n)))")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	// Handle zero
	g.line("(if (i64.eqz (local.get $n))")
	g.indent++
	g.line("(then")
	g.indent++
	g.line("(local.set $pos (i32.sub (local.get $pos) (i32.const 1)))")
	g.line("(i32.store8 (local.get $pos) (i32.const 48))") // '0'
	g.indent--
	g.line(")")
	g.line("(else")
	g.indent++
	// Extract digits
	g.line("(block $done")
	g.indent++
	g.line("(loop $digits")
	g.indent++
	g.line("(br_if $done (i64.eqz (local.get $n)))")
	g.line("(local.set $pos (i32.sub (local.get $pos) (i32.const 1)))")
	g.line("(i32.store8 (local.get $pos)")
	g.indent++
	g.line("(i32.add (i32.const 48) (i32.wrap_i64 (i64.rem_u (local.get $n) (i64.const 10)))))")
	g.indent--
	g.line("(local.set $n (i64.div_u (local.get $n) (i64.const 10)))")
	g.line("(br $digits)")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	// Prepend minus if negative
	g.line("(if (local.get $neg)")
	g.indent++
	g.line("(then")
	g.indent++
	g.line("(local.set $pos (i32.sub (local.get $pos) (i32.const 1)))")
	g.line("(i32.store8 (local.get $pos) (i32.const 45))") // '-'
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	// Length = 1044 - pos
	g.line("(local.set $len (i32.sub (i32.const 1044) (local.get $pos)))")
	// Create GC string array and copy
	g.line("(local.set $result (array.new_default $string (local.get $len)))")
	g.line("(local.set $i (i32.const 0))")
	g.line("(block $done2")
	g.indent++
	g.line("(loop $copy")
	g.indent++
	g.line("(br_if $done2 (i32.ge_u (local.get $i) (local.get $len)))")
	g.line("(array.set $string (ref.as_non_null (local.get $result))")
	g.indent++
	g.line("(local.get $i)")
	g.line("(i32.load8_u (i32.add (local.get $pos) (local.get $i))))")
	g.indent--
	g.line("(local.set $i (i32.add (local.get $i) (i32.const 1)))")
	g.line("(br $copy)")
	g.indent--
	g.line(")")
	g.indent--
	g.line(")")
	g.line("(local.get $result)")
	g.indent--
	g.line(")")
}

// emitShowFloat emits $showFloat: f64 → (ref null $string)
// Basic float-to-string conversion using dtoa-style algorithm.
func (g *watGen) emitShowFloat() {
	// For now, emit a simple wrapper that converts float to int and shows that,
	// plus a decimal point. Full dtoa is complex — this is a placeholder.
	// TODO: implement proper float formatting
	g.line(";; showFloat: f64 → string (placeholder — truncates to int)")
	g.line("(func $showFloat (param $f f64) (result (ref null $string))")
	g.indent++
	g.line("(call $showInt (i64.trunc_f64_s (local.get $f)))")
	g.indent--
	g.line(")")
}

// typeOfBuiltinApp returns the wasm result type for a builtin application.
func (g *watGen) typeOfBuiltinApp(name string, arg ir.Atom) string {
	switch name {
	case "println", "print":
		// a -> a: returns same type as argument
		return g.typeOfAtom(arg)
	case "showInt", "showFloat":
		return wtStringRef
	case "not":
		return wtI32
	default:
		return wtAnyRef
	}
}

// ---------------------------------------------------------------------------
// Builtin call emission
// ---------------------------------------------------------------------------

// emitBuiltinApp emits inline code for a builtin function call.
// Builtins are expanded at the call site rather than being separate wasm functions.
func (g *watGen) emitBuiltinApp(name string, arg ir.Atom) error {
	argType := g.typeOfAtom(arg)

	switch name {
	case "println":
		return g.emitBuiltinPrint(arg, argType, true)

	case "print":
		return g.emitBuiltinPrint(arg, argType, false)

	case "showInt":
		if err := g.emitAtom(arg); err != nil {
			return err
		}
		if argType != wtI64 {
			g.emitConvert(argType, wtI64)
		}
		g.line("call $showInt")
		return nil

	case "showFloat":
		if err := g.emitAtom(arg); err != nil {
			return err
		}
		if argType != wtF64 {
			g.emitConvert(argType, wtF64)
		}
		g.line("call $showFloat")
		return nil

	case "not":
		if err := g.emitAtom(arg); err != nil {
			return err
		}
		if argType != wtI32 {
			g.emitConvert(argType, wtI32)
		}
		g.line("(i32.eqz)")
		return nil

	default:
		return fmt.Errorf("codegen: unknown builtin %s", name)
	}
}

// emitBuiltinPrint handles println/print for any type. Converts the value to
// a string, prints it, and returns the original value.
func (g *watGen) emitBuiltinPrint(arg ir.Atom, argType string, newline bool) error {
	switch argType {
	case wtStringRef:
		// Print the string directly, return the same string
		if err := g.emitAtom(arg); err != nil {
			return err
		}
		g.line("call $__print_str")
		if newline {
			g.line("call $__print_newline")
		}
		// Return the original value
		return g.emitAtom(arg)

	case wtI64:
		// Convert to string, print, return original int
		if err := g.emitAtom(arg); err != nil {
			return err
		}
		g.line("call $showInt")
		g.line("call $__print_str")
		if newline {
			g.line("call $__print_newline")
		}
		return g.emitAtom(arg)

	case wtF64:
		if err := g.emitAtom(arg); err != nil {
			return err
		}
		g.line("call $showFloat")
		g.line("call $__print_str")
		if newline {
			g.line("call $__print_newline")
		}
		return g.emitAtom(arg)

	case wtI32:
		// Bool: print "true" or "false"
		if err := g.emitAtom(arg); err != nil {
			return err
		}
		// Convert bool to string: use if/else
		trueIdx := g.internString("true")
		falseIdx := g.internString("false")
		g.line("(if (result (ref null $string))")
		g.indent++
		g.line("(then (array.new_data $string $d%d (i32.const 0) (i32.const 4)))", trueIdx)
		g.line("(else (array.new_data $string $d%d (i32.const 0) (i32.const 5)))", falseIdx)
		g.indent--
		g.line(")")
		g.line("call $__print_str")
		if newline {
			g.line("call $__print_newline")
		}
		return g.emitAtom(arg)

	default:
		// For anyref/complex types: try to use show trait dispatch if available,
		// otherwise just print "<value>"
		if _, ok := g.dispatchFuncs["dispatch_show"]; ok {
			// Use Show trait dispatch
			if err := g.emitAtom(arg); err != nil {
				return err
			}
			g.emitBox(argType)
			g.line("call $dispatch_show")
			g.emitUnbox(wtStringRef)
			g.line("call $__print_str")
			if newline {
				g.line("call $__print_newline")
			}
			return g.emitAtom(arg)
		}
		// Fallback: print a placeholder
		placeholderIdx := g.internString("<value>")
		g.line("(array.new_data $string $d%d (i32.const 0) (i32.const 7))", placeholderIdx)
		g.line("call $__print_str")
		if newline {
			g.line("call $__print_newline")
		}
		return g.emitAtom(arg)
	}
}

// ---------------------------------------------------------------------------
// GC type declarations (closures + ADTs)
// ---------------------------------------------------------------------------

func (g *watGen) emitGCTypes() {
	// All GC types go in one (rec ...) group for mutual references
	g.line(";; GC types")
	g.line("(rec")
	g.indent++

	// Box types for primitives (always emitted, cheap)
	g.line(";; Box types")
	g.line("(type $box_i64 (struct (field $val i64)))")
	g.line("(type $box_f64 (struct (field $val f64)))")

	// Trait dispatch functypes
	if len(g.dispatchFuncs) > 0 {
		g.line(";; Trait dispatch functypes")
		dispNames := make([]string, 0, len(g.dispatchFuncs))
		for name := range g.dispatchFuncs {
			dispNames = append(dispNames, name)
		}
		sortStrings(dispNames)
		for _, name := range dispNames {
			df := g.dispatchFuncs[name]
			params := ""
			for i := 0; i < df.arity; i++ {
				params += " (param (ref null any))"
			}
			g.line("(type $ft_trait_%s (func%s (result (ref null any))))", df.methodName, params)
		}
	}

	// String type
	if g.needsStrings() {
		g.line("(type $string (array (mut i8)))")
	}

	// Closure types — use (ref null any) for param/result/captures
	if g.needsClosures() {
		g.line(";; Closure types")
		g.line("(type $ft_apply (func (param (ref null $closure)) (param (ref null any)) (result (ref null any))))")
		g.line("(type $closure (sub (struct (field $fn (ref $ft_apply)))))")
		for n := 1; n <= g.maxCaps; n++ {
			fields := "(field $fn (ref $ft_apply))"
			for i := 0; i < n; i++ {
				fields += fmt.Sprintf(" (field $c%d (ref null any))", i)
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
			// Per-ADT supertype: allows ref.test to match any constructor of this ADT
			g.line("(type $%s (sub $adt (struct (field $tag i32))))", adtName)
			for _, ci := range ai.ctors {
				if len(ci.fieldTypes) == 0 {
					g.line("(type $%s_%s (sub $%s (struct (field $tag i32))))",
						adtName, ci.name, adtName)
				} else {
					fields := "(field $tag i32)"
					for i, ft := range ci.fieldTypes {
						fields += fmt.Sprintf(" (field $f%d %s)", i, ft)
					}
					g.line("(type $%s_%s (sub $%s (struct %s)))",
						adtName, ci.name, adtName, fields)
				}
			}
		}
	}

	// Record types
	if len(g.records) > 0 {
		g.line(";; Record types")
		recNames := make([]string, 0, len(g.records))
		for name := range g.records {
			recNames = append(recNames, name)
		}
		sortStrings(recNames)
		for _, recName := range recNames {
			ri := g.records[recName]
			fields := ""
			for i, fn := range ri.fieldNames {
				ft := ri.fieldTypes[i]
				if fields != "" {
					fields += " "
				}
				fields += fmt.Sprintf("(field $%s %s)", fn, ft)
			}
			g.line("(type $rec_%s (struct %s))", recName, fields)
		}
	}

	// List types — head is (ref null any) for polymorphism
	if g.usesLists {
		g.line(";; List types")
		g.line("(type $list (sub (struct (field $tag i32))))")
		g.line("(type $list_cons (sub $list (struct (field $tag i32) (field $head (ref null any)) (field $tail (ref null $list)))))")
	}

	// Tuple types — all fields are (ref null any) for polymorphism
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
				fields += fmt.Sprintf("(field $f%d (ref null any))", i)
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
		// For let bindings where the RHS is a lambda, add the name to scope
		// before scanning (enables self-recursive closures from `let rec`).
		// Then post-process to convert self-captures to selfCapture field.
		lambdaIdxBefore := len(g.lambdas)
		bindScope := scope
		bindLet := letBindings
		if _, isLambda := e.Bind.(ir.CLambda); isLambda {
			bindScope = copyScope(scope)
			bindScope[e.Name] = true
			bindLet = copyScope(letBindings)
			bindLet[e.Name] = true
		}
		g.scanCExprForLambdas(owner, e.Bind, bindScope, bindLet)
		// Post-process: if a newly-added lambda captures e.Name, convert to selfCapture
		for i := lambdaIdxBefore; i < len(g.lambdas); i++ {
			lf := &g.lambdas[i]
			for j, cap := range lf.captures {
				if cap == e.Name {
					lf.selfCapture = cap
					// Remove from captures list
					lf.captures = append(lf.captures[:j], lf.captures[j+1:]...)
					lf.capTypes = append(lf.capTypes[:j], lf.capTypes[j+1:]...)
					// Recalculate maxCaps
					if len(lf.captures) > g.maxCaps {
						g.maxCaps = len(lf.captures)
					}
					break
				}
			}
		}
		// Add let-bound name to scope
		newScope := copyScope(scope)
		newScope[e.Name] = true
		newLet := copyScope(letBindings)
		newLet[e.Name] = true
		g.scanExprForLambdas(owner, e.Body, newScope, newLet)
	case ir.ELetRec:
		// All bindings are in scope for each other
		newScope := copyScope(scope)
		newLet := copyScope(letBindings)
		for _, b := range e.Bindings {
			newScope[b.Name] = true
			newLet[b.Name] = true
		}
		// Track which lambda index is the direct (outermost) lambda for each binding
		directLambdaIdx := make(map[int]string) // lambda index → binding name
		for _, b := range e.Bindings {
			idx := len(g.lambdas)
			g.scanCExprForLambdas(owner, b.Bind, newScope, newLet)
			if len(g.lambdas) > idx {
				directLambdaIdx[idx] = b.Name
			}
		}
		// Post-process: only set selfCapture on the DIRECT lambda of each binding,
		// not on inner lambdas (which should capture the binding as a regular capture)
		for i, bindName := range directLambdaIdx {
			lf := &g.lambdas[i]
			for j := 0; j < len(lf.captures); j++ {
				if lf.captures[j] == bindName {
					lf.selfCapture = lf.captures[j]
					lf.captures = append(lf.captures[:j], lf.captures[j+1:]...)
					lf.capTypes = append(lf.capTypes[:j], lf.capTypes[j+1:]...)
					if len(lf.captures) > g.maxCaps {
						g.maxCaps = len(lf.captures)
					}
					break
				}
			}
		}
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
		lambdaName := fmt.Sprintf("$%s__lambda_%d", owner, len(g.lambdas))
		capTypes := make([]string, len(captures))
		for i, capName := range captures {
			// Determine capture type from locals/params
			if t, ok := g.funcParams[capName]; ok {
				capTypes[i] = t
			} else if t, ok := g.locals[capName]; ok {
				capTypes[i] = t
			} else {
				capTypes[i] = wtAnyRef
			}
		}
		// Determine parameter and return types from the lambda's type annotation
		paramType := wtAnyRef
		retType := wtAnyRef
		if e.Ty != nil {
			paramTypes, rt := decomposeFuncType(e.Ty)
			if len(paramTypes) > 0 {
				paramType = g.wasmType(paramTypes[0])
			}
			retType = g.wasmType(rt)
		}
		g.lambdas = append(g.lambdas, lambdaFunc{
			name:      lambdaName,
			captures:  captures,
			capTypes:  capTypes,
			param:     e.Param,
			paramType: paramType,
			retType:   retType,
			body:      e.Body,
		})
		// Scan the lambda body for nested lambdas (e.g., multi-arg lambdas)
		innerScope := copyScope(scope)
		innerScope[e.Param] = true
		for _, cap := range captures {
			innerScope[cap] = true
		}
		g.scanExprForLambdas(lambdaName, e.Body, innerScope, nil)
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
		// Skip top-level functions, constructors, trait methods, dispatch functions
		if _, isFunc := g.funcs[name]; isFunc {
			continue
		}
		if _, isCtor := g.ctorToAdt[name]; isCtor {
			continue
		}
		if _, isDispatch := g.dispatchFuncs[name]; isDispatch {
			continue
		}
		if _, isTrait := g.traitMethods[name]; isTrait {
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
			armBound := copyScope(bound)
			collectPatternBindings(arm.Pat, armBound)
			g.collectFreeVars(arm.Body, armBound, free)
		}
	case ir.CLambda:
		newBound := copyScope(bound)
		newBound[e.Param] = true
		g.collectFreeVars(e.Body, newBound, free)
	}
}

// collectPatternBindings adds all variable names bound by a pattern to the set.
func collectPatternBindings(pat ir.Pattern, bound map[string]bool) {
	switch p := pat.(type) {
	case ir.PVar:
		bound[p.Name] = true
	case ir.PCons:
		collectPatternBindings(p.Head, bound)
		collectPatternBindings(p.Tail, bound)
	case ir.PTuple:
		for _, sub := range p.Pats {
			collectPatternBindings(sub, bound)
		}
	case ir.PCtor:
		for _, sub := range p.Args {
			collectPatternBindings(sub, bound)
		}
	case ir.PRecord:
		for _, f := range p.Fields {
			collectPatternBindings(f.Pat, bound)
		}
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

	// Declare locals (skip names that collide with params)
	for _, name := range g.localOrder(fi.body) {
		if _, isParam := g.funcParams[name]; isParam {
			continue
		}
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

	// Lambda function signature: (self: ref $closure, param: anyref) -> anyref
	closureType := "$closure"
	if len(lf.captures) > 0 {
		closureType = fmt.Sprintf("$closure_%d", len(lf.captures))
	}

	// Register captures and param as available locals
	for i, cap := range lf.captures {
		g.locals[cap] = lf.capTypes[i]
	}
	if lf.selfCapture != "" {
		g.locals[lf.selfCapture] = "(ref null $closure)"
	}
	// The raw param from $ft_apply is anyref; we unbox it into a concrete local
	g.funcParams[lf.param] = lf.paramType

	// Collect locals from body
	g.collectLocals(lf.body)

	g.line("(func %s (type $ft_apply) (param $self (ref null $closure)) (param $%s__raw (ref null any)) (result (ref null any))", lf.name, lf.param)
	g.indent++

	// Declare ALL locals first (WAT requires locals before any instructions)
	g.line("(local $%s %s)", lf.param, lf.paramType)
	for i, cap := range lf.captures {
		g.line("(local $%s %s)", cap, lf.capTypes[i])
	}
	for _, name := range g.localOrder(lf.body) {
		if !containsStr(lf.captures, name) && name != lf.param && name != lf.selfCapture {
			g.line("(local $%s %s)", name, g.locals[name])
		}
	}
	// Declare selfCapture local if present
	if lf.selfCapture != "" {
		g.line("(local $%s (ref null $closure))", lf.selfCapture)
	}

	// Unbox the raw anyref param into the concrete-typed local
	g.line("(local.set $%s", lf.param)
	g.indent++
	g.line("(local.get $%s__raw)", lf.param)
	g.emitUnbox(lf.paramType)
	g.indent--
	g.line(")")

	// Extract captures from closure struct — unbox from anyref to concrete type
	for i, cap := range lf.captures {
		g.line("(local.set $%s", cap)
		g.indent++
		g.line("(struct.get %s $c%d (ref.cast (ref %s) (local.get $self)))", closureType, i, closureType)
		g.emitUnbox(lf.capTypes[i])
		g.indent--
		g.line(")")
	}

	// For self-recursive closures, $self IS the closure itself
	if lf.selfCapture != "" {
		g.line("(local.set $%s (local.get $self))", lf.selfCapture)
	}

	if err := g.emitExpr(lf.body); err != nil {
		return fmt.Errorf("codegen lambda %s: %w", lf.name, err)
	}

	// Box result to anyref
	bodyType := g.typeOfExpr(lf.body)
	g.emitBox(bodyType)

	g.indent--
	g.line(")")
	return nil
}

// ---------------------------------------------------------------------------
// Partial application wrappers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Type conversion helpers
// ---------------------------------------------------------------------------

func (g *watGen) emitConvert(from, to string) {
	if from == to {
		return
	}
	// Boxing: concrete type → anyref
	if to == wtAnyRef {
		g.emitBox(from)
		return
	}
	// Unboxing: anyref → concrete type
	if from == wtAnyRef {
		g.emitUnbox(to)
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
	g.collectLocalsWithCtx(expr, nil, nil, nil)
}

// collectLocalsWithCtx collects local variable types, tracking partial constructor
// and trait method application chains to determine final result types.
func (g *watGen) collectLocalsWithCtx(expr ir.Expr, partialCtors map[string]*ctorInfo, partialTraits map[string]*funcInfo, partialRecCtors map[string]*recordInfo) {
	if partialCtors == nil {
		partialCtors = make(map[string]*ctorInfo)
	}
	if partialTraits == nil {
		partialTraits = make(map[string]*funcInfo)
	}
	if partialRecCtors == nil {
		partialRecCtors = make(map[string]*recordInfo)
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
				// Record constructor (positional): first arg of multi-field record ctor
				if ri, ok := g.records[fvar.Name]; ok && len(ri.fieldNames) > 1 {
					partialRecCtors[e.Name] = ri
				}
				// Continuing a partial record ctor chain
				if ri, ok := partialRecCtors[fvar.Name]; ok {
					recType := fmt.Sprintf("(ref null $rec_%s)", ri.name)
					localType = recType
					partialRecCtors[e.Name] = ri // keep propagating
				}
				// Detect trait method partial application chains
				if tmInfo, ok := g.traitMethods[fvar.Name]; ok && !g.isLocalOrParam(fvar.Name) {
					argType := g.rexTypeName(app.Arg)
					if argType != "" {
						implName := tmInfo.traitName + "_" + argType + "_" + tmInfo.methodName
						if fi, ok := g.funcs[implName]; ok && fi.arity > 1 {
							partialTraits[e.Name] = fi
							localType = fi.retType // saturated call result type
						}
					} else {
						// Fallback: dispatch function
						dispName := "dispatch_" + tmInfo.methodName
						if fi, ok := g.funcs[dispName]; ok && fi.arity > 1 {
							partialTraits[e.Name] = fi
							localType = wtAnyRef // dispatch returns anyref
						}
					}
				}
				// If applying a temp that was a partial trait method app
				if fi, ok := partialTraits[fvar.Name]; ok {
					partialTraits[e.Name] = fi
					localType = fi.retType
				}
			}
		}
		g.locals[e.Name] = localType
		g.collectLocalsCExpr(e.Bind)
		g.collectLocalsWithCtx(e.Body, partialCtors, partialTraits, partialRecCtors)
	case ir.ELetRec:
		for _, b := range e.Bindings {
			localType := g.typeOfCExpr(b.Bind)
			g.locals[b.Name] = localType
			g.collectLocalsCExpr(b.Bind)
		}
		g.collectLocalsWithCtx(e.Body, partialCtors, partialTraits, partialRecCtors)
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
			// Default to anyref for pattern variables; override for ADT fields
			g.locals[p.Name] = wtAnyRef
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
		// head is anyref (from polymorphic list storage) — unboxing happens at use
		if pv, ok := p.Head.(ir.PVar); ok {
			if _, exists := g.locals[pv.Name]; !exists {
				g.locals[pv.Name] = wtAnyRef
			}
		}
		if pv, ok := p.Tail.(ir.PVar); ok {
			g.locals[pv.Name] = wtListRef
		}
		g.collectPatternLocals(p.Head)
		g.collectPatternLocals(p.Tail)
	case ir.PTuple:
		// Tuple fields are anyref (from polymorphic tuple storage) — unboxing happens at use
		for _, sub := range p.Pats {
			if pv, ok := sub.(ir.PVar); ok {
				if _, exists := g.locals[pv.Name]; !exists {
					g.locals[pv.Name] = wtAnyRef
				}
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
	case ir.ELetRec:
		for _, b := range e.Bindings {
			if !seen[b.Name] {
				seen[b.Name] = true
				*names = append(*names, b.Name)
			}
			g.collectLocalOrderCExpr(b.Bind, names, seen)
		}
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
		return wtAnyRef
	default:
		return wtAnyRef
	}
}

func (g *watGen) typeOfCExpr(c ir.CExpr) string {
	switch e := c.(type) {
	case ir.CApp:
		// Identity function: __id x → type of x
		if v, ok := e.Func.(ir.AVar); ok && v.Name == "__id" {
			return g.typeOfAtom(e.Arg)
		}
		// Check if calling a builtin
		if v, ok := e.Func.(ir.AVar); ok && knownBuiltins[v.Name] && !g.isLocalOrParam(v.Name) {
			return g.typeOfBuiltinApp(v.Name, e.Arg)
		}
		// Check if calling a constructor
		if v, ok := e.Func.(ir.AVar); ok {
			if _, ok := g.ctorToAdt[v.Name]; ok {
				return wtAdtRef
			}
			// Trait method dispatch: infer return type from impl function
			if tmInfo, ok := g.traitMethods[v.Name]; ok && !g.isLocalOrParam(v.Name) {
				argType := g.rexTypeName(e.Arg)
				if argType != "" {
					implName := tmInfo.traitName + "_" + argType + "_" + tmInfo.methodName
					if fi, ok := g.funcs[implName]; ok {
						if fi.arity == 1 {
							return fi.retType
						}
						return wtRef
					}
				}
				// Fallback: dispatch function returns anyref
				dispName := "dispatch_" + tmInfo.methodName
				if df, ok := g.dispatchFuncs[dispName]; ok {
					if df.arity == 1 {
						return wtAnyRef
					}
					return wtRef // partial application returns closure
				}
			}
			if fi, ok := g.funcs[v.Name]; ok {
				if fi.arity == 1 {
					return fi.retType
				}
				// Partial application returns a closure
				return wtRef
			}
		}
		// call_ref returns anyref (from $ft_apply)
		return wtAnyRef
	case ir.CBinop:
		switch e.Op {
		case "Add", "Sub", "Mul", "Div", "Mod":
			// After unboxing, arithmetic result is concrete
			leftType := g.typeOfAtom(e.Left)
			rightType := g.typeOfAtom(e.Right)
			if leftType == wtAnyRef {
				if rightType == wtAnyRef {
					return wtI64 // default arithmetic type
				}
				return rightType
			}
			return leftType
		case "Eq", "Neq", "Lt", "Gt", "Leq", "Geq", "And", "Or":
			return wtI32
		case "Concat":
			return wtStringRef
		case "Cons":
			return wtListRef
		}
	case ir.CUnaryMinus:
		return g.typeOfAtom(e.Expr)
	case ir.CIf:
		thenType := g.typeOfExpr(e.Then)
		elseType := g.typeOfExpr(e.Else)
		if thenType != elseType {
			// Types differ: use function return type if available, else widen
			if g.currentFunc != nil {
				return g.currentFunc.retType
			}
			if isRefType(thenType) && isRefType(elseType) {
				return wtAnyRef
			}
		}
		return thenType
	case ir.CMatch:
		if len(e.Arms) > 0 {
			// Use widest type across all arms
			resultType := g.typeOfExpr(e.Arms[0].Body)
			for _, arm := range e.Arms[1:] {
				armType := g.typeOfExpr(arm.Body)
				if armType != resultType {
					// Types differ: for ref types use anyref, else prefer concrete
					if isRefType(resultType) && isRefType(armType) {
						return wtAnyRef
					} else if isRefType(armType) {
						// resultType is concrete (e.g., i64), armType is ref (e.g., anyref from tail call)
						// Keep concrete type — the ref arm likely uses return_call
						continue
					} else if isRefType(resultType) {
						// armType is concrete
						resultType = armType
					} else {
						// Both concrete but different — unexpected
						return wtAnyRef
					}
				}
			}
			return resultType
		}
	case ir.CLambda:
		return wtRef
	case ir.CList:
		return wtListRef
	case ir.CTuple:
		return wtTupleRef(len(e.Items))
	case ir.CStringInterp:
		return wtStringRef
	case ir.CRecord:
		return fmt.Sprintf("(ref null $rec_%s)", e.TypeName)
	case ir.CFieldAccess:
		if ri, ok := g.records[g.recordTypeOfAtom(e.Record)]; ok {
			for i, fn := range ri.fieldNames {
				if fn == e.Field {
					return ri.fieldTypes[i]
				}
			}
		}
		return wtAnyRef
	case ir.CRecordUpdate:
		return fmt.Sprintf("(ref null $rec_%s)", g.recordTypeOfAtom(e.Record))
	}
	return wtAnyRef
}

// recordTypeOfAtom returns the record type name for an atom, or "" if unknown.
func (g *watGen) recordTypeOfAtom(a ir.Atom) string {
	if v, ok := a.(ir.AVar); ok {
		wt := g.locals[v.Name]
		if wt == "" {
			if pt, ok := g.funcParams[v.Name]; ok {
				wt = pt
			}
		}
		// Extract record type name from "(ref null $rec_Foo)"
		if strings.HasPrefix(wt, "(ref null $rec_") {
			return wt[len("(ref null $rec_") : len(wt)-1]
		}
	}
	return ""
}

func (g *watGen) typeOfExpr(expr ir.Expr) string {
	switch e := expr.(type) {
	case ir.EAtom:
		return g.typeOfAtom(e.A)
	case ir.EComplex:
		return g.typeOfCExpr(e.C)
	case ir.ELet:
		return g.typeOfExpr(e.Body)
	case ir.ELetRec:
		return g.typeOfExpr(e.Body)
	}
	return wtAnyRef
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
	case ir.ELetRec:
		return g.emitLetRecTail(e, tail)
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
	if fi.arity == 1 {
		wrapperName, exists := g.funcWrappers[fi.name]
		if !exists {
			wrapperName = fmt.Sprintf("$%s__wrap", fi.name)
			g.funcWrappers[fi.name] = wrapperName
		}
		g.funcRefs[wrapperName] = true
		g.line("(struct.new $closure (ref.func %s))", wrapperName)
		return nil
	}

	// Multi-arg function used as value: create a closure whose apply function
	// performs the first partial application (depth=0 → receives first arg → pa1).
	paKey := fmt.Sprintf("%s__pa0", fi.name)
	g.partialApps[paKey] = true
	wrapperName := "$" + paKey
	g.funcRefs[wrapperName] = true
	g.line("(struct.new $closure (ref.func %s))", wrapperName)
	return nil
}

// emitPartialApp creates a closure that captures `depth` args for a multi-arg function.
// depth=1 means one arg has been applied; the closure captures it and waits for more.
func (g *watGen) emitPartialApp(fi *funcInfo, arg ir.Atom, depth int) error {
	// Register that we need a partial application wrapper for this function at this depth
	paKey := fmt.Sprintf("%s__pa%d", fi.name, depth)
	g.partialApps[paKey] = true
	wrapperName := "$" + paKey

	// Ensure we have enough closure captures
	if depth > g.maxCaps {
		g.maxCaps = depth
	}

	// Create closure_N with captured args
	// For depth=1: struct.new $closure_1 (ref.func $f__pa1) (box arg)
	g.funcRefs[wrapperName] = true
	g.line("(struct.new $closure_%d", depth)
	g.indent++
	g.line("(ref.func %s)", wrapperName)
	if err := g.emitAtom(arg); err != nil {
		return err
	}
	g.emitBox(g.typeOfAtom(arg))
	g.indent--
	g.line(")")
	return nil
}

// emitPartialAppWrapper emits a single partial application wrapper function.
// For depth=1, arity=2: extracts captured arg, calls $f(captured, newArg)
// For depth=1, arity=3: extracts captured arg, creates closure_2 with both args
// For depth=2, arity=3: extracts both captured args, calls $f(c0, c1, newArg)
func (g *watGen) emitPartialAppWrapper(fi *funcInfo, depth int, paKey string) {
	wrapperName := "$" + paKey
	g.line("(func %s (type $ft_apply) (param $self (ref null $closure)) (param $arg (ref null any)) (result (ref null any))",
		wrapperName)
	g.indent++

	remaining := fi.arity - depth
	if remaining == 1 {
		// Final application: extract all captured args and call the function
		// Cast self to the right closure type to access captures
		g.line("(local $self_cast (ref null $closure_%d))", depth)
		g.line("(local.set $self_cast (ref.cast (ref null $closure_%d) (local.get $self)))", depth)

		// Emit all captured args (c0, c1, ...)
		for i := 0; i < depth; i++ {
			g.line("(local.get $self_cast)")
			g.line("(struct.get $closure_%d $c%d)", depth, i)
			// Unbox captured arg to the expected param type
			g.emitUnbox(fi.params[i].wasmType)
		}

		// Emit the new arg (last param)
		g.line("(local.get $arg)")
		g.emitUnbox(fi.params[depth].wasmType)

		// Call the function
		g.line("(call $%s)", fi.name)
		// Box the result to anyref
		g.emitBox(fi.retType)
	} else {
		// Need more args: create a closure with one more capture
		newDepth := depth + 1
		nextPaKey := fmt.Sprintf("%s__pa%d", fi.name, newDepth)
		g.partialApps[nextPaKey] = true
		nextWrapper := "$" + nextPaKey

		if newDepth > g.maxCaps {
			g.maxCaps = newDepth
		}

		// Cast self to access existing captures (if any)
		if depth > 0 {
			g.line("(local $self_cast (ref null $closure_%d))", depth)
			g.line("(local.set $self_cast (ref.cast (ref null $closure_%d) (local.get $self)))", depth)
		}

		// Create new closure with all previous captures + the new arg
		g.funcRefs[nextWrapper] = true
		g.line("(struct.new $closure_%d", newDepth)
		g.indent++
		g.line("(ref.func %s)", nextWrapper)
		// Copy existing captures
		for i := 0; i < depth; i++ {
			g.line("(local.get $self_cast)")
			g.line("(struct.get $closure_%d $c%d)", depth, i)
		}
		// Add new arg
		g.line("(local.get $arg)")
		g.indent--
		g.line(")")
	}

	g.indent--
	g.line(")")
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
	case ir.CStringInterp:
		return g.emitStringInterp(e)
	case ir.CRecord:
		return g.emitRecord(e)
	case ir.CFieldAccess:
		return g.emitFieldAccess(e)
	case ir.CRecordUpdate:
		return g.emitRecordUpdate(e)
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

	// Identity function: __id x → just emit x
	if v.Name == "__id" {
		return g.emitAtom(e.Arg)
	}

	// Builtin intrinsics
	if knownBuiltins[v.Name] && !g.isLocalOrParam(v.Name) {
		return g.emitBuiltinApp(v.Name, e.Arg)
	}

	// Trait method dispatch (arity 1): redirect to impl function
	if tmInfo, ok := g.traitMethods[v.Name]; ok && !g.isLocalOrParam(v.Name) {
		argType := g.rexTypeName(e.Arg)
		if argType != "" {
			implName := tmInfo.traitName + "_" + argType + "_" + tmInfo.methodName
			if fi, ok := g.funcs[implName]; ok && fi.arity == 1 {
				if err := g.emitAtom(e.Arg); err != nil {
					return err
				}
				at := g.typeOfAtom(e.Arg)
				if at != fi.params[0].wasmType {
					g.emitConvert(at, fi.params[0].wasmType)
				}
				canTailCall := tail && g.currentFunc != nil && fi.retType == g.currentFunc.retType
				if canTailCall {
					g.line("return_call $%s", fi.name)
				} else {
					g.line("call $%s", fi.name)
				}
				return nil
			}
		}
		// Fallback: runtime dispatch when type is unknown
		dispName := "dispatch_" + tmInfo.methodName
		if df, ok := g.dispatchFuncs[dispName]; ok && df.arity == 1 {
			if err := g.emitAtom(e.Arg); err != nil {
				return err
			}
			at := g.typeOfAtom(e.Arg)
			g.emitConvert(at, wtAnyRef)
			g.line("call $%s", dispName)
			return nil
		}
	}

	// Record constructor application (single-field records, positional)
	if ri, ok := g.records[v.Name]; ok && len(ri.fieldNames) == 1 {
		g.line("(struct.new $rec_%s", ri.name)
		g.indent++
		if err := g.emitAtom(e.Arg); err != nil {
			return err
		}
		g.indent--
		g.line(")")
		return nil
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
			// Box field if the struct expects anyref
			if ci.fieldTypes[0] == wtAnyRef {
				g.emitBox(g.typeOfAtom(e.Arg))
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
			// Convert arg type to match param type
			argType := g.typeOfAtom(e.Arg)
			if argType != fi.params[0].wasmType {
				g.emitConvert(argType, fi.params[0].wasmType)
			}
			// Only use return_call if callee's return type matches caller's
			canTailCall := tail && g.currentFunc != nil && fi.retType == g.currentFunc.retType
			if canTailCall {
				g.line("return_call $%s", fi.name)
			} else {
				g.line("call $%s", fi.name)
			}
			return nil
		}
		// Partial application: create a closure that captures the first arg.
		// When called later with the next arg, the wrapper either calls the
		// function directly (arity 2) or creates another partial application.
		return g.emitPartialApp(fi, e.Arg, 1)
	}

	// Call through closure reference (the variable holds a ref $closure)
	if err := g.emitCallRef(v.Name, e.Arg); err != nil {
		return err
	}
	return nil
}

func (g *watGen) emitCallRef(closureVar string, arg ir.Atom) error {
	// call_ref $ft_apply: stack = [self, arg, funcref]
	varType := g.locals[closureVar]
	if varType == "" {
		varType = g.funcParams[closureVar]
	}
	needsCast := varType == wtAnyRef
	// Push self (cast to closure if needed)
	g.line("(local.get $%s)", closureVar)
	if needsCast {
		g.line("(ref.cast (ref $closure))")
	}
	if err := g.emitAtom(arg); err != nil {
		return err
	}
	// Box argument to anyref for $ft_apply convention
	g.emitBox(g.typeOfAtom(arg))
	// Get funcref (cast to closure if needed)
	g.line("(local.get $%s)", closureVar)
	if needsCast {
		g.line("(ref.cast (ref $closure))")
	}
	g.line("(struct.get $closure $fn)")
	g.line("(call_ref $ft_apply)")
	// Result is anyref — will be unboxed by the caller if needed
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
			// Trait method dispatch (arity > 1): redirect to impl function
			// e.g. let _t = eq x in _t y → call $Eq_Int_eq x y
			if tmInfo, ok := g.traitMethods[fvar.Name]; ok && !g.isLocalOrParam(fvar.Name) {
				argType := g.rexTypeName(app.Arg)
				if argType != "" {
					implName := tmInfo.traitName + "_" + argType + "_" + tmInfo.methodName
					if fi, ok := g.funcs[implName]; ok && fi.arity > 1 {
						if result, ok := g.trySaturatedCallTail(fi, []ir.Atom{app.Arg}, e.Name, e.Body, tail); ok {
							return result
						}
					}
				}
				// Fallback: runtime dispatch when type is unknown
				dispName := "dispatch_" + tmInfo.methodName
				if fi, ok := g.funcs[dispName]; ok && fi.arity > 1 {
					if result, ok := g.trySaturatedCallTail(fi, []ir.Atom{app.Arg}, e.Name, e.Body, tail); ok {
						return result
					}
				}
			}
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
			// Detect saturated record constructor applications (positional)
			if ri, ok := g.records[fvar.Name]; ok && len(ri.fieldNames) > 1 {
				if result, ok := g.trySaturatedRecordCtor(ri, []ir.Atom{app.Arg}, e.Name, e.Body); ok {
					return result
				}
			}
		}
	}

	// Normal let: emit bind, set local, emit body
	if err := g.emitCExpr(e.Bind); err != nil {
		return err
	}
	// If name collides with a param, drop the value (can't shadow param as local)
	if _, isParam := g.funcParams[e.Name]; isParam {
		g.line("drop")
	} else {
		g.line("local.set $%s", e.Name)
	}
	return g.emitExprTail(e.Body, tail)
}

func (g *watGen) emitLetRecTail(e ir.ELetRec, tail bool) error {
	// ELetRec bindings are typically CLambda (local recursive functions).
	// Emit each binding's closure, store in its local, then emit the body.
	for _, b := range e.Bindings {
		if err := g.emitCExpr(b.Bind); err != nil {
			return err
		}
		g.line("local.set $%s", b.Name)
	}
	return g.emitExprTail(e.Body, tail)
}

// trySaturatedCallTail detects chains of let-bound partial applications that
// resolve to a saturated direct call. Uses return_call in tail position.
func (g *watGen) trySaturatedCallTail(fi *funcInfo, args []ir.Atom, tempName string, body ir.Expr, tail bool) (error, bool) {
	if len(args) == fi.arity {
		// Saturated! Emit direct call
		for i, arg := range args {
			if err := g.emitAtom(arg); err != nil {
				return err, true
			}
			// Convert arg type to match the function's param type
			argType := g.typeOfAtom(arg)
			paramType := fi.params[i].wasmType
			if argType != paramType {
				g.emitConvert(argType, paramType)
			}
		}
		canTailCall := tail && g.currentFunc != nil && fi.retType == g.currentFunc.retType
		if canTailCall {
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
					for i, arg := range newArgs {
						if err := g.emitAtom(arg); err != nil {
							return err, true
						}
						argType := g.typeOfAtom(arg)
						paramType := fi.params[i].wasmType
						if argType != paramType {
							g.emitConvert(argType, paramType)
						}
					}
					g.line("call $%s", fi.name)
					// Convert result to match local type if needed
					if localType, ok := g.locals[let.Name]; ok && fi.retType != localType {
						g.emitConvert(fi.retType, localType)
					}
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
		for i, arg := range args {
			if err := g.emitAtom(arg); err != nil {
				return err, true
			}
			// Box field if the struct expects anyref
			if i < len(ci.fieldTypes) && ci.fieldTypes[i] == wtAnyRef {
				g.emitBox(g.typeOfAtom(arg))
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
					for i, arg := range newArgs {
						if err := g.emitAtom(arg); err != nil {
							return err, true
						}
						// Box field if the struct expects anyref
						if i < len(ci.fieldTypes) && ci.fieldTypes[i] == wtAnyRef {
							g.emitBox(g.typeOfAtom(arg))
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

// trySaturatedRecordCtor detects chains of let-bound partial record constructor applications.
func (g *watGen) trySaturatedRecordCtor(ri *recordInfo, args []ir.Atom, tempName string, body ir.Expr) (error, bool) {
	nFields := len(ri.fieldNames)
	if len(args) == nFields {
		// Saturated! Emit struct.new
		recType := fmt.Sprintf("(ref null $rec_%s)", ri.name)
		g.line("(struct.new $rec_%s", ri.name)
		g.indent++
		for _, arg := range args {
			if err := g.emitAtom(arg); err != nil {
				return err, true
			}
		}
		g.indent--
		g.line(")")
		_ = recType
		return nil, true
	}

	// Check if body is: ELet{newTemp, CApp{tempName, nextArg}, restBody}
	if let, ok := body.(ir.ELet); ok {
		if app, ok := let.Bind.(ir.CApp); ok {
			if v, ok := app.Func.(ir.AVar); ok && v.Name == tempName {
				newArgs := append(args, app.Arg)
				if len(newArgs) == nFields {
					// Saturated! Emit struct.new, then set local and continue
					recType := fmt.Sprintf("(ref null $rec_%s)", ri.name)
					g.line("(struct.new $rec_%s", ri.name)
					g.indent++
					for _, arg := range newArgs {
						if err := g.emitAtom(arg); err != nil {
							return err, true
						}
					}
					g.indent--
					g.line(")")
					// Override local type
					g.locals[let.Name] = recType
					g.line("local.set $%s", let.Name)
					err := g.emitExpr(let.Body)
					return err, true
				}
				return g.trySaturatedRecordCtor(ri, newArgs, let.Name, let.Body)
			}
		}
	}

	// Check if body is: EComplex{CApp{tempName, nextArg}}
	if ec, ok := body.(ir.EComplex); ok {
		if app, ok := ec.C.(ir.CApp); ok {
			if v, ok := app.Func.(ir.AVar); ok && v.Name == tempName {
				return g.trySaturatedRecordCtor(ri, append(args, app.Arg), tempName, nil)
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
		// Only use as scrutinee name if it's a local/param, not a constructor
		if _, isCtor := g.ctorToAdt[v.Name]; !isCtor {
			scrutName = v.Name
		}
	}

	// Determine the number of arms
	numArms := len(e.Arms)
	if numArms == 0 {
		return fmt.Errorf("codegen: empty match")
	}

	// Emit a block-based pattern match:
	// Get tag, then use nested if/else
	openBlocks := 0
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
				openBlocks++

				// Bind pattern variables by extracting fields
				if err := g.emitPatternBindings(scrut, scrutName, ci, pat.Args); err != nil {
					return err
				}
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
					return err
				}

				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				// Last arm: no condition check (catch-all or final branch)
				if err := g.emitPatternBindings(scrut, scrutName, ci, pat.Args); err != nil {
					return err
				}
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
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
			if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
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
				openBlocks++
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
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
				openBlocks++
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
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
				openBlocks++
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
					return err
				}
			}

		case ir.PNil:
			// Empty list: tag == 0
			if !isLast {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				scrutType := g.typeOfAtom(scrut)
				if scrutType == wtAnyRef {
					g.line("(ref.cast (ref $list))")
				}
				g.line("(struct.get $list $tag)")
				g.line("i32.eqz")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
				openBlocks++
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
					return err
				}
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			} else {
				if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
					return err
				}
			}

		case ir.PCons:
			// Cons cell: tag == 1, bind head and tail
			if !isLast {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				scrutType := g.typeOfAtom(scrut)
				if scrutType == wtAnyRef {
					g.line("(ref.cast (ref $list))")
				}
				g.line("(struct.get $list $tag)")
				g.line("(i32.const 1)")
				g.line("i32.eq")
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
				openBlocks++
			}
			// Bind head if PVar — unbox from anyref to concrete type
			if pv, ok := pat.Head.(ir.PVar); ok {
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("(struct.get $list_cons $head (ref.cast (ref $list_cons)))")
				headType := g.locals[pv.Name]
				g.emitUnbox(headType)
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
			if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
				return err
			}
			if !isLast {
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			}

		case ir.PTuple:
			// Tuple pattern: may have nested patterns (PCtor, PWild, PVar, etc.)
			arity := len(pat.Pats)

			// Check if this tuple pattern has any nested constructor patterns
			hasNestedCtors := false
			for _, subPat := range pat.Pats {
				if _, ok := subPat.(ir.PCtor); ok {
					hasNestedCtors = true
					break
				}
			}

			if hasNestedCtors && !isLast {
				// Build condition: all nested ctor tags must match
				// First, emit the combined condition
				conditions := 0
				for fi, subPat := range pat.Pats {
					if cp, ok := subPat.(ir.PCtor); ok {
						ci, ok := g.ctorToAdt[cp.Name]
						if !ok {
							return fmt.Errorf("codegen: unknown constructor %s in tuple pattern", cp.Name)
						}
						if err := g.emitAtom(scrut); err != nil {
							return err
						}
						g.line("(struct.get $tuple%d $f%d)", arity, fi)
						g.line("(ref.cast (ref $adt))")
						g.line("(struct.get $adt $tag)")
						g.line("(i32.const %d)", ci.tag)
						g.line("i32.eq")
						conditions++
					}
				}
				// AND all conditions together
				for c := 1; c < conditions; c++ {
					g.line("i32.and")
				}
				g.line("(if (result %s)", resultType)
				g.indent++
				g.line("(then")
				g.indent++
				openBlocks++
			}

			// Bind pattern variables by extracting fields
			for fi, subPat := range pat.Pats {
				switch sp := subPat.(type) {
				case ir.PVar:
					if err := g.emitAtom(scrut); err != nil {
						return err
					}
					g.line("(struct.get $tuple%d $f%d)", arity, fi)
					fieldType := g.locals[sp.Name]
					g.emitUnbox(fieldType)
					g.line("local.set $%s", sp.Name)
				case ir.PCtor:
					// Extract constructor fields
					ci, ok := g.ctorToAdt[sp.Name]
					if !ok {
						return fmt.Errorf("codegen: unknown constructor %s in tuple pattern", sp.Name)
					}
					for ai, argPat := range sp.Args {
						if pv, ok := argPat.(ir.PVar); ok {
							if err := g.emitAtom(scrut); err != nil {
								return err
							}
							g.line("(struct.get $tuple%d $f%d)", arity, fi)
							g.line("(ref.cast (ref $%s_%s))", ci.typeName, ci.name)
							g.line("(struct.get $%s_%s $f%d)", ci.typeName, ci.name, ai)
							fieldType := g.locals[pv.Name]
							g.emitUnbox(fieldType)
							g.line("local.set $%s", pv.Name)
						}
					}
				case ir.PWild:
					// Nothing to bind
				}
			}
			if err := g.emitMatchArmBody(arm.Body, resultType, tail); err != nil {
				return err
			}
			if hasNestedCtors && !isLast {
				g.indent--
				g.line(")")
				g.line("(else")
				g.indent++
			}

		default:
			return fmt.Errorf("codegen: unsupported pattern type %T", pat)
		}
	}

	// Close all the if/else blocks
	for i := 0; i < openBlocks; i++ {
		g.indent--
		g.line(")")
		g.indent--
		g.line(")")
	}

	return nil
}

// emitMatchArmBody emits the body of a match arm and inserts unboxing
// if the body's type doesn't match the match result type.
// This handles polymorphic ADT pattern variables (typed as anyref)
// that need to be unboxed to concrete types (i64, f64, i32).
func (g *watGen) emitMatchArmBody(body ir.Expr, resultType string, tail bool) error {
	if err := g.emitExprTail(body, tail); err != nil {
		return err
	}
	bodyType := g.typeOfExpr(body)
	if bodyType != resultType && bodyType == wtAnyRef && !isRefType(resultType) {
		g.emitUnbox(resultType)
	}
	return nil
}

// emitPatternBindings extracts fields from an ADT struct and binds them
// to the pattern variables.
func (g *watGen) emitPatternBindings(scrut ir.Atom, scrutName string, ci *ctorInfo, pats []ir.Pattern) error {
	for i, pat := range pats {
		switch p := pat.(type) {
		case ir.PVar:
			// Extract field i from the constructor struct
			ctorType := fmt.Sprintf("$%s_%s", ci.typeName, ci.name)
			if scrutName != "" {
				g.line("(struct.get %s $f%d (ref.cast (ref %s) (local.get $%s)))",
					ctorType, i, ctorType, scrutName)
			} else {
				// Scrutinee isn't a local variable — re-emit the atom
				if err := g.emitAtom(scrut); err != nil {
					return err
				}
				g.line("(ref.cast (ref %s))", ctorType)
				g.line("(struct.get %s $f%d)", ctorType, i)
			}
			// Unbox from anyref if needed
			fieldType := wtAnyRef
			if i < len(ci.fieldTypes) {
				fieldType = ci.fieldTypes[i]
			}
			localType := g.locals[p.Name]
			if fieldType == wtAnyRef && localType != wtAnyRef {
				g.emitUnbox(localType)
			}
			g.line("local.set $%s", p.Name)
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
		// Box the element to anyref for polymorphic storage
		g.emitBox(g.typeOfAtom(e.Items[i]))
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
		// Box the element to anyref for polymorphic storage
		g.emitBox(g.typeOfAtom(item))
	}
	g.indent--
	g.line(")")
	return nil
}

// ---------------------------------------------------------------------------
// Records
// ---------------------------------------------------------------------------

func (g *watGen) emitRecord(e ir.CRecord) error {
	ri, ok := g.records[e.TypeName]
	if !ok {
		return fmt.Errorf("codegen: unknown record type %s", e.TypeName)
	}
	g.line("(struct.new $rec_%s", e.TypeName)
	g.indent++
	// Emit fields in declaration order
	for _, fn := range ri.fieldNames {
		// Find the matching field init
		found := false
		for _, fi := range e.Fields {
			if fi.Name == fn {
				if err := g.emitAtom(fi.Value); err != nil {
					return err
				}
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("codegen: missing field %s in record %s", fn, e.TypeName)
		}
	}
	g.indent--
	g.line(")")
	return nil
}

func (g *watGen) emitFieldAccess(e ir.CFieldAccess) error {
	recType := g.recordTypeOfAtom(e.Record)
	if recType == "" {
		return fmt.Errorf("codegen: cannot determine record type for field access .%s", e.Field)
	}
	if err := g.emitAtom(e.Record); err != nil {
		return err
	}
	g.line("(struct.get $rec_%s $%s)", recType, e.Field)
	return nil
}

func (g *watGen) emitRecordUpdate(e ir.CRecordUpdate) error {
	recType := g.recordTypeOfAtom(e.Record)
	if recType == "" {
		return fmt.Errorf("codegen: cannot determine record type for record update")
	}
	ri, ok := g.records[recType]
	if !ok {
		return fmt.Errorf("codegen: unknown record type %s", recType)
	}
	// Build a set of updated field names
	updates := make(map[string]ir.Atom)
	for _, u := range e.Updates {
		if len(u.Path) == 1 {
			updates[u.Path[0]] = u.Value
		}
		// Nested updates not yet supported in wasm codegen
	}
	g.line("(struct.new $rec_%s", recType)
	g.indent++
	for _, fn := range ri.fieldNames {
		if val, ok := updates[fn]; ok {
			// Use the updated value
			if err := g.emitAtom(val); err != nil {
				return err
			}
		} else {
			// Copy the original field
			if err := g.emitAtom(e.Record); err != nil {
				return err
			}
			g.line("(struct.get $rec_%s $%s)", recType, fn)
		}
	}
	g.indent--
	g.line(")")
	return nil
}

// ---------------------------------------------------------------------------
// Closures
// ---------------------------------------------------------------------------

// countFreeVarsInLambda counts the free variables in a lambda that would
// become captures when lifted. Used to disambiguate lambdas with the same param name.
func (g *watGen) countFreeVarsInLambda(lam ir.CLambda) int {
	free := make(map[string]bool)
	g.collectFreeVars(lam.Body, map[string]bool{lam.Param: true}, free)
	count := 0
	for name := range free {
		if _, isCtor := g.ctorToAdt[name]; isCtor {
			continue
		}
		if _, isFunc := g.funcs[name]; isFunc {
			continue
		}
		// Skip trait dispatch functions and trait methods (globally available)
		if _, isDispatch := g.dispatchFuncs[name]; isDispatch {
			continue
		}
		if _, isTrait := g.traitMethods[name]; isTrait {
			continue
		}
		count++
	}
	return count
}

func (g *watGen) emitClosureCreate(lam ir.CLambda) error {
	// Find the corresponding lifted lambda.
	// Match by param name + number of captures.
	expectedCaptures := g.countFreeVarsInLambda(lam)
	for i := range g.lambdas {
		lf := &g.lambdas[i]
		if lf.used {
			continue
		}
		// Self-recursive closures have selfCapture removed from captures list
		effectiveCaptures := len(lf.captures)
		if lf.selfCapture != "" {
			effectiveCaptures++
		}
		if lf.param == lam.Param && effectiveCaptures == expectedCaptures {
			lf.used = true
			closureType := "$closure"
			nCaps := len(lf.captures)
			if nCaps > 0 {
				closureType = fmt.Sprintf("$closure_%d", nCaps)
			}

			g.line("(struct.new %s", closureType)
			g.indent++
			g.funcRefs[lf.name] = true
			g.line("(ref.func %s)", lf.name)
			for j, cap := range lf.captures {
				g.line("(local.get $%s)", cap)
				// Box capture to anyref for polymorphic closure storage
				g.emitBox(lf.capTypes[j])
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
	// Wrapper must match $ft_apply exactly: (ref null $closure, anyref) -> anyref
	g.line("(func %s (type $ft_apply) (param $self (ref null $closure)) (param $arg (ref null any)) (result (ref null any))",
		wrapperName)
	g.indent++
	// Unbox arg from anyref to the function's expected param type
	g.line("local.get $arg")
	g.emitUnbox(fi.params[0].wasmType)
	g.line("call $%s", fi.name)
	// Box return to anyref
	g.emitBox(fi.retType)
	g.indent--
	g.line(")")
}

// ---------------------------------------------------------------------------
// Binary operators
// ---------------------------------------------------------------------------

func (g *watGen) emitBinop(e ir.CBinop) error {
	leftType := g.typeOfAtom(e.Left)
	rightType := g.typeOfAtom(e.Right)

	// Resolve anyref operands: try to determine the concrete type from the other operand
	// or from the operation context (arithmetic → i64, string ops → string)
	opType := leftType
	if opType == wtAnyRef {
		opType = rightType
	}
	// If both are anyref, try to determine from the binop kind
	if opType == wtAnyRef {
		switch e.Op {
		case "Add", "Sub", "Mul", "Div", "Mod", "Lt", "Gt", "Leq", "Geq":
			opType = wtI64 // default arithmetic type
		case "And", "Or":
			opType = wtI32
		case "Eq", "Neq":
			opType = wtI64 // default equality type
		case "Concat":
			opType = wtStringRef // string concat
		}
	}

	// Cons operator: head :: tail → struct.new $list_cons (tag=1) (box head) tail
	if e.Op == "Cons" {
		g.line("(struct.new $list_cons (i32.const 1)")
		g.indent++
		if err := g.emitAtom(e.Left); err != nil {
			return err
		}
		// Box the head element to anyref for polymorphic storage
		g.emitBox(g.typeOfAtom(e.Left))
		if err := g.emitAtom(e.Right); err != nil {
			return err
		}
		// If tail is anyref, cast to list ref
		if rightType == wtAnyRef {
			g.line("(ref.cast (ref null $list))")
		}
		g.indent--
		g.line(")")
		return nil
	}

	// String/list concat: handle specially (no unboxing needed for string operands)
	if e.Op == "Concat" {
		if err := g.emitAtom(e.Left); err != nil {
			return err
		}
		if leftType == wtAnyRef {
			g.emitUnbox(wtStringRef)
		}
		if err := g.emitAtom(e.Right); err != nil {
			return err
		}
		if rightType == wtAnyRef {
			g.emitUnbox(wtStringRef)
		}
		g.line("call $string_concat")
		return nil
	}

	// Emit left operand, unboxing from anyref if needed
	if err := g.emitAtom(e.Left); err != nil {
		return err
	}
	if leftType == wtAnyRef && opType != wtAnyRef {
		g.emitUnbox(opType)
	}

	// Emit right operand, unboxing from anyref if needed
	if err := g.emitAtom(e.Right); err != nil {
		return err
	}
	if rightType == wtAnyRef && opType != wtAnyRef {
		g.emitUnbox(opType)
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
	thenType := g.typeOfExpr(e.Then)
	elseType := g.typeOfExpr(e.Else)
	resultType := thenType
	if thenType != elseType {
		// Use function return type when in tail position and types disagree
		if g.currentFunc != nil {
			resultType = g.currentFunc.retType
		} else {
			// For ref types, anyref is the common supertype
			// For non-ref types that differ, pick the concrete one
			if isRefType(thenType) && isRefType(elseType) {
				resultType = wtAnyRef
			} else if isRefType(thenType) {
				resultType = elseType
			} else {
				resultType = thenType
			}
		}
	}
	if err := g.emitAtom(e.Cond); err != nil {
		return err
	}
	// Unbox condition to i32 if it's an anyref (e.g., from call_ref returning boxed bool)
	condType := g.typeOfAtom(e.Cond)
	if condType != wtI32 {
		g.emitUnbox(wtI32)
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
