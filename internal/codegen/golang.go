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

// ---------------------------------------------------------------------------
// External builtins — companion file pattern
// ---------------------------------------------------------------------------

// companionFuncName converts a mangled external name to the companion function name.
// "Std$String$length" → "Std_String_length"
func companionFuncName(name string) string {
	s := strings.ReplaceAll(name, "$", "_")
	s = strings.ReplaceAll(s, ".", "_")
	return s
}

// externalModule extracts the module name from a mangled external name.
// "Std$String$length" → "String", "Std$Http$Server$httpServe" → "Http.Server"
func externalModule(name string) string {
	parts := strings.Split(name, "$")
	if len(parts) < 3 {
		return ""
	}
	// First element is namespace ("Std"), last is local name, middle elements form module name
	return strings.Join(parts[1:len(parts)-1], ".")
}

// NeededModules returns the set of stdlib module names that need companion files.
// Only includes modules where the external's type was successfully resolved.
func NeededModules(prog *ir.Program, typeEnv typechecker.TypeEnv) []string {
	g := newGoGen(typeEnv, false)
	g.analyze(prog)
	var modules []string
	for mod := range g.neededModules {
		modules = append(modules, mod)
	}
	return modules
}

// returnKind inspects a Rex return type and returns "simple", "maybe", or "result".
// Only uses "result" when the error type is String — otherwise the companion
// constructs the full Result ADT itself and the wrapper passes through ("simple").
func returnKind(ty types.Type) string {
	if ty == nil {
		return "simple"
	}
	tc, ok := ty.(types.TCon)
	if !ok {
		return "simple"
	}
	switch tc.Name {
	case "Maybe":
		return "maybe"
	case "Result":
		if len(tc.Args) >= 2 {
			if errTc, ok := tc.Args[1].(types.TCon); ok && errTc.Name == "String" {
				return "result"
			}
		}
		return "simple"
	}
	return "simple"
}

// resultOkIsUnit returns true if the Result's ok type is Unit.
func resultOkIsUnit(ty types.Type) bool {
	tc, ok := ty.(types.TCon)
	if !ok || tc.Name != "Result" || len(tc.Args) < 1 {
		return false
	}
	okTy := tc.Args[0]
	otc, ok := okTy.(types.TCon)
	return ok && otc.Name == "Unit"
}

// goTypeForExternalParam maps a Rex type to the Go type used in companion function parameters.
func goTypeForExternalParam(ty types.Type) string {
	if ty == nil {
		return "any"
	}
	tc, ok := ty.(types.TCon)
	if !ok {
		return "any" // type variable
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
	case "List":
		return "*RexList"
	default:
		return "any"
	}
}

// EmitGo converts an IR program to Go source code.
func EmitGo(prog *ir.Program, typeEnv typechecker.TypeEnv) (string, error) {
	g := newGoGen(typeEnv, false)
	return g.emit(prog)
}

// EmitGoTests converts an IR program to Go source code with a test runner.
func EmitGoTests(prog *ir.Program, typeEnv typechecker.TypeEnv) (string, error) {
	g := newGoGen(typeEnv, true)
	return g.emit(prog)
}

func newGoGen(typeEnv typechecker.TypeEnv, testMode bool) *goGen {
	return &goGen{
		buf:           &strings.Builder{},
		typeEnv:       typeEnv,
		funcs:         make(map[string]*goFuncInfo),
		adts:          make(map[string]*goAdtInfo),
		ctorToAdt:     make(map[string]*goCtorInfo),
		records:          make(map[string]*goRecordInfo),
		recordsByOrigName: make(map[string]*goRecordInfo),
		traitImpls:    make(map[string][]goImplCase),
		locals:        make(map[string]bool),
		knownTypes:    map[string]bool{"Int": true, "Float": true, "String": true, "Bool": true, "List": true, "Tuple2": true, "Tuple3": true, "Tuple4": true, "Unit": true},
		neededModules: make(map[string]bool),
		testMode:      testMode,
	}
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type goFuncInfo struct {
	name       string
	arity      int
	params     []goParamInfo
	retType    types.Type
	body       ir.Expr
	isExternal bool
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
	name       string // Go name (possibly renamed with _2 suffix)
	origName   string // original IR name (before collision rename)
	fieldNames []string
	fieldTypes []types.Type
}

type goImplCase struct {
	typeName string
	funcName string
	arity    int
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
	records          map[string]*goRecordInfo
	recordsByOrigName map[string]*goRecordInfo // original IR name → record info (before collision rename)
	allRecords       []*goRecordInfo           // all records including colliding names
	traitImpls map[string][]goImplCase
	locals      map[string]bool
	tempCounter int

	// trait method names → dispatch function names
	traitMethodNames map[string]string // "myShow" → "dispatch_myshow_myShow"
	knownTypes       map[string]bool   // types defined in the program (for filtering dispatch cases)

	// external companion file support
	neededModules map[string]bool // stdlib modules needed by externals

	// test mode support
	testMode  bool
	testFuncs []testFuncInfo
}

type testFuncInfo struct {
	name     string
	funcName string // Go function name
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

	// Emit package + imports (minimal — runtime.go has the heavy imports)
	out.WriteString("package main\n\n")
	if g.testMode && len(g.testFuncs) > 0 {
		out.WriteString("import (\n\t\"fmt\"\n\t\"os\"\n\t\"strings\"\n)\n\n")
	} else if g.testMode {
		out.WriteString("import \"fmt\"\n\n")
	} else {
		out.WriteString("import \"os\"\n\n")
	}

	// Emit type definitions (ADTs, records, tuples)
	out.WriteString(g.emitTypeDefinitions())

	// Ensure Result and Maybe ADTs are defined if needed by external wrappers
	g.ensureBuiltinADTs(out, prog)

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
	if g.testMode {
		g.emitTestMain(out)
	} else {
		out.WriteString("\nfunc main() {\n")
		out.WriteString("\tos.Exit(int(rex_main(nil).(int64)))\n")
		out.WriteString("}\n")
	}

	return out.String(), nil
}

func (g *goGen) emitTestMain(out *strings.Builder) {
	out.WriteString("\nfunc main() {\n")
	if len(g.testFuncs) == 0 {
		out.WriteString("\tfmt.Println(\"0 passed, 0 failed\")\n")
		out.WriteString("}\n")
		return
	}
	out.WriteString("\tvar only string\n")
	out.WriteString("\tif len(os.Args) > 1 { only = os.Args[1] }\n")
	out.WriteString("\tpassed, failed, skipped := 0, 0, 0\n")
	for _, tf := range g.testFuncs {
		fmt.Fprintf(out, "\tif only != \"\" && !strings.Contains(%q, only) {\n", tf.name)
		fmt.Fprintf(out, "\t\tskipped++\n")
		fmt.Fprintf(out, "\t} else {\n")
		fmt.Fprintf(out, "\t\tfunc() {\n")
		fmt.Fprintf(out, "\t\t\tdefer func() {\n")
		fmt.Fprintf(out, "\t\t\t\tif r := recover(); r != nil {\n")
		fmt.Fprintf(out, "\t\t\t\t\tfmt.Fprintf(os.Stderr, \"FAIL: %%s — %%v\\n\", %q, r)\n", tf.name)
		fmt.Fprintf(out, "\t\t\t\t\tfailed++\n")
		fmt.Fprintf(out, "\t\t\t\t}\n")
		fmt.Fprintf(out, "\t\t\t}()\n")
		fmt.Fprintf(out, "\t\t\t%s()\n", tf.funcName)
		fmt.Fprintf(out, "\t\t\tpassed++\n")
		fmt.Fprintf(out, "\t\t}()\n")
		fmt.Fprintf(out, "\t}\n")
	}
	out.WriteString("\tfmt.Printf(\"%d passed, %d failed\", passed, failed)\n")
	out.WriteString("\tif skipped > 0 { fmt.Printf(\", %d skipped\", skipped) }\n")
	out.WriteString("\tfmt.Println()\n")
	out.WriteString("\tif failed > 0 { os.Exit(1) }\n")
	out.WriteString("}\n")
}

// ---------------------------------------------------------------------------
// Phase 1: Analyze
// ---------------------------------------------------------------------------

func (g *goGen) analyze(prog *ir.Program) {
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
			g.knownTypes[d.Name] = true
			if len(d.Fields) > 0 {
				// Record type — disambiguate collisions with a counter suffix
				name := d.Name
				if _, exists := g.records[name]; exists {
					for i := 2; ; i++ {
						candidate := fmt.Sprintf("%s_%d", name, i)
						if _, exists := g.records[candidate]; !exists {
							name = candidate
							break
						}
					}
				}
				ri := &goRecordInfo{name: name, origName: d.Name}
				for _, f := range d.Fields {
					ri.fieldNames = append(ri.fieldNames, f.Name)
					ri.fieldTypes = append(ri.fieldTypes, f.Ty)
				}
				g.allRecords = append(g.allRecords, ri)
				g.records[name] = ri
				g.recordsByOrigName[d.Name] = ri
			} else if len(d.Ctors) > 0 {
				// ADT — disambiguate collisions with a counter suffix (same as records)
				name := d.Name
				if _, exists := g.adts[name]; exists {
					for i := 2; ; i++ {
						candidate := fmt.Sprintf("%s_%d", name, i)
						if _, exists := g.adts[candidate]; !exists {
							name = candidate
							break
						}
					}
				}
				ai := &goAdtInfo{name: name}
				for i, c := range d.Ctors {
					ci := goCtorInfo{
						name:     c.Name,
						tag:      i,
						typeName: name,
					}
					for _, t := range c.ArgTypes {
						ci.fieldTypes = append(ci.fieldTypes, t)
					}
					ai.ctors = append(ai.ctors, ci)
					g.ctorToAdt[c.Name] = &ai.ctors[len(ai.ctors)-1]
				}
				g.adts[name] = ai
			}

		case ir.DImpl:
			for _, m := range d.Methods {
				key := d.TraitName + ":" + m.Name
				funcName := fmt.Sprintf("impl_%s_%s_%s", d.TraitName, d.TargetTypeName, m.Name)
				// Count arity by counting nested lambdas in the method body
				arity := 0
				body := m.Body
				for {
					if ec, ok := body.(ir.EComplex); ok {
						if lam, ok := ec.C.(ir.CLambda); ok {
							arity++
							body = lam.Body
							continue
						}
					}
					break
				}
				if arity == 0 {
					arity = 1
				}
				g.traitImpls[key] = append(g.traitImpls[key], goImplCase{
					typeName: d.TargetTypeName,
					funcName: funcName,
					arity:    arity,
				})
				g.scanExpr(m.Body)
			}

		case ir.DExternal:
			fi := g.analyzeExternal(d.Name)
			if fi != nil {
				g.funcs[d.Name] = fi
			}

		case ir.DTest:
			if g.testMode {
				funcName := fmt.Sprintf("rex_test_%d", len(g.testFuncs))
				g.testFuncs = append(g.testFuncs, testFuncInfo{
					name:     d.Name,
					funcName: funcName,
				})
				g.scanExpr(d.Body)
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

// analyzeExternal extracts type info from typeEnv for an external declaration.
func (g *goGen) analyzeExternal(name string) *goFuncInfo {
	// Look up type from typeEnv. The typeEnv uses local names (e.g. "println")
	// while the IR uses prefixed names (e.g. "Std$IO$println").
	// Try prefixed first, then fall back to local name.
	localName := name
	if idx := strings.LastIndex(name, "$"); idx >= 0 {
		localName = name[idx+1:]
	}
	s, ok := g.typeEnv[name]
	if !ok {
		s, ok = g.typeEnv[localName]
	}
	if !ok {
		// Try the typechecker's module cache for transitively imported externals
		s = typechecker.LookupModuleType(name)
		if s == nil {
			return nil // type not available — skip this external
		}
	}
	scheme, ok := s.(types.Scheme)
	if !ok {
		return nil
	}

	// Track needed module (only after type lookup succeeds).
	mod := externalModule(name)
	if mod != "" {
		g.neededModules[mod] = true
	}

	fi := &goFuncInfo{name: name, isExternal: true}
	ty := scheme.Ty
	for {
		tc, ok := ty.(types.TCon)
		if !ok || tc.Name != "Fun" || len(tc.Args) != 2 {
			break
		}
		fi.params = append(fi.params, goParamInfo{
			name: fmt.Sprintf("p%d", fi.arity),
			ty:   tc.Args[0],
		})
		fi.arity++
		ty = tc.Args[1]
	}
	fi.retType = ty

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

func (g *goGen) scanAtom(a ir.Atom) {}

func (g *goGen) scanCExpr(c ir.CExpr) {
	switch c := c.(type) {
	case ir.CApp:
		g.scanAtom(c.Func)
		g.scanAtom(c.Arg)
	case ir.CIf:
		g.scanExpr(c.Then)
		g.scanExpr(c.Else)
	case ir.CMatch:
		for _, arm := range c.Arms {
			g.scanExpr(arm.Body)
		}
	case ir.CLambda:
		g.scanExpr(c.Body)
	case ir.CAssert:
		g.scanAtom(c.Expr)
	}
}

// ---------------------------------------------------------------------------
// Imports
// ---------------------------------------------------------------------------

// emitImports is no longer needed — runtime.go carries the imports.
// main.go only needs "fmt" and "os" which are emitted inline in emit().

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

	return b.String()
}

// ---------------------------------------------------------------------------
// Runtime helpers
// ---------------------------------------------------------------------------

// RuntimeSource returns the static Go runtime extracted to every build directory.
func RuntimeSource() string {
	return runtimeSource
}

// ---------------------------------------------------------------------------
// Trait dispatch
// ---------------------------------------------------------------------------

// ensureBuiltinADTs emits Result and Maybe type definitions if they're needed
// by external wrappers but not already defined in the program's type declarations.
func (g *goGen) ensureBuiltinADTs(out *strings.Builder, prog *ir.Program) {
	needResult, needMaybe := false, false
	for _, d := range prog.Decls {
		if ext, ok := d.(ir.DExternal); ok {
			fi := g.funcs[ext.Name]
			if fi == nil {
				continue
			}
			switch returnKind(fi.retType) {
			case "result":
				needResult = true
			case "maybe":
				needMaybe = true
			}
		}
	}

	if needResult && g.adts["Result"] == nil {
		out.WriteString("type Rex_Result interface{ tagRex_Result() int }\n")
		out.WriteString("type Rex_Result_Ok struct { F0 any }\n")
		out.WriteString("func (Rex_Result_Ok) tagRex_Result() int { return 0 }\n")
		out.WriteString("type Rex_Result_Err struct { F0 any }\n")
		out.WriteString("func (Rex_Result_Err) tagRex_Result() int { return 1 }\n\n")
	}
	if needMaybe && g.adts["Maybe"] == nil {
		out.WriteString("type Rex_Maybe interface{ tagRex_Maybe() int }\n")
		out.WriteString("type Rex_Maybe_Nothing struct{}\n")
		out.WriteString("func (Rex_Maybe_Nothing) tagRex_Maybe() int { return 0 }\n")
		out.WriteString("type Rex_Maybe_Just struct { F0 any }\n")
		out.WriteString("func (Rex_Maybe_Just) tagRex_Maybe() int { return 1 }\n\n")
	}
}

func (g *goGen) emitTraitDispatchers() string {
	var b strings.Builder

	for key, cases := range g.traitImpls {
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		traitName, methodName := parts[0], parts[1]
		dispatchName := fmt.Sprintf("dispatch_%s_%s", strings.ToLower(traitName), methodName)

		// Filter cases to only include types defined in the program
		var filteredCases []goImplCase
		for _, c := range cases {
			if g.knownTypes[c.typeName] {
				filteredCases = append(filteredCases, c)
			}
		}
		if len(filteredCases) == 0 {
			continue
		}

		// Determine arity from the first impl case
		arity := 1
		if len(filteredCases) > 0 {
			arity = filteredCases[0].arity
		}

		fmt.Fprintf(&b, "func %s(args ...any) any {\n", dispatchName)
		// Support curried calls for multi-arg methods (e.g., eq, compare)
		if arity > 1 {
			b.WriteString("\tif len(args) == 1 {\n")
			fmt.Fprintf(&b, "\t\treturn func(b any) any { return %s(args[0], b) }\n", dispatchName)
			b.WriteString("\t}\n")
		}
		b.WriteString("\tv := args[0]\n")
		// Handle nil (Unit) before the type switch
		for _, c := range filteredCases {
			if c.typeName == "Unit" {
				fmt.Fprintf(&b, "\tif v == nil { return %s(args...) }\n", c.funcName)
			}
		}
		b.WriteString("\tswitch v.(type) {\n")
		for _, c := range filteredCases {
			if c.typeName == "Unit" {
				continue // already handled above
			}
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
	case "Tuple2":
		return "Tuple2"
	case "Tuple3":
		return "Tuple3"
	case "Tuple4":
		return "Tuple4"
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
	case ir.DExternal:
		return g.emitDExternal(d.Name)
	case ir.DTest:
		if g.testMode {
			return g.emitDTest(d)
		}
		return nil
	case ir.DType, ir.DTrait, ir.DImport:
		return nil
	default:
		return nil
	}
}

func (g *goGen) emitDLet(d ir.DLet) error {
	fi := g.funcs[d.Name]

	// _ bindings: side-effect-only, always emit
	if d.Name == "_" {
		g.buf.WriteString("var _ = ")
		g.locals = make(map[string]bool)
		if err := g.emitExprInline(d.Body); err != nil {
			return err
		}
		g.buf.WriteString("\n\n")
		return nil
	}

	if fi == nil {
		return nil
	}

	goName := goFuncName(d.Name)

	if fi.arity == 0 {
		// Top-level value (not a function) — emit as var any to keep polymorphic
		g.buf.WriteString(fmt.Sprintf("var %s any = ", goName))
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

func (g *goGen) emitDExternal(name string) error {
	fi := g.funcs[name]
	if fi == nil {
		return nil
	}

	goName := goFuncName(name)
	compName := companionFuncName(name)

	if fi.arity == 0 {
		// Zero-arity: variable
		fmt.Fprintf(g.buf, "var %s any = %s\n\n", goName, compName)
		return nil
	}

	// Determine wrapper pattern from return type
	rk := returnKind(fi.retType)

	// Emit function signature: func rex_Name(p0 any, p1 any, ...) any {
	fmt.Fprintf(g.buf, "func %s(", goName)
	for i := range fi.params {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		fmt.Fprintf(g.buf, "p%d any", i)
	}
	g.buf.WriteString(") any {\n")

	// Build companion call arguments with type assertions
	var callArgs strings.Builder
	for i, p := range fi.params {
		if i > 0 {
			callArgs.WriteString(", ")
		}
		goTy := goTypeForExternalParam(p.ty)
		if goTy == "any" {
			fmt.Fprintf(&callArgs, "p%d", i)
		} else {
			fmt.Fprintf(&callArgs, "p%d.(%s)", i, goTy)
		}
	}
	call := fmt.Sprintf("%s(%s)", compName, callArgs.String())

	switch rk {
	case "maybe":
		fmt.Fprintf(g.buf, "\tval := %s\n", call)
		g.buf.WriteString("\tif val == nil { return Rex_Maybe_Nothing{} }\n")
		g.buf.WriteString("\treturn Rex_Maybe_Just{F0: *val}\n")

	case "result":
		if resultOkIsUnit(fi.retType) {
			fmt.Fprintf(g.buf, "\terr := %s\n", call)
			g.buf.WriteString("\tif err != nil { return Rex_Result_Err{F0: err.Error()} }\n")
			g.buf.WriteString("\treturn Rex_Result_Ok{F0: nil}\n")
		} else {
			fmt.Fprintf(g.buf, "\tval, err := %s\n", call)
			g.buf.WriteString("\tif err != nil { return Rex_Result_Err{F0: err.Error()} }\n")
			g.buf.WriteString("\treturn Rex_Result_Ok{F0: val}\n")
		}

	default:
		fmt.Fprintf(g.buf, "\treturn %s\n", call)
	}

	g.buf.WriteString("}\n\n")
	return nil
}

func (g *goGen) emitDTest(d ir.DTest) error {
	// Find the test info from analyze phase
	for _, ti := range g.testFuncs {
		if ti.name == d.Name {
			fmt.Fprintf(g.buf, "func %s() {\n", ti.funcName)
			g.indent = 1
			g.locals = make(map[string]bool)
			if err := g.emitExprStmt(d.Body, false); err != nil {
				return err
			}
			g.buf.WriteString("}\n\n")
			g.indent = 0
			return nil
		}
	}
	return nil
}

func (g *goGen) emitDImpl(d ir.DImpl) error {
	// Skip impls for types not in the program
	if !g.knownTypes[d.TargetTypeName] {
		return nil
	}
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
		} else if _, isUnit := e.A.(ir.AUnit); !isUnit {
			// Skip bare unit in non-return position (bare `nil` is invalid Go)
			g.wn("_ = ")
			g.emitAtom(e.A)
			g.buf.WriteByte('\n')
		}
		return nil

	case ir.EComplex:
		return g.emitCExprStmt(e.C, isReturn)

	case ir.ELet:
		// CAssert bindings: emit as statement (no variable needed)
		if _, isAssert := e.Bind.(ir.CAssert); isAssert {
			if err := g.emitCExprStmt(e.Bind, false); err != nil {
				return err
			}
			return g.emitExprStmt(e.Body, isReturn)
		}
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

	case ir.CAssert:
		g.wn("if !(any(")
		g.emitAtom(c.Expr)
		fmt.Fprintf(g.buf, ").(bool)) { panic(\"assert failed at line %d\") }\n", c.Line)
		return nil

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

	case ir.CAssert:
		// As inline expression, wrap in IIFE
		g.buf.WriteString("func() any { if !(any(")
		g.emitAtom(c.Expr)
		fmt.Fprintf(g.buf, ").(bool)) { panic(\"assert failed at line %d\") }; return nil }()", c.Line)
		return nil
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

	// Core builtins (available without import)
	switch funcName {
	case "__id":
		g.emitAtom(c.Arg)
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
		// Avoid compile-time constant division by zero — use runtime variable
		if isLiteralZero(c.Right) {
			g.buf.WriteString("func() int64 { d := int64(0); return ")
			g.emitAtomAsInt(c.Left)
			g.buf.WriteString(" / d }()")
			return nil
		}
		return g.emitArithBinop(c, "/")
	case "Mod":
		// Avoid compile-time constant modulo by zero — use explicit panic with distinct message
		if isLiteralZero(c.Right) {
			g.buf.WriteString("func() int64 { panic(\"runtime error: modulo by zero\") }()")
			return nil
		}
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
	isFloat := isFloatType(c.Ty) || isFloatAtom(c.Left) || isFloatAtom(c.Right)
	isInt := isIntType(c.Ty) || isIntAtom(c.Left) || isIntAtom(c.Right)
	if isFloat {
		g.buf.WriteString("(")
		g.emitAtomAsFloat(c.Left)
		fmt.Fprintf(g.buf, " %s ", op)
		g.emitAtomAsFloat(c.Right)
		g.buf.WriteString(")")
	} else if isInt {
		g.buf.WriteString("(")
		g.emitAtomAsInt(c.Left)
		fmt.Fprintf(g.buf, " %s ", op)
		g.emitAtomAsInt(c.Right)
		g.buf.WriteString(")")
	} else {
		// Unknown type — use runtime dispatch
		fmt.Fprintf(g.buf, "rex_arith(")
		g.emitAtom(c.Left)
		fmt.Fprintf(g.buf, ", ")
		g.emitAtom(c.Right)
		fmt.Fprintf(g.buf, ", %q)", op)
	}
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
	// Look up the (possibly renamed) record to get the correct Go struct name
	structName := goTypeName(c.TypeName)
	var fieldNames []string
	for _, f := range c.Fields {
		fieldNames = append(fieldNames, f.Name)
	}
	if ri := g.findRecordByOrigName(c.TypeName, fieldNames); ri != nil {
		structName = goTypeName(ri.name)
	}
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
	// Also check allRecords for colliding names
	for _, ri := range g.allRecords {
		for _, fn := range ri.fieldNames {
			if fn == field {
				return ri
			}
		}
	}
	return nil
}

// findRecordByOrigName finds a record by its original IR name and field names.
// When multiple records share the same original name (collision), it matches on fields.
func (g *goGen) findRecordByOrigName(origName string, fieldNames []string) *goRecordInfo {
	var candidates []*goRecordInfo
	for _, ri := range g.allRecords {
		if ri.origName == origName {
			candidates = append(candidates, ri)
		}
	}
	if len(candidates) == 1 {
		return candidates[0]
	}
	// Multiple records with same original name — match by field names
	for _, ri := range candidates {
		if len(ri.fieldNames) == len(fieldNames) {
			match := true
			for i, fn := range ri.fieldNames {
				if fn != fieldNames[i] {
					match = false
					break
				}
			}
			if match {
				return ri
			}
		}
	}
	// Fallback
	if len(candidates) > 0 {
		return candidates[0]
	}
	return nil
}

func (g *goGen) emitRecordUpdate(c ir.CRecordUpdate) error {
	// Resolve record type: try Ty annotation, then record atom's type, then field name lookup
	var recTypeName string
	if tc, ok := c.Ty.(types.TCon); ok {
		recTypeName = tc.Name
	}
	if recTypeName == "" {
		// Try the record atom's type
		if v, ok := c.Record.(ir.AVar); ok && v.Ty != nil {
			if tc, ok := v.Ty.(types.TCon); ok {
				recTypeName = tc.Name
			}
		}
	}
	if recTypeName == "" && len(c.Updates) > 0 {
		// Find a record that has ALL updated fields (avoids collision between records with overlapping fields)
		var updateFields []string
		for _, u := range c.Updates {
			updateFields = append(updateFields, u.Path[0])
		}
		for _, ri := range g.allRecords {
			hasAll := true
			for _, uf := range updateFields {
				found := false
				for _, fn := range ri.fieldNames {
					if fn == uf {
						found = true
						break
					}
				}
				if !found {
					hasAll = false
					break
				}
			}
			if hasAll {
				recTypeName = ri.name
				break
			}
		}
		if recTypeName == "" {
			ri := g.findRecordForField(c.Updates[0].Path[0])
			if ri != nil {
				recTypeName = ri.name
			}
		}
	}
	if recTypeName == "" {
		field := ""
		if len(c.Updates) > 0 {
			field = c.Updates[0].Path[0]
		}
		return fmt.Errorf("cannot determine record type for update (field=%q)", field)
	}

	// Map to the (possibly renamed) Go struct name
	if ri, ok := g.recordsByOrigName[recTypeName]; ok {
		recTypeName = ri.name
	}
	structName := goTypeName(recTypeName)
	g.buf.WriteString("func() any {\n")
	g.indent++
	g.w("r := %s", g.atomStr(c.Record))
	g.w("copy := r.(%s)", structName)
	for _, u := range c.Updates {
		if err := g.emitFieldUpdate("copy", u, recTypeName); err != nil {
			return err
		}
	}
	g.w("return copy")
	g.indent--
	for i := 0; i < g.indent; i++ {
		g.buf.WriteByte('\t')
	}
	g.buf.WriteString("}()")
	return nil
}

func (g *goGen) emitFieldUpdate(varName string, u ir.FieldUpdate, recTypeName string) error {
	if len(u.Path) == 1 {
		g.wn("%s.%s = ", varName, goExportedField(u.Path[0]))
		g.emitAtom(u.Value)
		g.buf.WriteByte('\n')
		return nil
	}
	// Nested path: clone each intermediate record.
	// Use the next field in the path to determine the intermediate record type.
	for i := 0; i < len(u.Path)-1; i++ {
		field := u.Path[i]
		nextField := u.Path[i+1]
		ri := g.findRecordForField(nextField)
		if ri == nil {
			return fmt.Errorf("cannot determine nested record type for field '%s'", field)
		}
		nestedStruct := goTypeName(ri.name)
		innerVar := fmt.Sprintf("inner%d", i)
		g.w("%s := %s.%s.(%s)", innerVar, varName, goExportedField(field), nestedStruct)
		varName = innerVar
	}
	// Set the leaf field
	lastField := u.Path[len(u.Path)-1]
	g.wn("%s.%s = ", varName, goExportedField(lastField))
	g.emitAtom(u.Value)
	g.buf.WriteByte('\n')
	// Write back up the chain
	for i := len(u.Path) - 2; i >= 0; i-- {
		field := u.Path[i]
		innerVar := fmt.Sprintf("inner%d", i)
		outerVar := "copy"
		if i > 0 {
			outerVar = fmt.Sprintf("inner%d", i-1)
		}
		g.w("%s.%s = %s", outerVar, goExportedField(field), innerVar)
	}
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
				g.w("_ = %s", goVarName(b.name))
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
			g.w("_ = %s", goVarName(b.name))
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
		return fmt.Sprintf("(%s.(*RexList) == nil || %s == nil)", scrutExpr, scrutExpr), nil

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
		var fieldNames []string
		for _, f := range p.Fields {
			fieldNames = append(fieldNames, f.Name)
		}
		ri := g.findRecordByOrigName(p.TypeName, fieldNames)
		if ri == nil {
			ri = g.records[p.TypeName]
		}
		if ri == nil {
			return "true", nil
		}
		structName := goTypeName(ri.name)
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
		// Core builtins as values (when passed as function arguments)
		switch name {
		case "showInt":
			return "func(v any) any { return rex_showInt(v) }"
		case "showFloat":
			return "func(v any) any { return rex_showFloat(v) }"
		case "not":
			return "func(v any) any { return rex_not(v) }"
		case "error":
			return "func(v any) any { return rex_error(v) }"
		}
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
		// Check if it's a known record type used as a positional constructor
		if ri, ok := g.recordsByOrigName[name]; ok {
			return g.recordAsClosure(ri)
		}
		// Check if it's a known top-level function
		if fi, ok := g.funcs[name]; ok {
			if !g.locals[name] {
				if fi.arity > 0 {
					// Wrap as closure when used as a value
					return g.funcAsClosure(name, fi)
				}
				// Zero-arity top-level binding — use goFuncName for rex_ prefix
				return goFuncName(name)
			}
		}
		// Module-qualified names (contain $) always use goFuncName for the rex_ prefix
		if strings.Contains(name, "$") {
			return goFuncName(name)
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

func (g *goGen) recordAsClosure(ri *goRecordInfo) string {
	structName := goTypeName(ri.name)
	n := len(ri.fieldNames)
	if n == 0 {
		return structName + "{}"
	}
	// Build params
	var params []string
	for i := 0; i < n; i++ {
		params = append(params, fmt.Sprintf("a%d", i))
	}
	// Build struct creation with named fields
	var fields []string
	for i, p := range params {
		fields = append(fields, fmt.Sprintf("%s: %s", goExportedField(ri.fieldNames[i]), p))
	}
	result := fmt.Sprintf("%s{%s}", structName, strings.Join(fields, ", "))
	// Wrap from inside out for currying
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

func isIntType(ty types.Type) bool {
	if ty == nil {
		return false
	}
	tc, ok := ty.(types.TCon)
	return ok && tc.Name == "Int"
}

func isIntAtom(a ir.Atom) bool {
	switch v := a.(type) {
	case ir.AInt:
		return true
	case ir.AVar:
		return v.Ty != nil && isIntType(v.Ty)
	}
	return false
}

func isFloatAtom(a ir.Atom) bool {
	switch v := a.(type) {
	case ir.AFloat:
		return true
	case ir.AVar:
		return v.Ty != nil && isFloatType(v.Ty)
	}
	return false
}

func isLiteralZero(a ir.Atom) bool {
	if v, ok := a.(ir.AInt); ok {
		return v.Value == 0
	}
	return false
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
		case c == '$':
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
