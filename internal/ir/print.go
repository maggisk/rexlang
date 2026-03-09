package ir

import (
	"fmt"
	"strings"
)

// ExprToString returns a human-readable string representation of an IR expression.
func ExprToString(expr Expr) string {
	return exprToString(expr, 0)
}

func indent(level int) string {
	return strings.Repeat("  ", level)
}

func exprToString(expr Expr, level int) string {
	switch e := expr.(type) {
	case EAtom:
		return atomToString(e.A)
	case EComplex:
		return cexprToString(e.C, level)
	case ELet:
		bind := cexprToString(e.Bind, level+1)
		body := exprToString(e.Body, level)
		return fmt.Sprintf("let %s = %s\nin %s", e.Name, bind, body)
	case ELetRec:
		var parts []string
		for _, b := range e.Bindings {
			parts = append(parts, fmt.Sprintf("%s = %s", b.Name, cexprToString(b.Bind, level+1)))
		}
		body := exprToString(e.Body, level)
		return fmt.Sprintf("let rec %s\nin %s", strings.Join(parts, "\n    and "), body)
	default:
		return fmt.Sprintf("<?%T>", expr)
	}
}

func atomToString(a Atom) string {
	switch v := a.(type) {
	case AVar:
		return v.Name
	case AInt:
		return fmt.Sprintf("%d", v.Value)
	case AFloat:
		return fmt.Sprintf("%g", v.Value)
	case AString:
		return fmt.Sprintf("%q", v.Value)
	case ABool:
		if v.Value {
			return "true"
		}
		return "false"
	case AUnit:
		return "()"
	default:
		return fmt.Sprintf("<?%T>", a)
	}
}

func cexprToString(c CExpr, level int) string {
	switch e := c.(type) {
	case CApp:
		return fmt.Sprintf("%s %s", atomToString(e.Func), atomToString(e.Arg))
	case CBinop:
		sym := e.Op
		return fmt.Sprintf("%s %s %s", atomToString(e.Left), sym, atomToString(e.Right))
	case CUnaryMinus:
		return fmt.Sprintf("-%s", atomToString(e.Expr))
	case CIf:
		return fmt.Sprintf("if %s then\n%s  %s\n%selse\n%s  %s",
			atomToString(e.Cond),
			indent(level+1), exprToString(e.Then, level+1),
			indent(level), indent(level+1), exprToString(e.Else, level+1))
	case CMatch:
		var arms []string
		for _, arm := range e.Arms {
			arms = append(arms, fmt.Sprintf("%swhen %s ->\n%s  %s",
				indent(level+1), patternToString(arm.Pat),
				indent(level+1), exprToString(arm.Body, level+2)))
		}
		return fmt.Sprintf("match %s\n%s", atomToString(e.Scrutinee), strings.Join(arms, "\n"))
	case CLambda:
		return fmt.Sprintf("\\%s -> %s", e.Param, exprToString(e.Body, level))
	case CCtor:
		if len(e.Args) == 0 {
			return e.Name
		}
		args := make([]string, len(e.Args))
		for i, a := range e.Args {
			args[i] = atomToString(a)
		}
		return fmt.Sprintf("%s %s", e.Name, strings.Join(args, " "))
	case CRecord:
		var fields []string
		for _, f := range e.Fields {
			fields = append(fields, fmt.Sprintf("%s = %s", f.Name, atomToString(f.Value)))
		}
		return fmt.Sprintf("%s { %s }", e.TypeName, strings.Join(fields, ", "))
	case CFieldAccess:
		return fmt.Sprintf("%s.%s", atomToString(e.Record), e.Field)
	case CRecordUpdate:
		var updates []string
		for _, u := range e.Updates {
			updates = append(updates, fmt.Sprintf("%s = %s", strings.Join(u.Path, "."), atomToString(u.Value)))
		}
		return fmt.Sprintf("{ %s | %s }", atomToString(e.Record), strings.Join(updates, ", "))
	case CList:
		items := make([]string, len(e.Items))
		for i, a := range e.Items {
			items[i] = atomToString(a)
		}
		return fmt.Sprintf("[%s]", strings.Join(items, ", "))
	case CTuple:
		items := make([]string, len(e.Items))
		for i, a := range e.Items {
			items[i] = atomToString(a)
		}
		return fmt.Sprintf("(%s)", strings.Join(items, ", "))
	case CStringInterp:
		parts := make([]string, len(e.Parts))
		for i, a := range e.Parts {
			parts[i] = atomToString(a)
		}
		return fmt.Sprintf("interp(%s)", strings.Join(parts, " ++ "))
	default:
		return fmt.Sprintf("<?%T>", c)
	}
}

func patternToString(p Pattern) string {
	switch v := p.(type) {
	case PWild:
		return "_"
	case PVar:
		return v.Name
	case PInt:
		return fmt.Sprintf("%d", v.Value)
	case PFloat:
		return fmt.Sprintf("%g", v.Value)
	case PString:
		return fmt.Sprintf("%q", v.Value)
	case PBool:
		if v.Value {
			return "true"
		}
		return "false"
	case PUnit:
		return "()"
	case PNil:
		return "[]"
	case PCons:
		return fmt.Sprintf("[%s | %s]", patternToString(v.Head), patternToString(v.Tail))
	case PTuple:
		pats := make([]string, len(v.Pats))
		for i, sub := range v.Pats {
			pats[i] = patternToString(sub)
		}
		return fmt.Sprintf("(%s)", strings.Join(pats, ", "))
	case PCtor:
		if len(v.Args) == 0 {
			return v.Name
		}
		args := make([]string, len(v.Args))
		for i, sub := range v.Args {
			args[i] = patternToString(sub)
		}
		return fmt.Sprintf("%s %s", v.Name, strings.Join(args, " "))
	case PRecord:
		fields := make([]string, len(v.Fields))
		for i, f := range v.Fields {
			fields[i] = fmt.Sprintf("%s = %s", f.Name, patternToString(f.Pat))
		}
		return fmt.Sprintf("%s { %s }", v.TypeName, strings.Join(fields, ", "))
	default:
		return fmt.Sprintf("<?%T>", p)
	}
}

// DeclToString returns a human-readable string of an IR declaration.
func DeclToString(d Decl) string {
	switch decl := d.(type) {
	case DLet:
		exp := ""
		if decl.Exported {
			exp = "export "
		}
		return fmt.Sprintf("%s%s = %s", exp, decl.Name, exprToString(decl.Body, 0))
	case DLetRec:
		var parts []string
		for _, b := range decl.Bindings {
			parts = append(parts, fmt.Sprintf("%s = %s", b.Name, cexprToString(b.Bind, 1)))
		}
		return fmt.Sprintf("let rec %s", strings.Join(parts, "\n    and "))
	case DType:
		return fmt.Sprintf("type %s", decl.Name)
	case DTrait:
		return fmt.Sprintf("trait %s", decl.Name)
	case DImpl:
		return fmt.Sprintf("impl %s", decl.TraitName)
	case DImport:
		return fmt.Sprintf("import %s", decl.Module)
	case DTest:
		return fmt.Sprintf("test %q = %s", decl.Name, exprToString(decl.Body, 1))
	default:
		return fmt.Sprintf("<?%T>", d)
	}
}

// ProgramToString returns a human-readable dump of the full IR program.
func ProgramToString(p *Program) string {
	var parts []string
	for _, d := range p.Decls {
		if d == nil {
			continue
		}
		parts = append(parts, DeclToString(d))
	}
	return strings.Join(parts, "\n\n")
}
