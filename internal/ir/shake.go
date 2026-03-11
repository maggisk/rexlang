// Tree shaking: remove unreachable declarations from a program.
package ir

// Shake removes declarations not transitively reachable from "main".
// Type declarations (DType), trait declarations (DTrait), impl declarations
// (DImpl), and import metadata (DImport) are always kept.
func Shake(prog *Program) *Program {
	// Collect all top-level function bodies for reference scanning
	type funcBody struct {
		expr  Expr  // for DLet
		cexpr CExpr // for DLetRec bindings
	}
	funcBodies := make(map[string]funcBody)
	for _, d := range prog.Decls {
		switch dl := d.(type) {
		case DLet:
			funcBodies[dl.Name] = funcBody{expr: dl.Body}
		case DLetRec:
			for _, b := range dl.Bindings {
				funcBodies[b.Name] = funcBody{cexpr: b.Bind}
			}
		}
	}

	// BFS from "main" to find all reachable functions
	reachable := make(map[string]bool)
	queue := []string{"main"}
	reachable["main"] = true

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		fb, ok := funcBodies[name]
		if !ok {
			continue
		}
		var refs []string
		if fb.expr != nil {
			refs = collectRefs(fb.expr)
		} else if fb.cexpr != nil {
			collectCExprRefs(fb.cexpr, &refs)
		}
		for _, ref := range refs {
			if _, ok := funcBodies[ref]; ok && !reachable[ref] {
				reachable[ref] = true
				queue = append(queue, ref)
			}
		}
	}

	// Filter: keep reachable functions + all types/traits/impls/imports
	var kept []Decl
	for _, d := range prog.Decls {
		switch dl := d.(type) {
		case DLet:
			if dl.Name == "_" || reachable[dl.Name] {
				kept = append(kept, d)
			}
		case DLetRec:
			// Keep if any binding is reachable
			for _, b := range dl.Bindings {
				if reachable[b.Name] {
					kept = append(kept, d)
					break
				}
			}
		default:
			// DType, DTrait, DImpl, DImport, DTest — always keep
			kept = append(kept, d)
		}
	}

	return &Program{Decls: kept}
}

// collectRefs returns all variable names referenced in an expression.
func collectRefs(expr Expr) []string {
	var refs []string
	collectExprRefs(expr, &refs)
	return refs
}

func collectExprRefs(expr Expr, refs *[]string) {
	switch e := expr.(type) {
	case EAtom:
		collectAtomRefs(e.A, refs)
	case EComplex:
		collectCExprRefs(e.C, refs)
	case ELet:
		collectCExprRefs(e.Bind, refs)
		collectExprRefs(e.Body, refs)
	case ELetRec:
		for _, b := range e.Bindings {
			collectCExprRefs(b.Bind, refs)
		}
		collectExprRefs(e.Body, refs)
	}
}

func collectCExprRefs(c CExpr, refs *[]string) {
	switch e := c.(type) {
	case CApp:
		collectAtomRefs(e.Func, refs)
		collectAtomRefs(e.Arg, refs)
	case CBinop:
		collectAtomRefs(e.Left, refs)
		collectAtomRefs(e.Right, refs)
	case CUnaryMinus:
		collectAtomRefs(e.Expr, refs)
	case CIf:
		collectAtomRefs(e.Cond, refs)
		collectExprRefs(e.Then, refs)
		collectExprRefs(e.Else, refs)
	case CMatch:
		collectAtomRefs(e.Scrutinee, refs)
		for _, arm := range e.Arms {
			collectExprRefs(arm.Body, refs)
		}
	case CLambda:
		collectExprRefs(e.Body, refs)
	case CCtor:
		for _, a := range e.Args {
			collectAtomRefs(a, refs)
		}
	case CRecord:
		for _, f := range e.Fields {
			collectAtomRefs(f.Value, refs)
		}
	case CFieldAccess:
		collectAtomRefs(e.Record, refs)
	case CRecordUpdate:
		collectAtomRefs(e.Record, refs)
		for _, u := range e.Updates {
			collectAtomRefs(u.Value, refs)
		}
	case CList:
		for _, a := range e.Items {
			collectAtomRefs(a, refs)
		}
	case CTuple:
		for _, a := range e.Items {
			collectAtomRefs(a, refs)
		}
	case CStringInterp:
		for _, a := range e.Parts {
			collectAtomRefs(a, refs)
		}
	}
}

func collectAtomRefs(a Atom, refs *[]string) {
	if v, ok := a.(AVar); ok {
		*refs = append(*refs, v.Name)
	}
}
