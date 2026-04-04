// Package codegen — library check support.
//
// GenerateLibRuntime and GenerateLibTypes produce Go source files that let
// library companions compile during development (before being absorbed into
// an app build). See docs/go-ffi-library-dev.md for the full design.
package codegen

import (
	"fmt"
	"strings"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/types"
)

// GenerateLibRuntime returns the content of rex_runtime.go — the minimal set
// of runtime types that companion code may reference. Package name comes from
// rex.toml [package] name.
func GenerateLibRuntime(pkgName string) string {
	return fmt.Sprintf(`package %s

type RexList struct {
	Head any
	Tail *RexList
}

type Tuple2 struct{ F0, F1 any }
type Tuple3 struct{ F0, F1, F2 any }
type Tuple4 struct{ F0, F1, F2, F3 any }
`, pkgName)
}

// GenerateLibTypes returns the content of a <Module>.types.go file containing
// Go type definitions for ADTs and records declared in a single IR module.
// Only modules with type declarations produce output; returns "" if the module
// has no types.
func GenerateLibTypes(pkgName string, decls []ir.Decl) string {
	var adts []goAdtInfo
	var records []goRecordInfo

	for _, d := range decls {
		dt, ok := d.(ir.DType)
		if !ok {
			continue
		}
		if len(dt.Fields) > 0 {
			// Record
			ri := goRecordInfo{name: dt.Name, origName: dt.Name}
			for _, f := range dt.Fields {
				ri.fieldNames = append(ri.fieldNames, f.Name)
				ri.fieldTypes = append(ri.fieldTypes, f.Ty)
			}
			records = append(records, ri)
		} else if len(dt.Ctors) > 0 {
			// ADT
			ai := goAdtInfo{name: dt.Name}
			for i, c := range dt.Ctors {
				ci := goCtorInfo{
					name:     c.Name,
					tag:      i,
					typeName: dt.Name,
				}
				for _, t := range c.ArgTypes {
					ci.fieldTypes = append(ci.fieldTypes, t)
				}
				ai.ctors = append(ai.ctors, ci)
			}
			adts = append(adts, ai)
		}
	}

	if len(adts) == 0 && len(records) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "package %s\n\n", pkgName)

	for _, adt := range adts {
		ifaceName := goTypeName(adt.name)
		fmt.Fprintf(&b, "type %s interface{ tag%s() int }\n\n", ifaceName, ifaceName)

		for _, ctor := range adt.ctors {
			structName := goCtorStructName(ctor.typeName, ctor.name)
			if len(ctor.fieldTypes) == 0 {
				fmt.Fprintf(&b, "type %s struct{}\n", structName)
			} else {
				fmt.Fprintf(&b, "type %s struct {\n", structName)
				for i, ft := range ctor.fieldTypes {
					fmt.Fprintf(&b, "\tF%d %s\n", i, libGoType(ft))
				}
				b.WriteString("}\n")
			}
			fmt.Fprintf(&b, "func (%s) tag%s() int { return %d }\n\n",
				structName, ifaceName, ctor.tag)
		}
	}

	for _, rec := range records {
		structName := goTypeName(rec.name)
		fmt.Fprintf(&b, "type %s struct {\n", structName)
		for i, fname := range rec.fieldNames {
			fmt.Fprintf(&b, "\t%s %s\n", goExportedField(fname), libGoType(rec.fieldTypes[i]))
		}
		b.WriteString("}\n\n")
	}

	return b.String()
}

// GenerateJSPrelude returns the contents of rex_prelude.mjs — runtime helpers
// for JS companions (list conversion, Maybe/Result constructors, actor bridge).
func GenerateJSPrelude() string {
	return `// rex_prelude.mjs — generated, do not edit

// -- Lists --

export function listToArray(lst) {
    const arr = [];
    while (lst !== null) {
        arr.push(lst.head);
        lst = lst.tail;
    }
    return arr;
}

export function arrayToList(arr) {
    let lst = null;
    for (let i = arr.length - 1; i >= 0; i--) {
        lst = { $tag: "Cons", head: arr[i], tail: lst };
    }
    return lst;
}

export function cons(head, tail) {
    return { $tag: "Cons", head, tail };
}

export const nil = null;

// -- Maybe --

export function just(val) {
    return { $tag: "Just", $type: "Maybe", _0: val };
}

export const nothing = { $tag: "Nothing", $type: "Maybe" };

// -- Result --

export function ok(val) {
    return { $tag: "Ok", $type: "Result", _0: val };
}

export function err(msg) {
    return { $tag: "Err", $type: "Result", _0: msg };
}

// -- Tuples --

export function tuple2(a, b) { return [a, b]; }
export function tuple3(a, b, c) { return [a, b, c]; }

// -- Unit --

export const unit = null;

// -- Actor bridge --

export function send(pid, msg) {
    if (pid._resume) {
        const fn = pid._resume;
        pid._resume = null;
        fn(msg);
    } else {
        pid.ch.push(msg);
    }
}
`
}

// GenerateJSTypes returns the contents of a <Module>.types.mjs file containing
// JS constructor functions for ADTs and records declared in a single IR module.
// Only modules with type declarations produce output; returns "" if the module
// has no types.
func GenerateJSTypes(moduleName string, decls []ir.Decl) string {
	var adts []jsAdtTypeInfo
	var records []jsRecordTypeInfo

	for _, d := range decls {
		dt, ok := d.(ir.DType)
		if !ok {
			continue
		}
		if len(dt.Fields) > 0 {
			// Record
			ri := jsRecordTypeInfo{name: dt.Name}
			for _, f := range dt.Fields {
				ri.fieldNames = append(ri.fieldNames, f.Name)
			}
			records = append(records, ri)
		} else if len(dt.Ctors) > 0 {
			// ADT
			ai := jsAdtTypeInfo{name: dt.Name}
			for _, c := range dt.Ctors {
				ci := jsCtorTypeInfo{
					name:     c.Name,
					typeName: dt.Name,
					arity:    len(c.ArgTypes),
				}
				ai.ctors = append(ai.ctors, ci)
			}
			adts = append(adts, ai)
		}
	}

	if len(adts) == 0 && len(records) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "// %s.types.mjs — generated, do not edit\n\n", moduleName)

	for _, adt := range adts {
		fmt.Fprintf(&b, "// type %s\n", adt.name)
		for _, ctor := range adt.ctors {
			if ctor.arity == 0 {
				fmt.Fprintf(&b, "export const %s = { $tag: \"%s\", $type: \"%s\" };\n",
					ctor.name, ctor.name, ctor.typeName)
			} else {
				params := make([]string, ctor.arity)
				fields := make([]string, ctor.arity)
				for i := range ctor.arity {
					params[i] = fmt.Sprintf("a%d", i)
					fields[i] = fmt.Sprintf("_%d: a%d", i, i)
				}
				fmt.Fprintf(&b, "export function %s(%s) {\n    return { $tag: \"%s\", $type: \"%s\", %s };\n}\n",
					ctor.name, strings.Join(params, ", "),
					ctor.name, ctor.typeName,
					strings.Join(fields, ", "))
			}
		}
		b.WriteString("\n")
	}

	for _, rec := range records {
		params := make([]string, len(rec.fieldNames))
		fields := make([]string, len(rec.fieldNames))
		for i, name := range rec.fieldNames {
			params[i] = name
			fields[i] = name
		}
		fmt.Fprintf(&b, "// record %s\n", rec.name)
		fmt.Fprintf(&b, "export function %s(%s) {\n    return { %s };\n}\n\n",
			rec.name, strings.Join(params, ", "), strings.Join(fields, ", "))
	}

	return b.String()
}

type jsAdtTypeInfo struct {
	name  string
	ctors []jsCtorTypeInfo
}

type jsCtorTypeInfo struct {
	name     string
	typeName string
	arity    int
}

type jsRecordTypeInfo struct {
	name       string
	fieldNames []string
}

// libGoType maps a Rex type to a Go type string for library type stubs.
// Same logic as goGen.goType but standalone (no receiver needed).
func libGoType(ty types.Type) string {
	if ty == nil {
		return "any"
	}
	tc, ok := ty.(types.TCon)
	if !ok {
		return "any"
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
