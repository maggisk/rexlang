// Package codegen — JavaScript backend.
//
// EmitJS converts an IR program into a JavaScript source file. The output is a
// standalone Node.js script that can be run with `node`.
package codegen

import (
	"fmt"
	"strings"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// EmitJS converts an IR program to JavaScript source code (browser target).
func EmitJS(prog *ir.Program, typeEnv typechecker.TypeEnv, jsBindings []ir.JsBinding, moduleMode string) (string, error) {
	g := &jsGen{
		buf:              &strings.Builder{},
		typeEnv:          typeEnv,
		funcs:            make(map[string]*jsFuncInfo),
		ctorToAdt:        make(map[string]*jsCtorInfo),
		records:          make(map[string]*jsRecordInfo),
		traitImpls:       make(map[string][]jsImplCase),
		traitMethodNames: make(map[string]string),
		usedBuiltins:     make(map[string]bool),
		locals:           make(map[string]bool),
		knownTypes:       map[string]bool{"Int": true, "Float": true, "String": true, "Bool": true},
		jsBindings:       jsBindings,
		moduleMode:       moduleMode,
	}
	return g.emit(prog)
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type jsFuncInfo struct {
	name   string
	arity  int
	params []jsParamInfo
	body   ir.Expr
}

type jsParamInfo struct {
	name string
	ty   types.Type
}

type jsCtorInfo struct {
	name       string
	tag        int
	typeName   string
	fieldTypes []types.Type
}

type jsRecordInfo struct {
	name       string
	fieldNames []string
	fieldTypes []types.Type
}

type jsImplCase struct {
	typeName string
	funcName string
}

// ---------------------------------------------------------------------------
// Generator state
// ---------------------------------------------------------------------------

type jsGen struct {
	buf              *strings.Builder
	indent           int
	typeEnv          typechecker.TypeEnv
	funcs            map[string]*jsFuncInfo
	ctorToAdt        map[string]*jsCtorInfo
	records          map[string]*jsRecordInfo
	traitImpls       map[string][]jsImplCase
	traitMethodNames map[string]string
	usedBuiltins     map[string]bool
	locals           map[string]bool
	tempCounter      int

	// track what features are used
	usesConcurrency bool
	usesJsFfi       bool              // browser: Std:Js FFI primitives
	knownTypes      map[string]bool   // types defined in the program
	jsBindings      []ir.JsBinding    // companion JS file bindings
	moduleMode      string            // module compilation mode
}

func (g *jsGen) fresh() string {
	g.tempCounter++
	return fmt.Sprintf("_t%d", g.tempCounter)
}

// ---------------------------------------------------------------------------
// Output helpers
// ---------------------------------------------------------------------------

func (g *jsGen) w(format string, args ...any) {
	for i := 0; i < g.indent; i++ {
		g.buf.WriteString("  ")
	}
	fmt.Fprintf(g.buf, format, args...)
	g.buf.WriteByte('\n')
}

func (g *jsGen) wn(format string, args ...any) {
	for i := 0; i < g.indent; i++ {
		g.buf.WriteString("  ")
	}
	fmt.Fprintf(g.buf, format, args...)
}

func (g *jsGen) raw(s string) {
	g.buf.WriteString(s)
	g.buf.WriteByte('\n')
}

// ---------------------------------------------------------------------------
// Main emit
// ---------------------------------------------------------------------------

func (g *jsGen) emit(prog *ir.Program) (string, error) {
	// Phase 1: analyze
	g.analyze(prog)

	// Phase 2: emit module body
	body := &strings.Builder{}

	// Emit runtime helpers
	body.WriteString(g.emitRuntimeHelpers())

	// Emit companion JS bindings (external FFI)
	for _, b := range g.jsBindings {
		fmt.Fprintf(body, "const %s = (() => {\n%s\n})();\n\n", b.MangledName, b.Source)
	}

	// Emit trait dispatch functions
	body.WriteString(g.emitTraitDispatchers())

	// Emit top-level declarations.
	// When overlay files shadow stubs, the same name appears multiple times.
	// Keep only the last DLet/DLetRec definition for each name (overlay wins).
	g.buf = body
	lastDeclIdx := make(map[string]int)
	for i, d := range prog.Decls {
		switch d := d.(type) {
		case ir.DLet:
			if d.Name != "_" {
				lastDeclIdx[d.Name] = i
			}
		case ir.DLetRec:
			for _, b := range d.Bindings {
				lastDeclIdx[b.Name] = i
			}
		}
	}
	for i, d := range prog.Decls {
		skip := false
		switch d := d.(type) {
		case ir.DLet:
			if d.Name != "_" && lastDeclIdx[d.Name] != i {
				skip = true
			}
		case ir.DLetRec:
			// Skip if ALL bindings in this group have a later definition
			allLater := true
			for _, b := range d.Bindings {
				if lastDeclIdx[b.Name] == i {
					allLater = false
					break
				}
			}
			if allLater {
				skip = true
			}
		}
		if skip {
			continue
		}
		if err := g.emitDecl(d); err != nil {
			return "", err
		}
	}

	// Emit trait dispatch functions (after all impls are collected)
	g.emitAllDispatchers()

	// Phase 3: wrap in module format
	return g.wrapModule(body.String()), nil
}

// wrapModule wraps the emitted JS body in the appropriate module format.
func (g *jsGen) wrapModule(body string) string {
	mode := g.moduleMode
	if mode == "" {
		mode = "global:Rex"
	}

	switch {
	case mode == "esm":
		return "\"use strict\";\n\n" + body + "\nexport function main() { return $main(null); }\n"

	case mode == "cjs":
		return "\"use strict\";\n\n" + body + "\nmodule.exports = { main: () => $main(null) };\n"

	default:
		// global or global:Name
		name := "Rex"
		if strings.HasPrefix(mode, "global:") {
			name = strings.TrimPrefix(mode, "global:")
		}
		return fmt.Sprintf("const %s = (() => {\n\"use strict\";\n\n%s\nreturn { main: () => $main(null) };\n})();\n%s.main();\n", name, body, name)
	}
}

// ---------------------------------------------------------------------------
// Phase 1: Analyze
// ---------------------------------------------------------------------------

func (g *jsGen) analyze(prog *ir.Program) {
	// Register Prelude trait methods as builtins
	g.traitMethodNames["show"] = "$display"
	g.traitMethodNames["eq"] = "$eq"
	g.traitMethodNames["compare"] = "$compare"

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
				// Record type
				ri := &jsRecordInfo{name: d.Name}
				for _, f := range d.Fields {
					ri.fieldNames = append(ri.fieldNames, f.Name)
					ri.fieldTypes = append(ri.fieldTypes, f.Ty)
				}
				g.records[d.Name] = ri
			} else if len(d.Ctors) > 0 {
				// ADT
				for i, c := range d.Ctors {
					ci := &jsCtorInfo{
						name:     c.Name,
						tag:      i,
						typeName: d.Name,
					}
					for _, t := range c.ArgTypes {
						ci.fieldTypes = append(ci.fieldTypes, t)
					}
					g.ctorToAdt[c.Name] = ci
				}
			}

		case ir.DImpl:
			for _, m := range d.Methods {
				key := d.TraitName + ":" + m.Name
				funcName := fmt.Sprintf("impl_%s_%s_%s", d.TraitName, d.TargetTypeName, m.Name)
				g.traitImpls[key] = append(g.traitImpls[key], jsImplCase{
					typeName: d.TargetTypeName,
					funcName: funcName,
				})
				g.scanExpr(m.Body)
			}
		}
	}
}

func (g *jsGen) analyzeFunc(d ir.DLet) *jsFuncInfo {
	fi := &jsFuncInfo{name: d.Name, body: d.Body}
	// Unwrap lambda chain to find params
	body := d.Body
	for {
		if ec, ok := body.(ir.EComplex); ok {
			if lam, ok := ec.C.(ir.CLambda); ok {
				fi.params = append(fi.params, jsParamInfo{name: lam.Param, ty: lam.Ty})
				fi.arity++
				body = lam.Body
				fi.body = body
				continue
			}
		}
		break
	}
	return fi
}

func (g *jsGen) analyzeFuncFromBinding(b ir.RecBinding) *jsFuncInfo {
	fi := &jsFuncInfo{name: b.Name}
	lam, ok := b.Bind.(ir.CLambda)
	if !ok {
		fi.body = ir.EComplex{C: b.Bind}
		return fi
	}
	body := ir.Expr(ir.EComplex{C: lam})
	for {
		if ec, ok := body.(ir.EComplex); ok {
			if l, ok := ec.C.(ir.CLambda); ok {
				fi.params = append(fi.params, jsParamInfo{name: l.Param, ty: l.Ty})
				fi.arity++
				body = l.Body
				fi.body = body
				continue
			}
		}
		break
	}
	return fi
}

// scanExpr walks an expression tree to detect which features are used.
func (g *jsGen) scanExpr(expr ir.Expr) {
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

// jsFfiBuiltin checks if a name is a Std:Js FFI builtin (possibly module-prefixed).
// Returns the short name (e.g. "jsGlobal") if it is, or "" if not.
func jsFfiBuiltin(name string) string {
	short := name
	if strings.HasPrefix(name, "Std$Js$") {
		short = name[len("Std$Js$"):]
	}
	switch short {
	case "jsGlobal", "jsGet", "jsSet", "jsCall", "jsNew", "jsCallback",
		"jsFromString", "jsFromInt", "jsFromFloat", "jsFromBool",
		"jsToString", "jsToInt", "jsToFloat", "jsToBool", "jsNull":
		return short
	}
	return ""
}

func (g *jsGen) scanAtom(a ir.Atom) {
	if v, ok := a.(ir.AVar); ok {
		switch v.Name {
		case "println", "print", "showInt", "showFloat", "not", "error", "todo", "toString":
			g.usedBuiltins[v.Name] = true
		case "spawn", "send", "receive", "self", "call":
			g.usesConcurrency = true
		default:
			if jsFfiBuiltin(v.Name) != "" {
				g.usesJsFfi = true
			}
		}
	}
}

func (g *jsGen) scanCExpr(c ir.CExpr) {
	switch c := c.(type) {
	case ir.CApp:
		g.scanAtom(c.Func)
		g.scanAtom(c.Arg)
	case ir.CBinop:
		g.scanAtom(c.Left)
		g.scanAtom(c.Right)
	case ir.CUnaryMinus:
		g.scanAtom(c.Expr)
	case ir.CIf:
		g.scanAtom(c.Cond)
		g.scanExpr(c.Then)
		g.scanExpr(c.Else)
	case ir.CMatch:
		g.scanAtom(c.Scrutinee)
		for _, arm := range c.Arms {
			g.scanExpr(arm.Body)
		}
	case ir.CLambda:
		g.scanExpr(c.Body)
	case ir.CCtor:
		for _, a := range c.Args {
			g.scanAtom(a)
		}
	case ir.CRecord:
		for _, f := range c.Fields {
			g.scanAtom(f.Value)
		}
	case ir.CFieldAccess:
		g.scanAtom(c.Record)
	case ir.CRecordUpdate:
		g.scanAtom(c.Record)
		for _, u := range c.Updates {
			g.scanAtom(u.Value)
		}
	case ir.CList:
		for _, a := range c.Items {
			g.scanAtom(a)
		}
	case ir.CTuple:
		for _, a := range c.Items {
			g.scanAtom(a)
		}
	case ir.CStringInterp:
		for _, a := range c.Parts {
			g.scanAtom(a)
		}
	}
}

// ---------------------------------------------------------------------------
// Runtime helpers
// ---------------------------------------------------------------------------

func (g *jsGen) emitRuntimeHelpers() string {
	var b strings.Builder

	// $eq: structural equality
	b.WriteString(`function $eq(a, b) {
  if (a === b) return true;
  if (a === null || b === null) return a === b;
  if (typeof a !== typeof b) return false;
  if (typeof a === "object") {
    if (a.$tag !== undefined && b.$tag !== undefined) {
      if (a.$tag !== b.$tag) return false;
      const ka = Object.keys(a), kb = Object.keys(b);
      if (ka.length !== kb.length) return false;
      for (const k of ka) { if (k !== "$tag" && !$eq(a[k], b[k])) return false; }
      return true;
    }
    if (Array.isArray(a) && Array.isArray(b)) {
      if (a.length !== b.length) return false;
      for (let i = 0; i < a.length; i++) { if (!$eq(a[i], b[i])) return false; }
      return true;
    }
    const ka = Object.keys(a), kb = Object.keys(b);
    if (ka.length !== kb.length) return false;
    for (const k of ka) { if (!$eq(a[k], b[k])) return false; }
    return true;
  }
  return false;
}

`)

	// $compare: structural comparison
	b.WriteString(`function $compare(a, b) {
  if (typeof a === "number" && typeof b === "number") return a < b ? -1 : a > b ? 1 : 0;
  if (typeof a === "string" && typeof b === "string") return a < b ? -1 : a > b ? 1 : 0;
  if (typeof a === "boolean" && typeof b === "boolean") return (a ? 1 : 0) - (b ? 1 : 0);
  return 0;
}

`)

	// $display: convert any value to string
	b.WriteString(`function $display(v) {
  if (v === null) return "()";
  if (typeof v === "number") return Number.isInteger(v) ? String(v) : String(v);
  if (typeof v === "string") return v;
  if (typeof v === "boolean") return v ? "true" : "false";
  if (Array.isArray(v)) return "(" + v.map($display).join(", ") + ")";
  if (typeof v === "object") {
    if (v.$tag === "Cons") {
      const items = [];
      let cur = v;
      while (cur !== null && cur.$tag === "Cons") { items.push($display(cur.head)); cur = cur.tail; }
      return "[" + items.join(", ") + "]";
    }
    if (v.$tag === "Nil") return "[]";
    if (v.$tag !== undefined) {
      const fields = Object.keys(v).filter(k => k !== "$tag" && k !== "$type");
      if (fields.length === 0) return v.$tag;
      return v.$tag + " " + fields.map(k => $display(v[k])).join(" ");
    }
    const entries = Object.entries(v);
    return "{ " + entries.map(([k, val]) => k + " = " + $display(val)).join(", ") + " }";
  }
  return String(v);
}

`)

	// $$apply: apply a function to an argument
	b.WriteString(`function $$apply(f, arg) {
  return f(arg);
}

`)

	// Builtins
	b.WriteString(`function $println(v) { console.log($display(v)); return null; }
function $print(v) { console.log($display(v)); return null; }`)
	b.WriteString(`
function $showInt(v) { return String(v); }
function $showFloat(v) { return String(v); }
function $not(v) { return !v; }
function $error(msg) { throw new Error(typeof msg === "string" ? msg : $display(msg)); }
function $todo(msg) { throw new Error("TODO: " + (typeof msg === "string" ? msg : $display(msg))); }

`)

	// String builtins
	b.WriteString(`function $stringLength(s) { return [...s].length; }
function $toUpper(s) { return s.toUpperCase(); }
function $toLower(s) { return s.toLowerCase(); }
function $trim(s) { return s.trim(); }
function $trimLeft(s) { return s.trimStart(); }
function $trimRight(s) { return s.trimEnd(); }
function $stringReverse(s) { return [...s].reverse().join(""); }
function $split(sep, s) { return s.split(sep).reduceRight((t, h) => ({$tag: "Cons", head: h, tail: t}), null); }
function $join(sep, lst) {
  const items = [];
  let cur = lst;
  while (cur !== null && cur.$tag === "Cons") { items.push(cur.head); cur = cur.tail; }
  return items.join(sep);
}
function $contains(sub, s) { return s.includes(sub); }
function $startsWith(pfx, s) { return s.startsWith(pfx); }
function $endsWith(sfx, s) { return s.endsWith(sfx); }
function $replace(from, to, s) { return s.split(from).join(to); }
function $substring(start, end, s) { return [...s].slice(start, end).join(""); }
function $repeat(n, s) { return s.repeat(n); }
function $charAt(i, s) { const chars = [...s]; return i >= 0 && i < chars.length ? {$tag: "Just", _0: chars[i], $type: "Maybe"} : {$tag: "Nothing", $type: "Maybe"}; }
function $indexOf(sub, s) { const i = s.indexOf(sub); return i >= 0 ? {$tag: "Just", _0: i, $type: "Maybe"} : {$tag: "Nothing", $type: "Maybe"}; }
function $padLeft(n, ch, s) { return s.padStart(n, ch); }
function $padRight(n, ch, s) { return s.padEnd(n, ch); }
function $words(s) { const ws = s.trim().split(/\s+/).filter(x => x); return ws.reduceRight((t, h) => ({$tag: "Cons", head: h, tail: t}), null); }
function $lines(s) { const ls = s.split(/\r?\n/); return ls.reduceRight((t, h) => ({$tag: "Cons", head: h, tail: t}), null); }
function $charCode(s) { return s.codePointAt(0) || 0; }
function $fromCharCode(n) { return String.fromCodePoint(n); }
function $stringParseInt(s) { const n = parseInt(s, 10); return isNaN(n) ? {$tag: "Nothing", $type: "Maybe"} : {$tag: "Just", _0: n, $type: "Maybe"}; }
function $stringParseFloat(s) { const n = parseFloat(s); return isNaN(n) ? {$tag: "Nothing", $type: "Maybe"} : {$tag: "Just", _0: n, $type: "Maybe"}; }
function $stringToList(s) { return [...s].reduceRight((t, h) => ({$tag: "Cons", head: h, tail: t}), null); }
function $stringFromList(lst) { const chars = []; let cur = lst; while (cur !== null && cur.$tag === "Cons") { chars.push(cur.head); cur = cur.tail; } return chars.join(""); }
function $toFloat(n) { return n; }

`)


	// Concurrency runtime — synchronous CPS actors
	if g.usesConcurrency {
		b.WriteString(`// Actor runtime — direct function calls via CPS-transformed receive().
// spawn(f) runs f, which sets pid._resume = (msg) => { ... } and returns.
// send(pid, msg) calls pid._resume(msg) synchronously.
// call(pid, msgFn) sends and reads the reply from a buffer.
let _pidCounter = 0;
let _currentPid = { ch: [], id: 0, _resume: null };

function $spawn(f) {
  const pid = { ch: [], id: ++_pidCounter, _resume: null };
  const prevPid = _currentPid;
  _currentPid = pid;
  f(pid);
  _currentPid = prevPid;
  return pid;
}

function $send(pid, msg) {
  if (pid._resume) {
    const resume = pid._resume;
    pid._resume = null;
    const prevPid = _currentPid;
    _currentPid = pid;
    resume(msg);
    _currentPid = prevPid;
  } else {
    pid.ch.push(msg);
  }
  return null;
}

function $receive_cps(pid, handler) {
  if (pid.ch.length > 0) {
    handler(pid.ch.shift());
  } else {
    pid._resume = handler;
  }
}

function $call(targetPid, msgFn) {
  const replyPid = { ch: [], id: ++_pidCounter, _resume: null };
  const msg = msgFn(replyPid);
  if (targetPid._resume) {
    const resume = targetPid._resume;
    targetPid._resume = null;
    const prevPid = _currentPid;
    _currentPid = targetPid;
    resume(msg);
    _currentPid = prevPid;
  } else {
    targetPid.ch.push(msg);
  }
  return replyPid.ch.shift();
}

function $getSelf() { return _currentPid; }

`)
	}

	// Js FFI runtime helpers
	if g.usesJsFfi {
		b.WriteString(`// Std:Js FFI helpers
function $listToArray(lst) {
  const arr = [];
  while (lst !== null && lst.$tag === "Cons") { arr.push(lst.head); lst = lst.tail; }
  return arr;
}
function $jsOk(v) { return {$tag: "Ok", $type: "Result", _0: v}; }
function $jsErr(msg) { return {$tag: "Err", $type: "Result", _0: msg}; }
function $jsGlobal(name) {
  try { const v = globalThis[name]; if (v === undefined) return $jsErr("global not found: " + name); return $jsOk(v); }
  catch(e) { return $jsErr(e.message); }
}
function $jsGet(prop, obj) {
  try { return $jsOk(obj[prop]); }
  catch(e) { return $jsErr(e.message); }
}
function $jsSet(prop, obj, val) {
  try { obj[prop] = val; return $jsOk(null); }
  catch(e) { return $jsErr(e.message); }
}
function $jsCall(method, args, obj) {
  try { return $jsOk(obj[method](...$listToArray(args))); }
  catch(e) { return $jsErr(e.message); }
}
function $jsNew(name, args) {
  try { const C = globalThis[name]; if (!C) return $jsErr("constructor not found: " + name); return $jsOk(new C(...$listToArray(args))); }
  catch(e) { return $jsErr(e.message); }
}
function $jsCallback(f) {
  return (function() { return f(arguments[0] !== undefined ? arguments[0] : null); });
}
function $jsToString(v) {
  if (typeof v === "string") return $jsOk(v);
  return $jsErr("expected string, got " + typeof v);
}
function $jsToInt(v) {
  if (typeof v === "number" && Number.isInteger(v)) return $jsOk(v);
  return $jsErr("expected integer, got " + typeof v);
}
function $jsToFloat(v) {
  if (typeof v === "number") return $jsOk(v);
  return $jsErr("expected number, got " + typeof v);
}
function $jsToBool(v) {
  if (typeof v === "boolean") return $jsOk(v);
  return $jsErr("expected boolean, got " + typeof v);
}

`)
	}

	// (Virtual DOM runtime is now implemented in Html.browser.rex overlay)

	return b.String()
}

// ---------------------------------------------------------------------------
// Trait dispatch
// ---------------------------------------------------------------------------

func (g *jsGen) emitTraitDispatchers() string {
	var b strings.Builder

	// First emit impl functions
	// (these are emitted in emitDecl, but we need dispatch functions here)
	// Dispatch functions will be emitted after impl functions.
	// Let's collect all unique dispatch names.
	dispatchers := make(map[string]bool)
	for key := range g.traitImpls {
		dispatchers[key] = true
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Declarations
// ---------------------------------------------------------------------------

func (g *jsGen) emitDecl(d ir.Decl) error {
	switch d := d.(type) {
	case ir.DLet:
		return g.emitDLet(d)
	case ir.DLetRec:
		return g.emitDLetRec(d)
	case ir.DType:
		// No type definitions needed in JS
		return nil
	case ir.DTrait:
		return nil
	case ir.DImpl:
		return g.emitDImpl(d)
	case ir.DImport:
		return nil
	case ir.DTest:
		return nil
	}
	return nil
}

func (g *jsGen) emitDLet(d ir.DLet) error {
	// _ bindings: side-effect-only, always emit as IIFE
	if d.Name == "_" {
		g.wn("(() => {\n")
		g.indent++
		g.locals = make(map[string]bool)
		if err := g.emitExprStmt(d.Body, true); err != nil {
			return err
		}
		g.indent--
		g.w("})();\n")
		return nil
	}

	fi := g.funcs[d.Name]
	if fi == nil {
		return nil
	}

	jsName := jsFuncName(d.Name)

	if fi.arity == 0 {
		// Top-level value (not a function) — emit as const
		g.wn("const %s = ", jsName)
		g.locals = make(map[string]bool)
		if err := g.emitExprInline(fi.body); err != nil {
			return err
		}
		g.buf.WriteString(";\n\n")
		return nil
	}

	// Function
	g.wn("function %s(", jsName)
	wildcardIdx := 0
	for i, p := range fi.params {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		pname := p.name
		if pname == "_" {
			pname = fmt.Sprintf("_%d", wildcardIdx)
			wildcardIdx++
		}
		g.buf.WriteString(jsVarName(pname))
	}
	g.buf.WriteString(") {\n")

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

func (g *jsGen) emitDLetRec(d ir.DLetRec) error {
	for _, b := range d.Bindings {
		fi := g.funcs[b.Name]
		if fi == nil {
			continue
		}
		jsName := jsFuncName(b.Name)
		g.wn("function %s(", jsName)
		wildcardIdx := 0
		for i, p := range fi.params {
			if i > 0 {
				g.buf.WriteString(", ")
			}
			pname := p.name
			if pname == "_" {
				pname = fmt.Sprintf("_%d", wildcardIdx)
				wildcardIdx++
			}
			g.buf.WriteString(jsVarName(pname))
		}
		g.buf.WriteString(") {\n")

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

func (g *jsGen) emitDImpl(d ir.DImpl) error {
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

		fmt.Fprintf(g.buf, "function %s(", funcName)
		for i, p := range params {
			if i > 0 {
				g.buf.WriteString(", ")
			}
			g.buf.WriteString(jsVarName(p))
		}
		g.buf.WriteString(") {\n")
		g.indent = 1
		g.locals = make(map[string]bool)
		for _, p := range params {
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

func (g *jsGen) emitAllDispatchers() {
	// Collect all unique dispatch names and emit one function per method
	emitted := make(map[string]bool)
	for key := range g.traitImpls {
		// key = "TraitName:methodName"
		parts := strings.SplitN(key, ":", 2)
		if len(parts) != 2 {
			continue
		}
		methodName := parts[1]
		dispatchName := g.traitMethodNames[methodName]
		if dispatchName == "" || emitted[dispatchName] {
			continue
		}
		emitted[dispatchName] = true
		// Filter cases to only include types defined in the program
		var filtered []jsImplCase
		for _, c := range g.traitImpls[key] {
			if g.knownTypes[c.typeName] {
				filtered = append(filtered, c)
			}
		}
		if len(filtered) > 0 {
			g.emitDispatchFunction(dispatchName, filtered)
		}
	}
}

func (g *jsGen) emitDispatchFunction(name string, cases []jsImplCase) {
	// We need to emit a curried dispatch function.
	// The first arg is the value to dispatch on.
	fmt.Fprintf(g.buf, "function %s(x) {\n", name)
	g.indent = 1
	for _, c := range cases {
		switch c.typeName {
		case "Int":
			g.w(`if (typeof x === "number" && Number.isInteger(x)) return %s(x);`, c.funcName)
		case "Float":
			g.w(`if (typeof x === "number" && !Number.isInteger(x)) return %s(x);`, c.funcName)
		case "String":
			g.w(`if (typeof x === "string") return %s(x);`, c.funcName)
		case "Bool":
			g.w(`if (typeof x === "boolean") return %s(x);`, c.funcName)
		case "List":
			g.w(`if (x === null || (typeof x === "object" && (x.$tag === "Cons" || x.$tag === "Nil"))) return %s(x);`, c.funcName)
		case "Unit":
			g.w(`if (x === null) return %s(x);`, c.funcName)
		default:
			// ADT or record — check $type field
			g.w(`if (typeof x === "object" && x !== null && x.$type === %q) return %s(x);`, c.typeName, c.funcName)
		}
	}
	g.w(`throw new Error("No trait instance for: " + $display(x));`)
	g.indent = 0
	g.buf.WriteString("}\n\n")
}

// ---------------------------------------------------------------------------
// Emit expressions
// ---------------------------------------------------------------------------

func (g *jsGen) emitExprStmt(expr ir.Expr, isReturn bool) error {
	switch e := expr.(type) {
	case ir.EAtom:
		if isReturn {
			g.wn("return ")
			g.emitAtom(e.A)
			g.buf.WriteString(";\n")
		} else {
			g.wn("")
			g.emitAtom(e.A)
			g.buf.WriteString(";\n")
		}
		return nil

	case ir.EComplex:
		return g.emitCExprStmt(e.C, isReturn)

	case ir.ELet:
		// CPS transform for receive(pid): instead of blocking, set _resume callback
		if g.usesConcurrency && isReceiveCall(e.Bind) {
			varName := jsVarName(e.Name)
			// Extract the pid argument from receive(pid)
			app := e.Bind.(ir.CApp)
			pidArg := g.atomStr(app.Arg)
			g.w("$receive_cps(%s, (%s) => {", pidArg, varName)
			g.indent++
			g.locals[e.Name] = true
			if err := g.emitExprStmt(e.Body, true); err != nil {
				return err
			}
			g.indent--
			g.w("});")
			return nil
		}

		varName := jsVarName(e.Name)
		if e.Name == "_" {
			g.wn("")
		} else {
			g.wn("const %s = ", varName)
		}
		if err := g.emitCExprInline(e.Bind); err != nil {
			return err
		}
		g.buf.WriteString(";\n")
		g.locals[e.Name] = true
		return g.emitExprStmt(e.Body, isReturn)

	case ir.ELetRec:
		// Declare variables first, then assign (for mutual recursion)
		for _, b := range e.Bindings {
			g.w("let %s;", jsVarName(b.Name))
			g.locals[b.Name] = true
		}
		for _, b := range e.Bindings {
			g.wn("%s = ", jsVarName(b.Name))
			if err := g.emitCExprInline(b.Bind); err != nil {
				return err
			}
			g.buf.WriteString(";\n")
		}
		return g.emitExprStmt(e.Body, isReturn)
	}
	return fmt.Errorf("unknown expr type: %T", expr)
}

func (g *jsGen) emitExprInline(expr ir.Expr) error {
	switch e := expr.(type) {
	case ir.EAtom:
		g.emitAtom(e.A)
		return nil
	case ir.EComplex:
		return g.emitCExprInline(e.C)
	default:
		// Complex expressions that need statements: wrap in IIFE
		g.buf.WriteString("(() => {\n")
		g.indent++
		if err := g.emitExprStmt(expr, true); err != nil {
			return err
		}
		g.indent--
		for i := 0; i < g.indent; i++ {
			g.buf.WriteString("  ")
		}
		g.buf.WriteString("})()")
		return nil
	}
}

func (g *jsGen) emitCExprStmt(c ir.CExpr, isReturn bool) error {
	switch c := c.(type) {
	case ir.CIf:
		g.w("if (%s) {", g.atomStr(c.Cond))
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
		g.buf.WriteString(";\n")
		return nil
	}
}

func (g *jsGen) emitCExprInline(c ir.CExpr) error {
	switch c := c.(type) {
	case ir.CApp:
		return g.emitApp(c)

	case ir.CBinop:
		return g.emitBinop(c)

	case ir.CUnaryMinus:
		g.buf.WriteString("(-")
		g.emitAtom(c.Expr)
		g.buf.WriteString(")")
		return nil

	case ir.CIf:
		// Inline if → ternary
		g.buf.WriteString("(")
		g.emitAtom(c.Cond)
		g.buf.WriteString(" ? ")
		if err := g.emitExprInline(c.Then); err != nil {
			return err
		}
		g.buf.WriteString(" : ")
		if err := g.emitExprInline(c.Else); err != nil {
			return err
		}
		g.buf.WriteString(")")
		return nil

	case ir.CMatch:
		// Inline match → IIFE
		g.buf.WriteString("(() => {\n")
		g.indent++
		if err := g.emitMatch(c, true); err != nil {
			return err
		}
		g.indent--
		for i := 0; i < g.indent; i++ {
			g.buf.WriteString("  ")
		}
		g.buf.WriteString("})()")
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

// emitStringBuiltin handles string/math builtins that might be shadowed by user code.
// Returns true if the builtin was emitted, false if funcName is not a known builtin.
func (g *jsGen) emitStringBuiltin(funcName string, c ir.CApp) bool {
	switch funcName {
	// 1-arg string builtins
	case "length":
		g.buf.WriteString("$stringLength(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "toUpper":
		g.buf.WriteString("$toUpper(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "toLower":
		g.buf.WriteString("$toLower(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "trim":
		g.buf.WriteString("$trim(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "trimLeft":
		g.buf.WriteString("$trimLeft(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "trimRight":
		g.buf.WriteString("$trimRight(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "reverse":
		g.buf.WriteString("$stringReverse(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "words":
		g.buf.WriteString("$words(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "lines":
		g.buf.WriteString("$lines(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "charCode":
		g.buf.WriteString("$charCode(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "fromCharCode":
		g.buf.WriteString("$fromCharCode(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "parseInt":
		g.buf.WriteString("$stringParseInt(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "parseFloat":
		g.buf.WriteString("$stringParseFloat(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "toList":
		g.buf.WriteString("$stringToList(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "fromList":
		g.buf.WriteString("$stringFromList(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	case "toFloat":
		g.buf.WriteString("$toFloat(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
	// 2-arg curried string builtins
	case "split":
		g.buf.WriteString("((_s) => $split(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _s))")
	case "join":
		g.buf.WriteString("((_lst) => $join(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _lst))")
	case "contains":
		g.buf.WriteString("((_s) => $contains(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _s))")
	case "startsWith":
		g.buf.WriteString("((_s) => $startsWith(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _s))")
	case "endsWith":
		g.buf.WriteString("((_s) => $endsWith(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _s))")
	case "indexOf":
		g.buf.WriteString("((_s) => $indexOf(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _s))")
	case "charAt":
		g.buf.WriteString("((_s) => $charAt(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _s))")
	case "repeat":
		g.buf.WriteString("((_s) => $repeat(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _s))")
	case "substring":
		g.buf.WriteString("((_end) => ((_s) => $substring(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _end, _s)))")
	// 3-arg curried string builtins
	case "replace":
		g.buf.WriteString("((_to) => ((_s) => $replace(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _to, _s)))")
	case "padLeft":
		g.buf.WriteString("((_ch) => ((_s) => $padLeft(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _ch, _s)))")
	case "padRight":
		g.buf.WriteString("((_ch) => ((_s) => $padRight(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _ch, _s)))")
	default:
		return false
	}
	return true
}

func (g *jsGen) emitApp(c ir.CApp) error {
	funcName := ""
	if v, ok := c.Func.(ir.AVar); ok {
		funcName = v.Name
	}

	// Check if name is a user-defined function or local (shadows builtins)
	_, isUserFunc := g.funcs[funcName]
	isLocal := g.locals[funcName]
	isShadowed := isUserFunc || isLocal

	// Known builtins
	switch funcName {
	case "__id":
		g.emitAtom(c.Arg)
		return nil
	case "println":
		g.buf.WriteString("$println(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "print":
		g.buf.WriteString("$print(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "showInt":
		g.buf.WriteString("$showInt(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "showFloat":
		g.buf.WriteString("$showFloat(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "toString":
		g.buf.WriteString("$display(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "not":
		g.buf.WriteString("$not(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "error":
		g.buf.WriteString("$error(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "todo":
		g.buf.WriteString("$todo(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	}

	// String/math builtins — only match if not shadowed by user-defined function
	if !isShadowed {
		if emitted := g.emitStringBuiltin(funcName, c); emitted {
			return nil
		}
	}

	// Actor builtins
	switch funcName {
	case "spawn":
		g.buf.WriteString("$spawn(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(")")
		return nil
	case "send":
		// send is curried: send pid msg → partial app
		g.buf.WriteString("((_msg) => $send(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _msg))")
		return nil
	case "receive":
		// receive pid — read from pid's channel
		g.emitAtom(c.Arg)
		g.buf.WriteString(".ch.shift()")
		return nil
	case "call":
		// call is curried: call pid fn → partial app
		g.buf.WriteString("((_fn) => $call(")
		g.emitAtom(c.Arg)
		g.buf.WriteString(", _fn))")
		return nil
	}

	// Std:Js FFI builtins (may be module-prefixed as Std$Js$*)
	if short := jsFfiBuiltin(funcName); short != "" {
		switch short {
		case "jsGlobal":
			g.buf.WriteString("$jsGlobal(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
			return nil
		case "jsGet":
			g.buf.WriteString("((_obj) => $jsGet(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(", _obj))")
			return nil
		case "jsSet":
			g.buf.WriteString("((_obj) => ((_val) => $jsSet(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(", _obj, _val)))")
			return nil
		case "jsCall":
			g.buf.WriteString("((_args) => ((_obj) => $jsCall(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(", _args, _obj)))")
			return nil
		case "jsNew":
			g.buf.WriteString("((_args) => $jsNew(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(", _args))")
			return nil
		case "jsCallback":
			g.buf.WriteString("$jsCallback(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
			return nil
		case "jsFromString", "jsFromInt", "jsFromFloat", "jsFromBool":
			g.emitAtom(c.Arg)
			return nil
		case "jsToString":
			g.buf.WriteString("$jsToString(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
			return nil
		case "jsToInt":
			g.buf.WriteString("$jsToInt(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
			return nil
		case "jsToFloat":
			g.buf.WriteString("$jsToFloat(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
			return nil
		case "jsToBool":
			g.buf.WriteString("$jsToBool(")
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
			return nil
		}
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
		jsName := jsFuncName(funcName)
		if fi.arity == 1 {
			fmt.Fprintf(g.buf, "%s(", jsName)
			g.emitAtom(c.Arg)
			g.buf.WriteString(")")
		} else {
			g.emitPartialApp(jsName, fi.arity, c.Arg)
		}
		return nil
	}

	// Unknown / variable function: use $$apply
	g.buf.WriteString("$$apply(")
	g.emitAtom(c.Func)
	g.buf.WriteString(", ")
	g.emitAtom(c.Arg)
	g.buf.WriteString(")")
	return nil
}

func (g *jsGen) emitPartialApp(jsName string, arity int, firstArg ir.Atom) {
	remaining := arity - 1
	var params []string
	for i := 0; i < remaining; i++ {
		param := fmt.Sprintf("_pa%d", i)
		params = append(params, param)
		fmt.Fprintf(g.buf, "((%s) => ", param)
	}
	fmt.Fprintf(g.buf, "%s(", jsName)
	g.emitAtom(firstArg)
	for _, p := range params {
		fmt.Fprintf(g.buf, ", %s", p)
	}
	g.buf.WriteString(")")
	for range params {
		g.buf.WriteString(")")
	}
}

// ---------------------------------------------------------------------------
// Binary operators
// ---------------------------------------------------------------------------

func (g *jsGen) emitBinop(c ir.CBinop) error {
	switch c.Op {
	case "Add":
		g.buf.WriteString("(")
		g.emitAtom(c.Left)
		g.buf.WriteString(" + ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Sub":
		g.buf.WriteString("(")
		g.emitAtom(c.Left)
		g.buf.WriteString(" - ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Mul":
		g.buf.WriteString("(")
		g.emitAtom(c.Left)
		g.buf.WriteString(" * ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Div":
		if isFloatType(c.Ty) {
			g.buf.WriteString("(")
			g.emitAtom(c.Left)
			g.buf.WriteString(" / ")
			g.emitAtom(c.Right)
			g.buf.WriteString(")")
		} else {
			// Integer division: Math.trunc
			g.buf.WriteString("Math.trunc(")
			g.emitAtom(c.Left)
			g.buf.WriteString(" / ")
			g.emitAtom(c.Right)
			g.buf.WriteString(")")
		}
		return nil
	case "Mod":
		g.buf.WriteString("(")
		g.emitAtom(c.Left)
		g.buf.WriteString(" % ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Eq":
		g.buf.WriteString("$eq(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Neq":
		g.buf.WriteString("!$eq(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Lt":
		g.buf.WriteString("($compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") < 0)")
		return nil
	case "Gt":
		g.buf.WriteString("($compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") > 0)")
		return nil
	case "Leq":
		g.buf.WriteString("($compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") <= 0)")
		return nil
	case "Geq":
		g.buf.WriteString("($compare(")
		g.emitAtom(c.Left)
		g.buf.WriteString(", ")
		g.emitAtom(c.Right)
		g.buf.WriteString(") >= 0)")
		return nil
	case "And":
		g.buf.WriteString("(")
		g.emitAtom(c.Left)
		g.buf.WriteString(" && ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Or":
		g.buf.WriteString("(")
		g.emitAtom(c.Left)
		g.buf.WriteString(" || ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Concat":
		g.buf.WriteString("(")
		g.emitAtom(c.Left)
		g.buf.WriteString(" + ")
		g.emitAtom(c.Right)
		g.buf.WriteString(")")
		return nil
	case "Cons":
		g.buf.WriteString(`{$tag: "Cons", head: `)
		g.emitAtom(c.Left)
		g.buf.WriteString(", tail: ")
		g.emitAtom(c.Right)
		g.buf.WriteString("}")
		return nil
	}
	return fmt.Errorf("unknown binop: %s", c.Op)
}

// ---------------------------------------------------------------------------
// Lambda / closure
// ---------------------------------------------------------------------------

func (g *jsGen) emitLambda(c ir.CLambda) error {
	param := jsVarName(c.Param)
	fmt.Fprintf(g.buf, "((%s) => {\n", param)
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
		g.buf.WriteString("  ")
	}
	g.buf.WriteString("})")
	g.locals = oldLocals
	return nil
}

// ---------------------------------------------------------------------------
// ADT constructors
// ---------------------------------------------------------------------------

func (g *jsGen) emitCtor(c ir.CCtor) error {
	ci, ok := g.ctorToAdt[c.Name]
	if !ok {
		return fmt.Errorf("unknown constructor: %s", c.Name)
	}
	if len(c.Args) == 0 {
		fmt.Fprintf(g.buf, `{$tag: %q, $type: %q}`, c.Name, ci.typeName)
		return nil
	}
	fmt.Fprintf(g.buf, `{$tag: %q, $type: %q`, c.Name, ci.typeName)
	for i, a := range c.Args {
		fmt.Fprintf(g.buf, ", _%d: ", i)
		g.emitAtom(a)
	}
	g.buf.WriteString("}")
	return nil
}

// ---------------------------------------------------------------------------
// Records
// ---------------------------------------------------------------------------

func (g *jsGen) emitRecord(c ir.CRecord) error {
	g.buf.WriteString("{")
	for i, f := range c.Fields {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		fmt.Fprintf(g.buf, "%s: ", f.Name)
		g.emitAtom(f.Value)
	}
	g.buf.WriteString("}")
	return nil
}

func (g *jsGen) emitFieldAccess(c ir.CFieldAccess) error {
	g.emitAtom(c.Record)
	fmt.Fprintf(g.buf, ".%s", c.Field)
	return nil
}

func (g *jsGen) emitRecordUpdate(c ir.CRecordUpdate) error {
	g.buf.WriteString("{...")
	g.emitAtom(c.Record)
	for _, u := range c.Updates {
		if len(u.Path) == 1 {
			fmt.Fprintf(g.buf, ", %s: ", u.Path[0])
			g.emitAtom(u.Value)
		}
		// TODO: nested paths
	}
	g.buf.WriteString("}")
	return nil
}

// ---------------------------------------------------------------------------
// Lists
// ---------------------------------------------------------------------------

func (g *jsGen) emitList(c ir.CList) error {
	if len(c.Items) == 0 {
		g.buf.WriteString("null")
		return nil
	}
	// Build cons list from right to left
	g.buf.WriteString(`{$tag: "Cons", head: `)
	g.emitAtom(c.Items[0])
	for i := 1; i < len(c.Items); i++ {
		g.buf.WriteString(`, tail: {$tag: "Cons", head: `)
		g.emitAtom(c.Items[i])
	}
	g.buf.WriteString(", tail: null")
	for i := 1; i < len(c.Items); i++ {
		g.buf.WriteString("}")
	}
	g.buf.WriteString("}")
	return nil
}

// ---------------------------------------------------------------------------
// Tuples
// ---------------------------------------------------------------------------

func (g *jsGen) emitTuple(c ir.CTuple) error {
	if len(c.Items) == 0 {
		g.buf.WriteString("null") // unit
		return nil
	}
	g.buf.WriteString("[")
	for i, item := range c.Items {
		if i > 0 {
			g.buf.WriteString(", ")
		}
		g.emitAtom(item)
	}
	g.buf.WriteString("]")
	return nil
}

// ---------------------------------------------------------------------------
// String interpolation
// ---------------------------------------------------------------------------

func (g *jsGen) emitStringInterp(c ir.CStringInterp) error {
	if len(c.Parts) == 0 {
		g.buf.WriteString(`""`)
		return nil
	}
	if len(c.Parts) == 1 {
		g.buf.WriteString("$display(")
		g.emitAtom(c.Parts[0])
		g.buf.WriteString(")")
		return nil
	}
	parts := make([]string, 0, len(c.Parts))
	for _, p := range c.Parts {
		if s, ok := p.(ir.AString); ok {
			parts = append(parts, fmt.Sprintf("%q", s.Value))
		} else {
			parts = append(parts, fmt.Sprintf("$display(%s)", g.atomStr(p)))
		}
	}
	g.buf.WriteString(strings.Join(parts, " + "))
	return nil
}

// ---------------------------------------------------------------------------
// Pattern matching
// ---------------------------------------------------------------------------

func (g *jsGen) emitMatch(c ir.CMatch, isReturn bool) error {
	scrutVar := g.atomStr(c.Scrutinee)

	for i, arm := range c.Arms {
		cond, bindings := g.patternCondition(scrutVar, arm.Pat)

		if cond == "" || cond == "true" {
			// Unconditional (wildcard, variable)
			if i > 0 {
				g.w("} else {")
			}
			g.indent++
			for _, b := range bindings {
				g.w("const %s = %s;", jsVarName(b.name), b.expr)
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
			g.w("if (%s) {", cond)
		} else {
			g.w("} else if (%s) {", cond)
		}
		g.indent++
		for _, b := range bindings {
			g.w("const %s = %s;", jsVarName(b.name), b.expr)
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
		g.w(`throw new Error("non-exhaustive match");`)
		g.indent--
		g.w("}")
	}
	return nil
}

type jsPatBinding struct {
	name string
	expr string
}

func (g *jsGen) patternCondition(scrutExpr string, pat ir.Pattern) (string, []jsPatBinding) {
	switch p := pat.(type) {
	case ir.PWild:
		return "true", nil

	case ir.PVar:
		return "true", []jsPatBinding{{name: p.Name, expr: scrutExpr}}

	case ir.PInt:
		return fmt.Sprintf("$eq(%s, %d)", scrutExpr, p.Value), nil

	case ir.PFloat:
		return fmt.Sprintf("$eq(%s, %g)", scrutExpr, p.Value), nil

	case ir.PString:
		return fmt.Sprintf("$eq(%s, %q)", scrutExpr, p.Value), nil

	case ir.PBool:
		if p.Value {
			return fmt.Sprintf("(%s === true)", scrutExpr), nil
		}
		return fmt.Sprintf("(%s === false)", scrutExpr), nil

	case ir.PUnit:
		return "true", nil

	case ir.PNil:
		return fmt.Sprintf("(%s === null)", scrutExpr), nil

	case ir.PCons:
		cond := fmt.Sprintf("(%s !== null && %s.$tag === \"Cons\")", scrutExpr, scrutExpr)
		headExpr := fmt.Sprintf("%s.head", scrutExpr)
		tailExpr := fmt.Sprintf("%s.tail", scrutExpr)

		headCond, headBindings := g.patternCondition(headExpr, p.Head)
		tailCond, tailBindings := g.patternCondition(tailExpr, p.Tail)

		var allConds []string
		allConds = append(allConds, cond)
		if headCond != "" && headCond != "true" {
			allConds = append(allConds, headCond)
		}
		if tailCond != "" && tailCond != "true" {
			allConds = append(allConds, tailCond)
		}

		var bindings []jsPatBinding
		bindings = append(bindings, headBindings...)
		bindings = append(bindings, tailBindings...)

		return strings.Join(allConds, " && "), bindings

	case ir.PTuple:
		var conds []string
		var bindings []jsPatBinding
		for i, subPat := range p.Pats {
			fieldExpr := fmt.Sprintf("%s[%d]", scrutExpr, i)
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
		_ = ci

		var conds []string
		var bindings []jsPatBinding
		conds = append(conds, fmt.Sprintf("(typeof %s === \"object\" && %s !== null && %s.$tag === %q)", scrutExpr, scrutExpr, scrutExpr, p.Name))

		for i, subPat := range p.Args {
			fieldExpr := fmt.Sprintf("%s._%d", scrutExpr, i)
			c, b := g.patternCondition(fieldExpr, subPat)
			if c != "" && c != "true" {
				conds = append(conds, c)
			}
			bindings = append(bindings, b...)
		}

		return strings.Join(conds, " && "), bindings

	case ir.PRecord:
		var bindings []jsPatBinding
		var conds []string
		for _, f := range p.Fields {
			fieldExpr := fmt.Sprintf("%s.%s", scrutExpr, f.Name)
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

// ---------------------------------------------------------------------------
// Atoms
// ---------------------------------------------------------------------------

func (g *jsGen) emitAtom(a ir.Atom) {
	g.buf.WriteString(g.atomStr(a))
}

func (g *jsGen) atomStr(a ir.Atom) string {
	switch a := a.(type) {
	case ir.AInt:
		return fmt.Sprintf("%d", a.Value)
	case ir.AFloat:
		return fmt.Sprintf("%g", a.Value)
	case ir.AString:
		return fmt.Sprintf("%q", a.Value)
	case ir.ABool:
		if a.Value {
			return "true"
		}
		return "false"
	case ir.AUnit:
		return "null"
	case ir.AVar:
		name := a.Name
		// Actor builtins as values
		if g.usesConcurrency {
			switch name {
			case "receive":
				return "((pid) => pid.ch.shift())"
			case "spawn":
				return "((f) => $spawn(f))"
			case "send":
				return "((pid) => (msg) => $send(pid, msg))"
			case "call":
				return "((pid) => (fn) => $call(pid, fn))"
			}
		}
		// Js FFI builtins as values
		if g.usesJsFfi {
			if short := jsFfiBuiltin(name); short != "" {
				switch short {
				case "jsGlobal":
					return "((n) => $jsGlobal(n))"
				case "jsGet":
					return "((p) => (o) => $jsGet(p, o))"
				case "jsSet":
					return "((p) => (o) => (v) => $jsSet(p, o, v))"
				case "jsCall":
					return "((m) => (a) => (o) => $jsCall(m, a, o))"
				case "jsNew":
					return "((n) => (a) => $jsNew(n, a))"
				case "jsCallback":
					return "((f) => $jsCallback(f))"
				case "jsFromString", "jsFromInt", "jsFromFloat", "jsFromBool":
					return "((v) => v)"
				case "jsToString":
					return "((v) => $jsToString(v))"
				case "jsToInt":
					return "((v) => $jsToInt(v))"
				case "jsToFloat":
					return "((v) => $jsToFloat(v))"
				case "jsToBool":
					return "((v) => $jsToBool(v))"
				case "jsNull":
					return "null"
				}
			}
		}
		// Builtins as values
		switch name {
		case "println":
			return "((v) => $println(v))"
		case "print":
			return "((v) => $print(v))"
		case "toString":
			return "((v) => $display(v))"
		case "showInt":
			return "((v) => $showInt(v))"
		case "showFloat":
			return "((v) => $showFloat(v))"
		case "not":
			return "((v) => $not(v))"
		case "error":
			return "((v) => $error(v))"
		}
		// Check if it's a trait method
		if dispatchName, ok := g.traitMethodNames[name]; ok {
			return fmt.Sprintf("((a) => %s(a))", dispatchName)
		}
		// Check if it's a known ADT constructor
		if ci, ok := g.ctorToAdt[name]; ok {
			if len(ci.fieldTypes) == 0 {
				return fmt.Sprintf(`{$tag: %q, $type: %q}`, name, ci.typeName)
			}
			return g.ctorAsClosure(ci)
		}
		// Check if it's a known record constructor
		if ri, ok := g.records[name]; ok {
			return g.recordCtorAsClosure(ri)
		}
		// Check if it's a known top-level function
		if fi, ok := g.funcs[name]; ok {
			if !g.locals[name] {
				if fi.arity > 0 {
					return g.funcAsClosure(name, fi)
				}
				return jsFuncName(name)
			}
		}
		// String/math builtins as values (after user-defined names)
		switch name {
		case "length":
			return "((s) => $stringLength(s))"
		case "toUpper":
			return "((s) => $toUpper(s))"
		case "toLower":
			return "((s) => $toLower(s))"
		case "trim":
			return "((s) => $trim(s))"
		case "trimLeft":
			return "((s) => $trimLeft(s))"
		case "trimRight":
			return "((s) => $trimRight(s))"
		case "reverse":
			return "((s) => $stringReverse(s))"
		case "words":
			return "((s) => $words(s))"
		case "lines":
			return "((s) => $lines(s))"
		case "charCode":
			return "((s) => $charCode(s))"
		case "fromCharCode":
			return "((n) => $fromCharCode(n))"
		case "parseInt":
			return "((s) => $stringParseInt(s))"
		case "parseFloat":
			return "((s) => $stringParseFloat(s))"
		case "toList":
			return "((s) => $stringToList(s))"
		case "fromList":
			return "((lst) => $stringFromList(lst))"
		case "toFloat":
			return "((n) => $toFloat(n))"
		case "split":
			return "((sep) => (s) => $split(sep, s))"
		case "join":
			return "((sep) => (lst) => $join(sep, lst))"
		case "contains":
			return "((sub) => (s) => $contains(sub, s))"
		case "startsWith":
			return "((pfx) => (s) => $startsWith(pfx, s))"
		case "endsWith":
			return "((sfx) => (s) => $endsWith(sfx, s))"
		case "indexOf":
			return "((sub) => (s) => $indexOf(sub, s))"
		case "charAt":
			return "((i) => (s) => $charAt(i, s))"
		case "repeat":
			return "((n) => (s) => $repeat(n, s))"
		case "substring":
			return "((start) => (end) => (s) => $substring(start, end, s))"
		case "replace":
			return "((from) => (to) => (s) => $replace(from, to, s))"
		case "padLeft":
			return "((n) => (ch) => (s) => $padLeft(n, ch, s))"
		case "padRight":
			return "((n) => (ch) => (s) => $padRight(n, ch, s))"
		}
		return jsVarName(name)
	}
	return "null"
}

func (g *jsGen) ctorAsClosure(ci *jsCtorInfo) string {
	n := len(ci.fieldTypes)
	if n == 1 {
		return fmt.Sprintf(`((a0) => ({$tag: %q, $type: %q, _0: a0}))`, ci.name, ci.typeName)
	}
	// Multi-field: curry
	var params []string
	for i := 0; i < n; i++ {
		params = append(params, fmt.Sprintf("a%d", i))
	}
	var fields []string
	fields = append(fields, fmt.Sprintf("$tag: %q", ci.name))
	fields = append(fields, fmt.Sprintf("$type: %q", ci.typeName))
	for i, p := range params {
		fields = append(fields, fmt.Sprintf("_%d: %s", i, p))
	}
	result := fmt.Sprintf("({%s})", strings.Join(fields, ", "))
	for i := n - 1; i >= 0; i-- {
		result = fmt.Sprintf("((%s) => %s)", params[i], result)
	}
	return result
}

func (g *jsGen) recordCtorAsClosure(ri *jsRecordInfo) string {
	n := len(ri.fieldNames)
	if n == 0 {
		return fmt.Sprintf("({$type: %q})", ri.name)
	}
	params := make([]string, n)
	for i := range params {
		params[i] = fmt.Sprintf("a%d", i)
	}
	fields := []string{fmt.Sprintf("$type: %q", ri.name)}
	for i, fname := range ri.fieldNames {
		fields = append(fields, fmt.Sprintf("%s: %s", fname, params[i]))
	}
	result := fmt.Sprintf("({%s})", strings.Join(fields, ", "))
	for i := n - 1; i >= 0; i-- {
		result = fmt.Sprintf("((%s) => %s)", params[i], result)
	}
	return result
}

func (g *jsGen) funcAsClosure(name string, fi *jsFuncInfo) string {
	jsName := jsFuncName(name)
	if fi.arity == 1 {
		return fmt.Sprintf("((a) => %s(a))", jsName)
	}
	// Multi-arg: curry
	var params []string
	for i := 0; i < fi.arity; i++ {
		params = append(params, fmt.Sprintf("_a%d", i))
	}
	result := fmt.Sprintf("%s(%s)", jsName, strings.Join(params, ", "))
	for i := fi.arity - 1; i >= 0; i-- {
		result = fmt.Sprintf("((%s) => %s)", params[i], result)
	}
	return result
}

// ---------------------------------------------------------------------------
// Name mangling
// ---------------------------------------------------------------------------

var jsReserved = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "export": true, "extends": true, "false": true, "finally": true,
	"for": true, "function": true, "if": true, "import": true, "in": true,
	"instanceof": true, "let": true, "new": true, "null": true, "return": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "undefined": true, "var": true, "void": true,
	"while": true, "with": true, "yield": true, "await": true, "enum": true,
	"implements": true, "interface": true, "package": true, "private": true,
	"protected": true, "public": true, "static": true, "arguments": true,
}

func jsFuncName(name string) string {
	// Module-prefixed names (containing $) already have their prefix from ModulePrefix
	if strings.Contains(name, "$") {
		return jsSanitize(name)
	}
	return "$" + jsSanitize(name)
}

func jsVarName(name string) string {
	s := jsSanitize(name)
	if jsReserved[s] {
		return "$" + s
	}
	return s
}

// isReceiveCall checks if a CExpr is a call to the "receive" builtin.
func isReceiveCall(c ir.CExpr) bool {
	if app, ok := c.(ir.CApp); ok {
		if v, ok := app.Func.(ir.AVar); ok {
			return v.Name == "receive"
		}
	}
	return false
}

func jsSanitize(name string) string {
	var b strings.Builder
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '_', c == '$':
			b.WriteRune(c)
		case c == '\'':
			b.WriteString("$prime")
		default:
			fmt.Fprintf(&b, "$%d", c)
		}
	}
	return b.String()
}

// EmitBrowserHTML generates a minimal HTML wrapper for a browser JS app.
func EmitBrowserHTML(jsFileName string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Rex App</title>
</head>
<body>
  <div id="app"></div>
  <script src="%s"></script>
</body>
</html>
`, jsFileName)
}
