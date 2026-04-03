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
