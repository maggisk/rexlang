// Package typechecker implements Algorithm W (Hindley-Milner) for RexLang.
package typechecker

import (
	"fmt"
	"strings"
	"sync"

	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/stdlib"
	"github.com/maggisk/rexlang/internal/types"
)

// ---------------------------------------------------------------------------
// TypeEnv — maps names to Scheme or metadata objects
// ---------------------------------------------------------------------------

// TypeEnv is a map from name to Scheme (or metadata).
type TypeEnv map[string]interface{}

// Clone returns a shallow copy of the TypeEnv.
func (e TypeEnv) Clone() TypeEnv {
	return e.clone()
}

func (e TypeEnv) clone() TypeEnv {
	out := make(TypeEnv, len(e))
	for k, v := range e {
		out[k] = v
	}
	return out
}

func (e TypeEnv) extend(name string, val interface{}) TypeEnv {
	out := e.clone()
	out[name] = val
	return out
}

func (e TypeEnv) extendMany(m map[string]interface{}) TypeEnv {
	out := e.clone()
	for k, v := range m {
		out[k] = v
	}
	return out
}

func scheme(v interface{}) (types.Scheme, bool) {
	s, ok := v.(types.Scheme)
	return s, ok
}

// ---------------------------------------------------------------------------
// TypeChecker
// ---------------------------------------------------------------------------

// TypeChecker holds mutable state for HM inference.
type TypeChecker struct {
	counter     int
	typeAliases map[string]types.TypeAliasInfo
}

// NewTypeChecker creates a new TypeChecker.
func NewTypeChecker() *TypeChecker {
	return &TypeChecker{}
}

func (tc *TypeChecker) fresh() types.Type {
	tc.counter++
	return types.TVar{Name: fmt.Sprintf("t%d", tc.counter)}
}

func (tc *TypeChecker) instantiate(s types.Scheme) types.Type {
	subst := make(types.Subst, len(s.Vars))
	for _, v := range s.Vars {
		subst[v] = tc.fresh()
	}
	return types.SubstOnce(subst, s.Ty)
}

// ---------------------------------------------------------------------------
// Pattern inference
// ---------------------------------------------------------------------------

// inferPattern infers the type of a pattern.
// Returns (subst, patType, bindings).
func (tc *TypeChecker) inferPattern(pat ast.Pattern, env TypeEnv, typeDefs map[string]types.Type, subst types.Subst) (types.Subst, types.Type, map[string]types.Scheme, error) {
	switch p := pat.(type) {
	case ast.PWild:
		return subst, tc.fresh(), map[string]types.Scheme{}, nil

	case ast.PUnit:
		return subst, types.TUnit, map[string]types.Scheme{}, nil

	case ast.PVar:
		tv := tc.fresh()
		return subst, tv, map[string]types.Scheme{p.Name: {Ty: tv}}, nil

	case ast.PInt:
		return subst, types.TInt, map[string]types.Scheme{}, nil

	case ast.PFloat:
		return subst, types.TFloat, map[string]types.Scheme{}, nil

	case ast.PString:
		return subst, types.TString, map[string]types.Scheme{}, nil

	case ast.PBool:
		return subst, types.TBool, map[string]types.Scheme{}, nil

	case ast.PNil:
		tv := tc.fresh()
		return subst, types.TList(tv), map[string]types.Scheme{}, nil

	case ast.PCons:
		tv := tc.fresh()
		s1, th, hbinds, err := tc.inferPattern(p.Head, env, typeDefs, subst)
		if err != nil {
			return nil, nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, th), types.ApplySubst(s1, tv))
		if err != nil {
			return nil, nil, nil, &types.TypeError{Msg: "in cons pattern head: " + err.Error()}
		}
		s12 := types.ComposeSubst(s2, s1)
		s3, tt, tbinds, err := tc.inferPattern(p.Tail, env, typeDefs, s12)
		if err != nil {
			return nil, nil, nil, err
		}
		listTv := types.TList(types.ApplySubst(types.ComposeSubst(s3, s12), tv))
		s4, err := types.Unify(types.ApplySubst(s3, tt), listTv)
		if err != nil {
			return nil, nil, nil, &types.TypeError{Msg: "in cons pattern tail: " + err.Error()}
		}
		sFinal := types.ComposeSubst(s4, types.ComposeSubst(s3, s12))
		merged := mergeBind(hbinds, tbinds)
		return sFinal, types.TList(types.ApplySubst(sFinal, tv)), merged, nil

	case ast.PTuple:
		s := subst
		itemTypes := []types.Type{}
		allBinds := map[string]types.Scheme{}
		for _, pp := range p.Pats {
			s1, pt, binds, err := tc.inferPattern(pp, env, typeDefs, s)
			if err != nil {
				return nil, nil, nil, err
			}
			s = types.ComposeSubst(s1, s)
			itemTypes = append(itemTypes, pt)
			for k, v := range binds {
				allBinds[k] = v
			}
		}
		finalTypes := make([]types.Type, len(itemTypes))
		for i, t := range itemTypes {
			finalTypes[i] = types.ApplySubst(s, t)
		}
		return s, types.TTuple(finalTypes), allBinds, nil

	case ast.PRecord:
		recordFields, _ := env["__record_fields__"]
		rfMap, ok := recordFields.(map[string]types.RecordInfo)
		if !ok {
			return nil, nil, nil, &types.TypeError{Msg: "unknown record type: " + p.TypeName}
		}
		ri, ok := rfMap[p.TypeName]
		if !ok {
			return nil, nil, nil, &types.TypeError{Msg: "unknown record type: " + p.TypeName}
		}
		// Instantiate type parameters
		paramSubst := make(types.Subst, len(ri.Params))
		for _, param := range ri.Params {
			paramSubst[param] = tc.fresh()
		}
		expectedFields := map[string]types.Type{}
		for _, fi := range ri.Fields {
			expectedFields[fi.Name] = types.ApplySubst(paramSubst, fi.Type)
		}
		// Check field names are valid
		for _, f := range p.Fields {
			if _, ok := expectedFields[f.Name]; !ok {
				return nil, nil, nil, &types.TypeError{Msg: fmt.Sprintf("record '%s' has no field '%s'", p.TypeName, f.Name)}
			}
		}
		s := subst
		allBinds := map[string]types.Scheme{}
		for _, f := range p.Fields {
			s1, patTy, binds, err := tc.inferPattern(f.Pat, env, typeDefs, s)
			if err != nil {
				return nil, nil, nil, err
			}
			s = types.ComposeSubst(s1, s)
			expected := types.ApplySubst(s, expectedFields[f.Name])
			s2, err := types.Unify(types.ApplySubst(s, patTy), expected)
			if err != nil {
				return nil, nil, nil, &types.TypeError{Msg: fmt.Sprintf("in record pattern %s, field '%s': %s", p.TypeName, f.Name, err.Error())}
			}
			s = types.ComposeSubst(s2, s)
			for k, v := range binds {
				allBinds[k] = v
			}
		}
		resultArgs := make([]types.Type, len(ri.Params))
		for i, param := range ri.Params {
			resultArgs[i] = types.ApplySubst(s, paramSubst[param])
		}
		return s, types.TCon{Name: p.TypeName, Args: resultArgs}, allBinds, nil

	case ast.PCtor:
		envVal, ok := env[p.Name]
		if !ok {
			return nil, nil, nil, &types.TypeError{Msg: "unknown constructor: " + p.Name}
		}
		sc, ok := envVal.(types.Scheme)
		if !ok {
			return nil, nil, nil, &types.TypeError{Msg: "not a constructor: " + p.Name}
		}
		ctorTy := tc.instantiate(sc)
		argTys, resultTy, err := tc.decomposeFun(ctorTy, len(p.Args))
		if err != nil {
			return nil, nil, nil, &types.TypeError{Msg: fmt.Sprintf("constructor %s applied to wrong number of arguments", p.Name)}
		}
		s := subst
		allBinds := map[string]types.Scheme{}
		for i, argPat := range p.Args {
			s1, patTy, binds, err := tc.inferPattern(argPat, env, typeDefs, s)
			if err != nil {
				return nil, nil, nil, err
			}
			s2, err := types.Unify(types.ApplySubst(s1, patTy), types.ApplySubst(s1, argTys[i]))
			if err != nil {
				return nil, nil, nil, &types.TypeError{Msg: fmt.Sprintf("in constructor pattern %s: %s", p.Name, err.Error())}
			}
			s = types.ComposeSubst(s2, s1)
			for k, v := range binds {
				allBinds[k] = v
			}
		}
		return s, types.ApplySubst(s, resultTy), allBinds, nil
	}
	return nil, nil, nil, &types.TypeError{Msg: fmt.Sprintf("unknown pattern type: %T", pat)}
}

func mergeBind(a, b map[string]types.Scheme) map[string]types.Scheme {
	out := make(map[string]types.Scheme, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Expression inference
// ---------------------------------------------------------------------------

func (tc *TypeChecker) infer(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, expr ast.Expr) (types.Subst, types.Type, error) {
	switch e := expr.(type) {
	case ast.IntLit:
		return subst, types.TInt, nil
	case ast.FloatLit:
		return subst, types.TFloat, nil
	case ast.StringLit:
		return subst, types.TString, nil
	case ast.StringInterp:
		s := subst
		for _, part := range e.Parts {
			var err error
			s, _, err = tc.infer(env, typeDefs, s, part)
			if err != nil {
				return nil, nil, err
			}
		}
		return s, types.TString, nil
	case ast.BoolLit:
		return subst, types.TBool, nil
	case ast.UnitLit:
		return subst, types.TUnit, nil

	case ast.Var:
		v, ok := env[e.Name]
		if !ok {
			return nil, nil, &types.TypeError{Msg: "unbound variable: " + e.Name}
		}
		sc, ok := v.(types.Scheme)
		if !ok {
			return nil, nil, &types.TypeError{Msg: "not a value: " + e.Name}
		}
		return subst, tc.instantiate(sc), nil

	case ast.UnaryMinus:
		s, t, err := tc.infer(env, typeDefs, subst, e.Expr)
		if err != nil {
			return nil, nil, err
		}
		return s, t, nil

	case ast.Binop:
		return tc.inferBinop(env, typeDefs, subst, e)

	case ast.If:
		s1, tc1, err := tc.infer(env, typeDefs, subst, e.Cond)
		if err != nil {
			return nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, tc1), types.TBool)
		if err != nil {
			return nil, nil, &types.TypeError{Msg: "if condition must be Bool, got " + types.TypeToString(types.ApplySubst(s1, tc1))}
		}
		s12 := types.ComposeSubst(s2, s1)
		env12 := applySubstEnv(s12, env)
		s3, tt, err := tc.infer(env12, typeDefs, s12, e.ThenExpr)
		if err != nil {
			return nil, nil, err
		}
		s123 := types.ComposeSubst(s3, s12)
		env123 := applySubstEnv(s123, env)
		s4, te, err := tc.infer(env123, typeDefs, s123, e.ElseExpr)
		if err != nil {
			return nil, nil, err
		}
		s1234 := types.ComposeSubst(s4, s123)
		s5, err := types.Unify(types.ApplySubst(s4, tt), types.ApplySubst(s4, te))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("if branches have different types: %s vs %s",
				types.TypeToString(types.ApplySubst(s4, tt)), types.TypeToString(types.ApplySubst(s4, te)))}
		}
		sFinal := types.ComposeSubst(s5, s1234)
		return sFinal, types.ApplySubst(sFinal, tt), nil

	case ast.Fun:
		tv := tc.fresh()
		env1 := env.extend(e.Param, types.Scheme{Ty: tv})
		s1, tBody, err := tc.infer(env1, typeDefs, subst, e.Body)
		if err != nil {
			return nil, nil, err
		}
		return s1, types.TFun(types.ApplySubst(s1, tv), tBody), nil

	case ast.App:
		s1, tf, err := tc.infer(env, typeDefs, subst, e.Func)
		if err != nil {
			return nil, nil, err
		}
		s2, ta, err := tc.infer(applySubstEnv(s1, env), typeDefs, types.ComposeSubst(s1, subst), e.Arg)
		if err != nil {
			return nil, nil, err
		}
		tr := tc.fresh()
		s3, err := types.Unify(types.ApplySubst(s2, tf), types.TFun(ta, tr))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("cannot apply %s to argument of type %s",
				types.TypeToString(types.ApplySubst(s2, tf)), types.TypeToString(ta))}
		}
		sFinal := types.ComposeSubst(s3, types.ComposeSubst(s2, s1))
		return sFinal, types.ApplySubst(sFinal, tr), nil

	case ast.Let:
		return tc.inferLet(env, typeDefs, subst, e)

	case ast.LetRec:
		return tc.inferLetrec(env, typeDefs, subst, e)

	case ast.LetPat:
		s1, tBody, err := tc.infer(env, typeDefs, subst, e.Body)
		if err != nil {
			return nil, nil, err
		}
		env1 := applySubstEnv(s1, env)
		s2, patTy, bindings, err := tc.inferPattern(e.Pat, env1, typeDefs, s1)
		if err != nil {
			return nil, nil, err
		}
		s12 := types.ComposeSubst(s2, s1)
		s3, err := types.Unify(types.ApplySubst(s12, tBody), types.ApplySubst(s12, patTy))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: "in let pattern: " + err.Error()}
		}
		sFinal := types.ComposeSubst(s3, s12)
		if e.InExpr != nil {
			appliedBindings := make(TypeEnv)
			for k, v := range bindings {
				appliedBindings[k] = types.ApplySubstScheme(sFinal, v)
			}
			env2 := applySubstEnv(sFinal, env).extendMany(appliedBindings)
			s4, tIn, err := tc.infer(env2, typeDefs, sFinal, e.InExpr)
			if err != nil {
				return nil, nil, err
			}
			return types.ComposeSubst(s4, sFinal), tIn, nil
		}
		return sFinal, types.ApplySubst(sFinal, tBody), nil

	case ast.Match:
		return tc.inferMatch(env, typeDefs, subst, e)

	case ast.ListLit:
		tv := tc.fresh()
		s := subst
		for _, item := range e.Items {
			s1, ti, err := tc.infer(applySubstEnv(s, env), typeDefs, s, item)
			if err != nil {
				return nil, nil, err
			}
			s2, err := types.Unify(types.ApplySubst(s1, ti), types.ApplySubst(s1, tv))
			if err != nil {
				return nil, nil, &types.TypeError{Msg: fmt.Sprintf("list elements must all have the same type: expected %s, got %s",
					types.TypeToString(types.ApplySubst(s1, tv)), types.TypeToString(types.ApplySubst(s1, ti)))}
			}
			s = types.ComposeSubst(s2, s1)
			tv = types.ApplySubst(s, tv)
		}
		return s, types.TList(types.ApplySubst(s, tv)), nil

	case ast.TupleLit:
		s := subst
		itemTypes := []types.Type{}
		for _, item := range e.Items {
			s1, ti, err := tc.infer(applySubstEnv(s, env), typeDefs, s, item)
			if err != nil {
				return nil, nil, err
			}
			s = types.ComposeSubst(s1, s)
			itemTypes = append(itemTypes, ti)
		}
		finalTypes := make([]types.Type, len(itemTypes))
		for i, t := range itemTypes {
			finalTypes[i] = types.ApplySubst(s, t)
		}
		return s, types.TTuple(finalTypes), nil

	case ast.DotAccess:
		modules, _ := env["__modules__"].(map[string]TypeEnv)
		if modules == nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("'%s' is not a qualified module", e.ModuleName)}
		}
		modEnv, ok := modules[e.ModuleName]
		if !ok {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("'%s' is not a qualified module", e.ModuleName)}
		}
		v, ok := modEnv[e.FieldName]
		if !ok {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("module '%s' does not export '%s'", e.ModuleName, e.FieldName)}
		}
		sc, ok := v.(types.Scheme)
		if !ok {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("'%s.%s' is not a value", e.ModuleName, e.FieldName)}
		}
		return subst, tc.instantiate(sc), nil

	case ast.RecordCreate:
		recordFields, ok := env["__record_fields__"]
		if !ok {
			return nil, nil, &types.TypeError{Msg: "unknown record type: " + e.TypeName}
		}
		rfMap, ok := recordFields.(map[string]types.RecordInfo)
		if !ok {
			return nil, nil, &types.TypeError{Msg: "unknown record type: " + e.TypeName}
		}
		ri, ok := rfMap[e.TypeName]
		if !ok {
			return nil, nil, &types.TypeError{Msg: "unknown record type: " + e.TypeName}
		}
		// Instantiate type parameters
		paramSubst := make(types.Subst, len(ri.Params))
		for _, p := range ri.Params {
			paramSubst[p] = tc.fresh()
		}
		// Check all fields present, no extra
		provided := map[string]bool{}
		for _, f := range e.Fields {
			provided[f.Name] = true
		}
		for _, fi := range ri.Fields {
			if !provided[fi.Name] {
				return nil, nil, &types.TypeError{Msg: fmt.Sprintf("missing field '%s' in %s", fi.Name, e.TypeName)}
			}
		}
		expectedFields := map[string]types.Type{}
		for _, fi := range ri.Fields {
			expectedFields[fi.Name] = types.ApplySubst(paramSubst, fi.Type)
		}
		for _, f := range e.Fields {
			if _, ok := expectedFields[f.Name]; !ok {
				return nil, nil, &types.TypeError{Msg: fmt.Sprintf("unknown field '%s' in %s", f.Name, e.TypeName)}
			}
		}
		s := subst
		for _, f := range e.Fields {
			s1, ft, err := tc.infer(applySubstEnv(s, env), typeDefs, s, f.Value)
			if err != nil {
				return nil, nil, err
			}
			s = types.ComposeSubst(s1, s)
			expected := types.ApplySubst(s, expectedFields[f.Name])
			s2, err := types.Unify(types.ApplySubst(s, ft), expected)
			if err != nil {
				return nil, nil, &types.TypeError{Msg: fmt.Sprintf("field '%s' in %s: %s", f.Name, e.TypeName, err.Error())}
			}
			s = types.ComposeSubst(s2, s)
		}
		// Build result type
		resultArgs := make([]types.Type, len(ri.Params))
		for i, p := range ri.Params {
			resultArgs[i] = types.ApplySubst(s, paramSubst[p])
		}
		return s, types.TCon{Name: e.TypeName, Args: resultArgs}, nil

	case ast.FieldAccess:
		s1, recTy, err := tc.infer(env, typeDefs, subst, e.Record)
		if err != nil {
			return nil, nil, err
		}
		resolved := types.ApplySubst(s1, recTy)
		recordFields, _ := env["__record_fields__"]
		rfMap, _ := recordFields.(map[string]types.RecordInfo)
		if rfMap == nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("cannot access field '%s': no record types defined", e.Field)}
		}
		// If the type is a TVar, find which record type has this field
		if _, isTVar := resolved.(types.TVar); isTVar {
			var matches []string
			for typeName, ri := range rfMap {
				for _, fi := range ri.Fields {
					if fi.Name == e.Field {
						matches = append(matches, typeName)
						break
					}
				}
			}
			if len(matches) == 0 {
				return nil, nil, &types.TypeError{Msg: fmt.Sprintf("no record type has field '%s'", e.Field)}
			}
			if len(matches) > 1 {
				return nil, nil, &types.TypeError{Msg: fmt.Sprintf("ambiguous field '%s': multiple record types have this field", e.Field)}
			}
			ri := rfMap[matches[0]]
			paramSubst := make(types.Subst, len(ri.Params))
			for _, p := range ri.Params {
				paramSubst[p] = tc.fresh()
			}
			paramArgs := make([]types.Type, len(ri.Params))
			for i, p := range ri.Params {
				paramArgs[i] = paramSubst[p]
			}
			expectedRecTy := types.TCon{Name: matches[0], Args: paramArgs}
			s2, err := types.Unify(resolved, expectedRecTy)
			if err != nil {
				return nil, nil, err
			}
			s := types.ComposeSubst(s2, s1)
			for _, fi := range ri.Fields {
				if fi.Name == e.Field {
					return s, types.ApplySubst(s, types.ApplySubst(paramSubst, fi.Type)), nil
				}
			}
		}
		con, ok := resolved.(types.TCon)
		if !ok {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("cannot access field '%s' on non-record type %s", e.Field, types.TypeToString(resolved))}
		}
		ri, ok := rfMap[con.Name]
		if !ok {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("'%s' is not a record type", con.Name)}
		}
		// Build substitution from type params to actual args
		paramSubst := make(types.Subst, len(ri.Params))
		for i, p := range ri.Params {
			if i < len(con.Args) {
				paramSubst[p] = con.Args[i]
			}
		}
		for _, fi := range ri.Fields {
			if fi.Name == e.Field {
				return s1, types.ApplySubst(paramSubst, fi.Type), nil
			}
		}
		return nil, nil, &types.TypeError{Msg: fmt.Sprintf("record '%s' has no field '%s'", con.Name, e.Field)}

	case ast.RecordUpdate:
		s1, recTy, err := tc.infer(env, typeDefs, subst, e.Record)
		if err != nil {
			return nil, nil, err
		}
		resolved := types.ApplySubst(s1, recTy)
		con, ok := resolved.(types.TCon)
		if !ok {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("record update requires a record type, got %s", types.TypeToString(resolved))}
		}
		recordFields, _ := env["__record_fields__"]
		rfMap, _ := recordFields.(map[string]types.RecordInfo)
		if rfMap == nil {
			return nil, nil, &types.TypeError{Msg: "no record types defined"}
		}
		ri, ok := rfMap[con.Name]
		if !ok {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("'%s' is not a record type", con.Name)}
		}
		paramSubst := make(types.Subst, len(ri.Params))
		for i, p := range ri.Params {
			if i < len(con.Args) {
				paramSubst[p] = con.Args[i]
			}
		}
		s := s1
		for _, upd := range e.Updates {
			leafTy, err := tc.resolveFieldPath(rfMap, con.Name, upd.Path, paramSubst)
			if err != nil {
				return nil, nil, err
			}
			s2, valTy, err := tc.infer(applySubstEnv(s, env), typeDefs, s, upd.Value)
			if err != nil {
				return nil, nil, err
			}
			s = types.ComposeSubst(s2, s)
			s3, err := types.Unify(types.ApplySubst(s, valTy), types.ApplySubst(s, leafTy))
			if err != nil {
				return nil, nil, &types.TypeError{Msg: fmt.Sprintf("record update field '%s': %s", dotPathString(upd.Path), err.Error())}
			}
			s = types.ComposeSubst(s3, s)
		}
		resultArgs := make([]types.Type, len(con.Args))
		for i, a := range con.Args {
			resultArgs[i] = types.ApplySubst(s, a)
		}
		return s, types.TCon{Name: con.Name, Args: resultArgs}, nil

	case ast.Assert:
		s1, t, err := tc.infer(env, typeDefs, subst, e.Expr)
		if err != nil {
			return nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, t), types.TBool)
		if err != nil {
			return nil, nil, &types.TypeError{Msg: "assert requires Bool, got " + types.TypeToString(types.ApplySubst(s1, t))}
		}
		return types.ComposeSubst(s2, s1), types.TUnit, nil

	case ast.TypeDecl, ast.Import, ast.Export, ast.TraitDecl, ast.ImplDecl:
		return subst, types.TUnit, nil
	}
	return nil, nil, &types.TypeError{Msg: fmt.Sprintf("unknown AST node: %T", expr)}
}

// resolveFieldPath walks a dot-path through record types and returns the leaf field type.
func (tc *TypeChecker) resolveFieldPath(rfMap map[string]types.RecordInfo, recTypeName string, path []string, paramSubst types.Subst) (types.Type, error) {
	ri, ok := rfMap[recTypeName]
	if !ok {
		return nil, &types.TypeError{Msg: fmt.Sprintf("'%s' is not a record type", recTypeName)}
	}
	fieldName := path[0]
	var fieldType types.Type
	found := false
	for _, fi := range ri.Fields {
		if fi.Name == fieldName {
			fieldType = types.ApplySubst(paramSubst, fi.Type)
			found = true
			break
		}
	}
	if !found {
		return nil, &types.TypeError{Msg: fmt.Sprintf("record '%s' has no field '%s'", recTypeName, fieldName)}
	}
	if len(path) == 1 {
		return fieldType, nil
	}
	// Nested: fieldType must be a record TCon
	con, ok := fieldType.(types.TCon)
	if !ok {
		return nil, &types.TypeError{Msg: fmt.Sprintf("field '%s' in '%s' is not a record type, cannot access '%s'", fieldName, recTypeName, path[1])}
	}
	nestedRI, ok := rfMap[con.Name]
	if !ok {
		return nil, &types.TypeError{Msg: fmt.Sprintf("field '%s' type '%s' is not a record type", fieldName, con.Name)}
	}
	nestedParamSubst := make(types.Subst, len(nestedRI.Params))
	for i, p := range nestedRI.Params {
		if i < len(con.Args) {
			nestedParamSubst[p] = con.Args[i]
		}
	}
	return tc.resolveFieldPath(rfMap, con.Name, path[1:], nestedParamSubst)
}

func dotPathString(path []string) string {
	result := path[0]
	for _, p := range path[1:] {
		result += "." + p
	}
	return result
}

func (tc *TypeChecker) inferBinop(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, e ast.Binop) (types.Subst, types.Type, error) {
	op := e.Op
	switch op {
	case "Add", "Sub", "Mul", "Div", "Mod":
		s1, tl, err := tc.infer(env, typeDefs, subst, e.Left)
		if err != nil {
			return nil, nil, err
		}
		s2, tr, err := tc.infer(applySubstEnv(s1, env), typeDefs, types.ComposeSubst(s1, subst), e.Right)
		if err != nil {
			return nil, nil, err
		}
		s12 := types.ComposeSubst(s2, s1)
		s3, err := types.Unify(types.ApplySubst(s12, tl), types.ApplySubst(s12, tr))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("arithmetic type mismatch: %s vs %s",
				types.TypeToString(types.ApplySubst(s12, tl)), types.TypeToString(types.ApplySubst(s12, tr)))}
		}
		sFinal := types.ComposeSubst(s3, s12)
		resultTy := types.ApplySubst(sFinal, tl)
		if tv, ok := resultTy.(types.TVar); ok {
			sFinal = types.ComposeSubst(types.Subst{tv.Name: types.TInt}, sFinal)
			resultTy = types.TInt
		} else if !typeIsInt(resultTy) && !typeIsFloat(resultTy) {
			return nil, nil, &types.TypeError{Msg: "arithmetic requires Int or Float, got " + types.TypeToString(resultTy)}
		}
		if op == "Mod" && !typeIsInt(resultTy) {
			return nil, nil, &types.TypeError{Msg: "(%) requires Int operands, got " + types.TypeToString(resultTy)}
		}
		return sFinal, resultTy, nil

	case "Concat":
		s1, tl, err := tc.infer(env, typeDefs, subst, e.Left)
		if err != nil {
			return nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, tl), types.TString)
		if err != nil {
			return nil, nil, &types.TypeError{Msg: "(++) requires String, got " + types.TypeToString(types.ApplySubst(s1, tl))}
		}
		s12 := types.ComposeSubst(s2, s1)
		s3, tr, err := tc.infer(applySubstEnv(s12, env), typeDefs, s12, e.Right)
		if err != nil {
			return nil, nil, err
		}
		s123 := types.ComposeSubst(s3, s12)
		s4, err := types.Unify(types.ApplySubst(s123, tr), types.TString)
		if err != nil {
			return nil, nil, &types.TypeError{Msg: "(++) requires String, got " + types.TypeToString(types.ApplySubst(s123, tr))}
		}
		return types.ComposeSubst(s4, s123), types.TString, nil

	case "And", "Or":
		s1, tl, err := tc.infer(env, typeDefs, subst, e.Left)
		if err != nil {
			return nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, tl), types.TBool)
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("(%s) requires Bool, got %s", op, types.TypeToString(types.ApplySubst(s1, tl)))}
		}
		s12 := types.ComposeSubst(s2, s1)
		s3, tr, err := tc.infer(applySubstEnv(s12, env), typeDefs, s12, e.Right)
		if err != nil {
			return nil, nil, err
		}
		s123 := types.ComposeSubst(s3, s12)
		s4, err := types.Unify(types.ApplySubst(s123, tr), types.TBool)
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("(%s) requires Bool, got %s", op, types.TypeToString(types.ApplySubst(s123, tr)))}
		}
		return types.ComposeSubst(s4, s123), types.TBool, nil

	case "Lt", "Gt", "Leq", "Geq", "Eq", "Neq":
		s1, tl, err := tc.infer(env, typeDefs, subst, e.Left)
		if err != nil {
			return nil, nil, err
		}
		s2, tr, err := tc.infer(applySubstEnv(s1, env), typeDefs, types.ComposeSubst(s1, subst), e.Right)
		if err != nil {
			return nil, nil, err
		}
		s12 := types.ComposeSubst(s2, s1)
		s3, err := types.Unify(types.ApplySubst(s12, tl), types.ApplySubst(s12, tr))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("comparison type mismatch: %s vs %s",
				types.TypeToString(types.ApplySubst(s12, tl)), types.TypeToString(types.ApplySubst(s12, tr)))}
		}
		return types.ComposeSubst(s3, s12), types.TBool, nil

	case "Cons":
		s1, th, err := tc.infer(env, typeDefs, subst, e.Left)
		if err != nil {
			return nil, nil, err
		}
		s2, tt, err := tc.infer(applySubstEnv(s1, env), typeDefs, types.ComposeSubst(s1, subst), e.Right)
		if err != nil {
			return nil, nil, err
		}
		s12 := types.ComposeSubst(s2, s1)
		listTh := types.TList(types.ApplySubst(s12, th))
		s3, err := types.Unify(types.ApplySubst(s12, tt), listTh)
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("cons (::) type mismatch: tail must be [%s], got %s",
				types.TypeToString(types.ApplySubst(s12, th)), types.TypeToString(types.ApplySubst(s12, tt)))}
		}
		sFinal := types.ComposeSubst(s3, s12)
		return sFinal, types.ApplySubst(sFinal, listTh), nil
	}
	return nil, nil, &types.TypeError{Msg: "unknown operator: " + op}
}

func (tc *TypeChecker) inferLet(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, e ast.Let) (types.Subst, types.Type, error) {
	if e.Recursive {
		tv := tc.fresh()
		env1 := env.extend(e.Name, types.Scheme{Ty: tv})
		s1, t1, err := tc.infer(env1, typeDefs, subst, e.Body)
		if err != nil {
			return nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, tv), types.ApplySubst(s1, t1))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("in recursive let %s: %s", e.Name, err.Error())}
		}
		s12 := types.ComposeSubst(s2, s1)
		env2 := applySubstEnv(s12, env)
		gen := types.Generalize(env2, types.ApplySubst(s12, t1))
		env3 := env2.extend(e.Name, gen)
		if e.InExpr != nil {
			s3, t3, err := tc.infer(env3, typeDefs, s12, e.InExpr)
			if err != nil {
				return nil, nil, err
			}
			return types.ComposeSubst(s3, s12), t3, nil
		}
		return s12, types.ApplySubst(s12, gen.Ty), nil
	}
	s1, t1, err := tc.infer(env, typeDefs, subst, e.Body)
	if err != nil {
		return nil, nil, err
	}
	env1 := applySubstEnv(s1, env)
	gen := types.Generalize(env1, types.ApplySubst(s1, t1))
	env2 := env1.extend(e.Name, gen)
	if e.InExpr != nil {
		s2, t2, err := tc.infer(env2, typeDefs, types.ComposeSubst(s1, subst), e.InExpr)
		if err != nil {
			return nil, nil, err
		}
		return types.ComposeSubst(s2, s1), t2, nil
	}
	return s1, types.ApplySubst(s1, gen.Ty), nil
}

func (tc *TypeChecker) inferLetrecCore(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, bindings []ast.LetRecBinding) (types.Subst, map[string]types.Scheme, TypeEnv, error) {
	tvs := map[string]types.Type{}
	envExt := make(TypeEnv)
	for _, b := range bindings {
		tv := tc.fresh()
		tvs[b.Name] = tv
		envExt[b.Name] = types.Scheme{Ty: tv}
	}
	env1 := env.extendMany(envExt)
	s := subst
	for _, b := range bindings {
		s1, t1, err := tc.infer(env1, typeDefs, s, b.Body)
		if err != nil {
			return nil, nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, tvs[b.Name]), types.ApplySubst(s1, t1))
		if err != nil {
			return nil, nil, nil, &types.TypeError{Msg: fmt.Sprintf("in mutually recursive let %s: %s", b.Name, err.Error())}
		}
		s = types.ComposeSubst(s2, s1)
	}
	env2 := applySubstEnv(s, env)
	genEnv := map[string]types.Scheme{}
	for name, tv := range tvs {
		gen := types.Generalize(env2, types.ApplySubst(s, tv))
		genEnv[name] = gen
	}
	newEnvExt := make(TypeEnv)
	for k, v := range genEnv {
		newEnvExt[k] = v
	}
	newEnv := env2.extendMany(newEnvExt)
	return s, genEnv, newEnv, nil
}

func (tc *TypeChecker) inferLetrec(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, e ast.LetRec) (types.Subst, types.Type, error) {
	s, genEnv, env3, err := tc.inferLetrecCore(env, typeDefs, subst, e.Bindings)
	if err != nil {
		return nil, nil, err
	}
	if e.InExpr != nil {
		s2, t2, err := tc.infer(env3, typeDefs, s, e.InExpr)
		if err != nil {
			return nil, nil, err
		}
		return types.ComposeSubst(s2, s), t2, nil
	}
	lastName := e.Bindings[len(e.Bindings)-1].Name
	return s, types.ApplySubst(s, genEnv[lastName].Ty), nil
}

// checkExhaustive checks that a match is exhaustive.
func checkExhaustive(arms []ast.MatchArm, ctorFamilies map[string]map[string]bool) error {
	pats := make([]ast.Pattern, len(arms))
	for i, a := range arms {
		pats[i] = a.Pat
	}
	for _, p := range pats {
		if _, ok := p.(ast.PWild); ok {
			return nil
		}
		if _, ok := p.(ast.PVar); ok {
			return nil
		}
		if _, ok := p.(ast.PRecord); ok {
			return nil // record types have a single constructor, always exhaustive
		}
	}
	// Bool patterns
	hasBool := false
	for _, p := range pats {
		if _, ok := p.(ast.PBool); ok {
			hasBool = true
			break
		}
	}
	if hasBool {
		covered := map[bool]bool{}
		for _, p := range pats {
			if pb, ok := p.(ast.PBool); ok {
				covered[pb.Value] = true
			}
		}
		var missing []string
		if !covered[true] {
			missing = append(missing, "true")
		}
		if !covered[false] {
			missing = append(missing, "false")
		}
		if len(missing) > 0 {
			return &types.TypeError{Msg: fmt.Sprintf("non-exhaustive patterns: missing %v", missing)}
		}
		return nil
	}
	// List patterns
	hasNil, hasCons := false, false
	for _, p := range pats {
		if _, ok := p.(ast.PNil); ok {
			hasNil = true
		}
		if _, ok := p.(ast.PCons); ok {
			hasCons = true
		}
	}
	if hasNil || hasCons {
		var missing []string
		if !hasNil {
			missing = append(missing, "[]")
		}
		if !hasCons {
			missing = append(missing, "[h|t]")
		}
		if len(missing) > 0 {
			return &types.TypeError{Msg: fmt.Sprintf("non-exhaustive patterns: missing %v", missing)}
		}
		return nil
	}
	// Constructor patterns
	var ctorPats []ast.PCtor
	for _, p := range pats {
		if pc, ok := p.(ast.PCtor); ok {
			ctorPats = append(ctorPats, pc)
		}
	}
	if len(ctorPats) > 0 {
		firstName := ctorPats[0].Name
		if family, ok := ctorFamilies[firstName]; ok {
			covered := map[string]bool{}
			for _, pc := range ctorPats {
				covered[pc.Name] = true
			}
			var missing []string
			for name := range family {
				if !covered[name] {
					missing = append(missing, name)
				}
			}
			if len(missing) > 0 {
				return &types.TypeError{Msg: fmt.Sprintf("non-exhaustive patterns: missing %v", missing)}
			}
		}
	}
	return nil
}

func (tc *TypeChecker) inferMatch(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, e ast.Match) (types.Subst, types.Type, error) {
	s0, ts, err := tc.infer(env, typeDefs, subst, e.Scrutinee)
	if err != nil {
		return nil, nil, err
	}
	resultTv := tc.fresh()
	s := s0
	for _, arm := range e.Arms {
		envS := applySubstEnv(s, env)
		s1, patTy, bindings, err := tc.inferPattern(arm.Pat, envS, typeDefs, s)
		if err != nil {
			return nil, nil, err
		}
		s = types.ComposeSubst(s1, s)
		s2, err := types.Unify(types.ApplySubst(s, ts), types.ApplySubst(s, patTy))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("pattern type mismatch: scrutinee is %s, pattern expects %s",
				types.TypeToString(types.ApplySubst(s, ts)), types.TypeToString(types.ApplySubst(s, patTy)))}
		}
		s = types.ComposeSubst(s2, s)
		appliedBindings := make(TypeEnv)
		for k, v := range bindings {
			appliedBindings[k] = types.ApplySubstScheme(s, v)
		}
		bodyEnv := applySubstEnv(s, env).extendMany(appliedBindings)
		s3, bodyTy, err := tc.infer(bodyEnv, typeDefs, s, arm.Body)
		if err != nil {
			return nil, nil, err
		}
		s = types.ComposeSubst(s3, s)
		s4, err := types.Unify(types.ApplySubst(s, resultTv), types.ApplySubst(s, bodyTy))
		if err != nil {
			return nil, nil, &types.TypeError{Msg: fmt.Sprintf("match arms have different types: %s vs %s",
				types.TypeToString(types.ApplySubst(s, resultTv)), types.TypeToString(types.ApplySubst(s, bodyTy)))}
		}
		s = types.ComposeSubst(s4, s)
	}
	// Exhaustiveness check
	var ctorFamilies map[string]map[string]bool
	if cf, ok := env["__ctor_families__"]; ok {
		ctorFamilies, _ = cf.(map[string]map[string]bool)
	}
	if err := checkExhaustive(e.Arms, ctorFamilies); err != nil {
		return nil, nil, err
	}
	return s, types.ApplySubst(s, resultTv), nil
}

// ---------------------------------------------------------------------------
// Type comparison helpers
// ---------------------------------------------------------------------------

func typeIsInt(ty types.Type) bool {
	c, ok := ty.(types.TCon)
	return ok && c.Name == "Int" && len(c.Args) == 0
}

func typeIsFloat(ty types.Type) bool {
	c, ok := ty.(types.TCon)
	return ok && c.Name == "Float" && len(c.Args) == 0
}

// ---------------------------------------------------------------------------
// Helper utilities
// ---------------------------------------------------------------------------

func (tc *TypeChecker) decomposeFun(ty types.Type, n int) ([]types.Type, types.Type, error) {
	argTys := []types.Type{}
	for i := 0; i < n; i++ {
		con, ok := ty.(types.TCon)
		if !ok || con.Name != "Fun" {
			return nil, nil, fmt.Errorf("constructor applied to too many arguments")
		}
		argTys = append(argTys, con.Args[0])
		ty = con.Args[1]
	}
	return argTys, ty, nil
}

func (tc *TypeChecker) resolveTypeSig(node ast.TySyntax, typeDefs map[string]types.Type) (types.Type, error) {
	switch n := node.(type) {
	case ast.TyName:
		name := n.Name
		lowercasePrims := map[string]types.Type{
			"int": types.TInt, "float": types.TFloat,
			"string": types.TString, "bool": types.TBool,
		}
		if t, ok := lowercasePrims[name]; ok {
			return t, nil
		}
		if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' {
			return types.TVar{Name: name}, nil
		}
		primitives := map[string]types.Type{
			"Int": types.TInt, "Float": types.TFloat,
			"String": types.TString, "Bool": types.TBool, "Unit": types.TUnit,
		}
		if t, ok := primitives[name]; ok {
			return t, nil
		}
		if t, ok := typeDefs[name]; ok {
			return t, nil
		}
		// Check type aliases for non-parametric aliases not yet in typeDefs
		if info, ok := tc.typeAliases[name]; ok && len(info.Params) == 0 {
			return info.Body, nil
		}
		return types.TCon{Name: name, Args: nil}, nil
	case ast.TyFun:
		arg, err := tc.resolveTypeSig(n.Arg, typeDefs)
		if err != nil {
			return nil, err
		}
		ret, err := tc.resolveTypeSig(n.Ret, typeDefs)
		if err != nil {
			return nil, err
		}
		return types.TFun(arg, ret), nil
	case ast.TyList:
		elem, err := tc.resolveTypeSig(n.Elem, typeDefs)
		if err != nil {
			return nil, err
		}
		return types.TList(elem), nil
	case ast.TyTuple:
		elems := make([]types.Type, len(n.Elems))
		for i, e := range n.Elems {
			t, err := tc.resolveTypeSig(e, typeDefs)
			if err != nil {
				return nil, err
			}
			elems[i] = t
		}
		return types.TTuple(elems), nil
	case ast.TyUnit:
		return types.TUnit, nil
	case ast.TyApp:
		args := make([]types.Type, len(n.Args))
		for i, a := range n.Args {
			t, err := tc.resolveTypeSig(a, typeDefs)
			if err != nil {
				return nil, err
			}
			args[i] = t
		}
		// Check type aliases for parametric aliases
		if info, ok := tc.typeAliases[n.Name]; ok && len(info.Params) == len(args) {
			s := types.Subst{}
			for i, p := range info.Params {
				s[p] = args[i]
			}
			return types.SubstOnce(s, info.Body), nil
		}
		return types.TCon{Name: n.Name, Args: args}, nil
	case ast.TyRecord:
		// Record type syntax in annotations — not a standalone type, just pass through
		// This would need a type name lookup; for now we don't support anonymous record types
		return nil, &types.TypeError{Msg: "anonymous record types are not supported; use the named type instead"}
	}
	return nil, &types.TypeError{Msg: fmt.Sprintf("unknown type syntax node: %T", node)}
}

func (tc *TypeChecker) resolveType(name string, typeDefs map[string]types.Type, paramEnv map[string]types.Type) (types.Type, error) {
	if paramEnv != nil {
		if t, ok := paramEnv[name]; ok {
			return t, nil
		}
	}
	primitives := map[string]types.Type{
		"int": types.TInt, "float": types.TFloat,
		"string": types.TString, "bool": types.TBool,
	}
	if t, ok := primitives[name]; ok {
		return t, nil
	}
	if t, ok := typeDefs[name]; ok {
		return t, nil
	}
	return nil, &types.TypeError{Msg: "unknown type: " + name}
}

// ---------------------------------------------------------------------------
// Top-level inference
// ---------------------------------------------------------------------------

// InferToplevelResult holds the result of infer_toplevel.
type InferToplevelResult struct {
	Subst    types.Subst
	Ty       types.Type
	Env      TypeEnv
	TypeDefs map[string]types.Type
}

// InferToplevel infers at top level.
func (tc *TypeChecker) InferToplevel(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, expr ast.Expr) (InferToplevelResult, error) {
	switch e := expr.(type) {
	case ast.TypeDecl:
		// Type alias
		if e.AliasType != nil {
			body, err := tc.resolveTypeSig(e.AliasType, typeDefs)
			if err != nil {
				return InferToplevelResult{}, err
			}
			info := types.TypeAliasInfo{Params: e.Params, Body: body}
			if tc.typeAliases == nil {
				tc.typeAliases = map[string]types.TypeAliasInfo{}
			}
			tc.typeAliases[e.Name] = info
			newTypeDefs := make(map[string]types.Type, len(typeDefs)+1)
			for k, v := range typeDefs {
				newTypeDefs[k] = v
			}
			if len(e.Params) == 0 {
				newTypeDefs[e.Name] = body
			}
			return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: env, TypeDefs: newTypeDefs}, nil
		}

		paramVars := make([]types.Type, len(e.Params))
		for i, p := range e.Params {
			paramVars[i] = types.TVar{Name: p}
		}
		adtTy := types.TCon{Name: e.Name, Args: paramVars}
		newTypeDefs := make(map[string]types.Type, len(typeDefs)+1)
		for k, v := range typeDefs {
			newTypeDefs[k] = v
		}
		newTypeDefs[e.Name] = adtTy
		newEnv := env.clone()

		// Record type
		if len(e.RecordFields) > 0 {
			fieldInfos := make([]types.RecordFieldInfo, len(e.RecordFields))
			for i, f := range e.RecordFields {
				t, err := tc.resolveTypeSig(f.Type, newTypeDefs)
				if err != nil {
					return InferToplevelResult{}, err
				}
				fieldInfos[i] = types.RecordFieldInfo{Name: f.Name, Type: t}
			}
			ri := types.RecordInfo{Fields: fieldInfos, Params: e.Params}
			recordFields := map[string]types.RecordInfo{}
			if rf, ok := newEnv["__record_fields__"]; ok {
				if rfm, ok := rf.(map[string]types.RecordInfo); ok {
					for k, v := range rfm {
						recordFields[k] = v
					}
				}
			}
			recordFields[e.Name] = ri
			newEnv["__record_fields__"] = recordFields
			return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: newEnv, TypeDefs: newTypeDefs}, nil
		}

		// ADT type
		paramEnv := map[string]types.Type{}
		for _, p := range e.Params {
			paramEnv[p] = types.TVar{Name: p}
		}
		for _, ctor := range e.Ctors {
			argTypes := make([]types.Type, len(ctor.ArgTypes))
			for i, argAST := range ctor.ArgTypes {
				t, err := tc.resolveTypeSig(argAST, newTypeDefs)
				if err != nil {
					return InferToplevelResult{}, err
				}
				argTypes[i] = t
			}
			var ctorTy types.Type = adtTy
			for i := len(argTypes) - 1; i >= 0; i-- {
				ctorTy = types.TFun(argTypes[i], ctorTy)
			}
			newEnv[ctor.Name] = types.Scheme{Vars: e.Params, Ty: ctorTy}
		}
		// Build ctor_families
		ctorFamilies := map[string]map[string]bool{}
		if cf, ok := newEnv["__ctor_families__"]; ok {
			if cfm, ok := cf.(map[string]map[string]bool); ok {
				for k, v := range cfm {
					ctorFamilies[k] = v
				}
			}
		}
		allCtors := map[string]bool{}
		for _, ctor := range e.Ctors {
			allCtors[ctor.Name] = true
		}
		for _, ctor := range e.Ctors {
			ctorFamilies[ctor.Name] = allCtors
		}
		newEnv["__ctor_families__"] = ctorFamilies
		return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: newEnv, TypeDefs: newTypeDefs}, nil

	case ast.TypeAnnotation:
		resolved, err := tc.resolveTypeSig(e.Type, typeDefs)
		if err != nil {
			return InferToplevelResult{}, err
		}
		gen := types.Generalize(env, resolved)
		newEnv := env.extend("__ann:"+e.Name, gen)
		return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: newEnv, TypeDefs: typeDefs}, nil

	case ast.Let:
		if e.InExpr == nil {
			s, ty, newEnv, err := tc.toplevelLet(env, typeDefs, subst, e)
			if err != nil {
				return InferToplevelResult{}, err
			}
			return InferToplevelResult{Subst: s, Ty: ty, Env: newEnv, TypeDefs: typeDefs}, nil
		}

	case ast.LetRec:
		if e.InExpr == nil {
			s, genEnv, newEnv, err := tc.inferLetrecCore(env, typeDefs, subst, e.Bindings)
			if err != nil {
				return InferToplevelResult{}, err
			}
			// Check annotations for each binding
			for _, b := range e.Bindings {
				annScheme, err := tc.checkAnnotation(newEnv, genEnv[b.Name], b.Name)
				if err != nil {
					return InferToplevelResult{}, err
				}
				if annScheme != nil {
					newEnv[b.Name] = *annScheme
				}
			}
			// Clean up annotation keys
			for _, b := range e.Bindings {
				delete(newEnv, "__ann:"+b.Name)
			}
			lastName := e.Bindings[len(e.Bindings)-1].Name
			return InferToplevelResult{Subst: s, Ty: types.ApplySubst(s, genEnv[lastName].Ty), Env: newEnv, TypeDefs: typeDefs}, nil
		}

	case ast.LetPat:
		if e.InExpr == nil {
			s1, tBody, err := tc.infer(env, typeDefs, subst, e.Body)
			if err != nil {
				return InferToplevelResult{}, err
			}
			env1 := applySubstEnv(s1, env)
			s2, patTy, bindings, err := tc.inferPattern(e.Pat, env1, typeDefs, s1)
			if err != nil {
				return InferToplevelResult{}, err
			}
			s12 := types.ComposeSubst(s2, s1)
			s3, err := types.Unify(types.ApplySubst(s12, tBody), types.ApplySubst(s12, patTy))
			if err != nil {
				return InferToplevelResult{}, &types.TypeError{Msg: "in let pattern: " + err.Error()}
			}
			sFinal := types.ComposeSubst(s3, s12)
			appliedBindings := make(TypeEnv)
			for k, v := range bindings {
				appliedBindings[k] = types.ApplySubstScheme(sFinal, v)
			}
			newEnv := applySubstEnv(sFinal, env).extendMany(appliedBindings)
			return InferToplevelResult{Subst: sFinal, Ty: types.ApplySubst(sFinal, tBody), Env: newEnv, TypeDefs: typeDefs}, nil
		}

	case ast.Import:
		modResult, err := CheckModule(e.Module)
		if err != nil {
			return InferToplevelResult{}, err
		}
		modEnv := modResult.Env
		modCtorFamilies := modResult.CtorFamilies
		modTraits := modResult.Traits
		modInstances := modResult.TraitInstances
		if e.Alias != "" {
			modules := map[string]TypeEnv{}
			if m, ok := env["__modules__"]; ok {
				if mm, ok := m.(map[string]TypeEnv); ok {
					for k, v := range mm {
						modules[k] = v
					}
				}
			}
			modules[e.Alias] = modEnv
			ctorFamilies := mergeFamilies(env, modCtorFamilies)
			traits := mergeTraits(env, modTraits)
			instances := mergeInstances(env, modInstances)
			newEnv := env.clone()
			newEnv["__modules__"] = modules
			newEnv["__ctor_families__"] = ctorFamilies
			newEnv["__traits__"] = traits
			newEnv["__trait_instances__"] = instances
			return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: newEnv, TypeDefs: typeDefs}, nil
		}
		newEnv := env.clone()
		ctorFamilies := mergeFamilies(env, modCtorFamilies)
		traits := mergeTraits(env, modTraits)
		instances := mergeInstances(env, modInstances)
		for _, name := range e.Names {
			v, ok := modEnv[name]
			if !ok {
				return InferToplevelResult{}, &types.TypeError{Msg: fmt.Sprintf("'%s' is not exported by module '%s'", name, e.Module)}
			}
			newEnv[name] = v
			if cf, ok := modCtorFamilies[name]; ok {
				ctorFamilies[name] = cf
			}
		}
		newEnv["__ctor_families__"] = ctorFamilies
		newEnv["__traits__"] = traits
		newEnv["__trait_instances__"] = instances
		return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: newEnv, TypeDefs: typeDefs}, nil

	case ast.Export:
		return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: env, TypeDefs: typeDefs}, nil

	case ast.TraitDecl:
		traits := map[string]TraitInfo{}
		if t, ok := env["__traits__"]; ok {
			if tm, ok := t.(map[string]TraitInfo); ok {
				for k, v := range tm {
					traits[k] = v
				}
			}
		}
		methodsDict := map[string]types.Scheme{}
		newEnv := env.clone()
		for _, m := range e.Methods {
			ty, err := tc.resolveTypeSig(m.Type, typeDefs)
			if err != nil {
				return InferToplevelResult{}, err
			}
			fv := types.FreeVars(ty)
			var qvars []string
			if fv[e.Param] {
				qvars = []string{e.Param}
			}
			sc := types.Scheme{Vars: qvars, Ty: ty}
			methodsDict[m.Name] = sc
			newEnv[m.Name] = sc
		}
		traits[e.Name] = TraitInfo{Param: e.Param, Methods: methodsDict}
		newEnv["__traits__"] = traits
		return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: newEnv, TypeDefs: typeDefs}, nil

	case ast.TestDecl:
		// Type-check body in isolated env (bindings don't leak)
		testEnv := env.clone()
		testTypeDefs := make(map[string]types.Type, len(typeDefs))
		for k, v := range typeDefs {
			testTypeDefs[k] = v
		}
		for _, bodyExpr := range e.Body {
			res, err := tc.InferToplevel(testEnv, testTypeDefs, types.Subst{}, bodyExpr)
			if err != nil {
				return InferToplevelResult{}, err
			}
			testEnv = res.Env
			testTypeDefs = res.TypeDefs
		}
		return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: env, TypeDefs: typeDefs}, nil

	case ast.ImplDecl:
		traits := map[string]TraitInfo{}
		if t, ok := env["__traits__"]; ok {
			if tm, ok := t.(map[string]TraitInfo); ok {
				for k, v := range tm {
					traits[k] = v
				}
			}
		}
		traitInfo, ok := traits[e.TraitName]
		if !ok {
			return InferToplevelResult{}, &types.TypeError{Msg: "unknown trait: " + e.TraitName}
		}
		targetTy, err := tc.resolveTypeSig(ast.TyName{Name: e.TargetType}, typeDefs)
		if err != nil {
			return InferToplevelResult{}, err
		}
		implNames := map[string]bool{}
		for _, m := range e.Methods {
			implNames[m.Name] = true
		}
		for name := range traitInfo.Methods {
			if !implNames[name] {
				return InferToplevelResult{}, &types.TypeError{Msg: fmt.Sprintf("impl %s %s is missing: %s", e.TraitName, e.TargetType, name)}
			}
		}
		for _, m := range e.Methods {
			traitScheme, ok := traitInfo.Methods[m.Name]
			if !ok {
				return InferToplevelResult{}, &types.TypeError{Msg: fmt.Sprintf("'%s' is not a method of trait %s", m.Name, e.TraitName)}
			}
			paramSubst := types.Subst{traitInfo.Param: targetTy}
			expectedTy := types.ApplySubst(paramSubst, traitScheme.Ty)
			s1, actualTy, err := tc.infer(env, typeDefs, subst, m.Body)
			if err != nil {
				return InferToplevelResult{}, err
			}
			_, err = types.Unify(types.ApplySubst(s1, actualTy), types.ApplySubst(s1, expectedTy))
			if err != nil {
				return InferToplevelResult{}, &types.TypeError{Msg: fmt.Sprintf("in impl %s %s, method '%s': %s", e.TraitName, e.TargetType, m.Name, err.Error())}
			}
		}
		instances := map[string]map[string]bool{}
		if inst, ok := env["__trait_instances__"]; ok {
			if im, ok := inst.(map[string]map[string]bool); ok {
				for k, v := range im {
					instances[k] = v
				}
			}
		}
		key := e.TraitName + ":" + e.TargetType
		instances[key] = implNames
		newEnv := env.clone()
		newEnv["__trait_instances__"] = instances
		return InferToplevelResult{Subst: subst, Ty: types.TUnit, Env: newEnv, TypeDefs: typeDefs}, nil
	}

	// Regular expression
	s, ty, err := tc.infer(env, typeDefs, subst, expr)
	if err != nil {
		return InferToplevelResult{}, err
	}
	return InferToplevelResult{Subst: s, Ty: ty, Env: env, TypeDefs: typeDefs}, nil
}

// checkAnnotation checks if a pending annotation matches the inferred scheme.
// Returns the annotation scheme if it exists (to constrain the type), or nil.
func (tc *TypeChecker) checkAnnotation(env TypeEnv, inferred types.Scheme, name string) (*types.Scheme, error) {
	annKey := "__ann:" + name
	annVal, ok := env[annKey]
	if !ok {
		return nil, nil
	}
	annScheme, ok := annVal.(types.Scheme)
	if !ok {
		return nil, nil
	}
	annTy := tc.instantiate(annScheme)
	infTy := tc.instantiate(inferred)
	if _, err := types.Unify(annTy, infTy); err != nil {
		return nil, &types.TypeError{Msg: fmt.Sprintf(
			"type annotation mismatch for '%s': declared %s but inferred %s",
			name, types.TypeToString(annScheme.Ty), types.TypeToString(inferred.Ty),
		)}
	}
	return &annScheme, nil
}

func (tc *TypeChecker) toplevelLet(env TypeEnv, typeDefs map[string]types.Type, subst types.Subst, e ast.Let) (types.Subst, types.Type, TypeEnv, error) {
	if e.Recursive {
		tv := tc.fresh()
		env1 := env.extend(e.Name, types.Scheme{Ty: tv})
		s1, t1, err := tc.infer(env1, typeDefs, subst, e.Body)
		if err != nil {
			return nil, nil, nil, err
		}
		s2, err := types.Unify(types.ApplySubst(s1, tv), types.ApplySubst(s1, t1))
		if err != nil {
			return nil, nil, nil, &types.TypeError{Msg: fmt.Sprintf("in recursive let %s: %s", e.Name, err.Error())}
		}
		s12 := types.ComposeSubst(s2, s1)
		env2 := applySubstEnv(s12, env)
		gen := types.Generalize(env2, types.ApplySubst(s12, t1))
		annScheme, err := tc.checkAnnotation(env2, gen, e.Name)
		if err != nil {
			return nil, nil, nil, err
		}
		if annScheme != nil {
			gen = *annScheme
		}
		newEnv := env2.extend(e.Name, gen)
		delete(newEnv, "__ann:"+e.Name)
		return s12, types.ApplySubst(s12, gen.Ty), newEnv, nil
	}
	s1, t1, err := tc.infer(env, typeDefs, subst, e.Body)
	if err != nil {
		return nil, nil, nil, err
	}
	env1 := applySubstEnv(s1, env)
	gen := types.Generalize(env1, types.ApplySubst(s1, t1))
	annScheme, err := tc.checkAnnotation(env1, gen, e.Name)
	if err != nil {
		return nil, nil, nil, err
	}
	if annScheme != nil {
		gen = *annScheme
	}
	newEnv := env1.extend(e.Name, gen)
	delete(newEnv, "__ann:"+e.Name)
	return s1, types.ApplySubst(s1, gen.Ty), newEnv, nil
}

// ---------------------------------------------------------------------------
// Module loading
// ---------------------------------------------------------------------------

// TraitInfo stores information about a declared trait.
type TraitInfo struct {
	Param   string
	Methods map[string]types.Scheme
}

// ModuleResult is the result of type-checking a module.
type ModuleResult struct {
	Env            TypeEnv
	CtorFamilies   map[string]map[string]bool
	Traits         map[string]TraitInfo
	TraitInstances map[string]map[string]bool
}

var (
	moduleCache   = map[string]*ModuleResult{}
	moduleCacheMu sync.Mutex
)

// PreregisterTypes pre-registers all TypeDecl names so mutually recursive types can resolve each other.
func PreregisterTypes(exprs []ast.Expr, typeDefs map[string]types.Type) map[string]types.Type {
	result := make(map[string]types.Type, len(typeDefs))
	for k, v := range typeDefs {
		result[k] = v
	}
	for _, e := range exprs {
		if td, ok := e.(ast.TypeDecl); ok {
			if td.AliasType != nil {
				continue // aliases expand to their target, no new TCon
			}
			paramVars := make([]types.Type, len(td.Params))
			for i, p := range td.Params {
				paramVars[i] = types.TVar{Name: p}
			}
			result[td.Name] = types.TCon{Name: td.Name, Args: paramVars}
		}
	}
	return result
}

// CheckModule type-checks a stdlib module and caches the result.
func CheckModule(moduleName string) (*ModuleResult, error) {
	moduleCacheMu.Lock()
	if r, ok := moduleCache[moduleName]; ok {
		moduleCacheMu.Unlock()
		return r, nil
	}
	moduleCacheMu.Unlock()

	var name string
	if len(moduleName) > 4 && moduleName[:4] == "std:" {
		name = moduleName[4:]
	} else {
		return nil, &types.TypeError{Msg: fmt.Sprintf("bare module name '%s': use 'std:%s' for stdlib", moduleName, moduleName)}
	}

	src, err := stdlib.Source(name)
	if err != nil {
		return nil, &types.TypeError{Msg: "unknown module: " + moduleName}
	}

	exprs, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	tc := NewTypeChecker()
	prelude, err := loadPreludeTC()
	if err != nil {
		return nil, err
	}
	extraEnv := typeEnvForModule(name)
	env := prelude.Env.clone()
	for k, v := range extraEnv {
		env[k] = v
	}
	typeDefs := PreregisterTypes(exprs, copyTypeDefs(prelude.TypeDefs))
	exports := map[string]bool{}

	for _, expr := range exprs {
		if ex, ok := expr.(ast.Export); ok {
			for _, n := range ex.Names {
				exports[n] = true
			}
		} else if _, ok := expr.(ast.TestDecl); ok {
			// skip test blocks in imported modules
		} else {
			// Collect export names from Exported flags
			switch e := expr.(type) {
			case ast.Let:
				if e.Exported && e.InExpr == nil {
					exports[e.Name] = true
				}
			case ast.LetRec:
				if e.Exported && e.InExpr == nil {
					for _, b := range e.Bindings {
						exports[b.Name] = true
					}
				}
			case ast.LetPat:
				// LetPat exports are unusual; skip for now
			case ast.TypeDecl:
				if e.Exported {
					for _, ctor := range e.Ctors {
						exports[ctor.Name] = true
					}
				}
			case ast.TraitDecl:
				if e.Exported {
					for _, m := range e.Methods {
						exports[m.Name] = true
					}
				}
			}
			res, err := tc.InferToplevel(env, typeDefs, types.Subst{}, expr)
			if err != nil {
				return nil, err
			}
			env = res.Env
			typeDefs = res.TypeDefs
		}
	}

	exportedEnv := make(TypeEnv)
	for name := range exports {
		if v, ok := env[name]; ok {
			exportedEnv[name] = v
		}
	}
	ctorFamilies := map[string]map[string]bool{}
	if cf, ok := env["__ctor_families__"]; ok {
		if cfm, ok := cf.(map[string]map[string]bool); ok {
			ctorFamilies = cfm
		}
	}
	traits := map[string]TraitInfo{}
	if t, ok := env["__traits__"]; ok {
		if tm, ok := t.(map[string]TraitInfo); ok {
			traits = tm
		}
	}
	instances := map[string]map[string]bool{}
	if inst, ok := env["__trait_instances__"]; ok {
		if im, ok := inst.(map[string]map[string]bool); ok {
			instances = im
		}
	}

	result := &ModuleResult{
		Env:            exportedEnv,
		CtorFamilies:   ctorFamilies,
		Traits:         traits,
		TraitInstances: instances,
	}
	moduleCacheMu.Lock()
	moduleCache[moduleName] = result
	moduleCacheMu.Unlock()
	return result, nil
}

// ---------------------------------------------------------------------------
// Initial type environments
// ---------------------------------------------------------------------------

func mathTypeEnv() TypeEnv {
	return TypeEnv{
		"toFloat":  types.Scheme{Ty: types.TFun(types.TInt, types.TFloat)},
		"round":    types.Scheme{Ty: types.TFun(types.TFloat, types.TInt)},
		"floor":    types.Scheme{Ty: types.TFun(types.TFloat, types.TInt)},
		"ceiling":  types.Scheme{Ty: types.TFun(types.TFloat, types.TInt)},
		"truncate": types.Scheme{Ty: types.TFun(types.TFloat, types.TInt)},
		"sqrt":     types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"abs":      types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TVar{Name: "a"}, types.TVar{Name: "a"})},
		"min":      types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TVar{Name: "a"}, types.TFun(types.TVar{Name: "a"}, types.TVar{Name: "a"}))},
		"max":      types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TVar{Name: "a"}, types.TFun(types.TVar{Name: "a"}, types.TVar{Name: "a"}))},
		"pow":      types.Scheme{Ty: types.TFun(types.TFloat, types.TFun(types.TFloat, types.TFloat))},
		"sin":      types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"cos":      types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"tan":      types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"asin":     types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"acos":     types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"atan":     types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"atan2":    types.Scheme{Ty: types.TFun(types.TFloat, types.TFun(types.TFloat, types.TFloat))},
		"log":      types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"exp":      types.Scheme{Ty: types.TFun(types.TFloat, types.TFloat)},
		"pi":       types.Scheme{Ty: types.TFloat},
		"e":        types.Scheme{Ty: types.TFloat},
	}
}

func stringTypeEnv() TypeEnv {
	return TypeEnv{
		"length":       types.Scheme{Ty: types.TFun(types.TString, types.TInt)},
		"toUpper":      types.Scheme{Ty: types.TFun(types.TString, types.TString)},
		"toLower":      types.Scheme{Ty: types.TFun(types.TString, types.TString)},
		"trim":         types.Scheme{Ty: types.TFun(types.TString, types.TString)},
		"split":        types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TList(types.TString)))},
		"join":         types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TList(types.TString), types.TString))},
		"toString":     types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TVar{Name: "a"}, types.TString)},
		"contains":     types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TBool))},
		"startsWith":   types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TBool))},
		"endsWith":     types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TBool))},
		"charAt":       types.Scheme{Ty: types.TFun(types.TInt, types.TFun(types.TString, types.TMaybe(types.TString)))},
		"substring":    types.Scheme{Ty: types.TFun(types.TInt, types.TFun(types.TInt, types.TFun(types.TString, types.TString)))},
		"indexOf":      types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TMaybe(types.TInt)))},
		"replace":      types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TFun(types.TString, types.TString)))},
		"take":         types.Scheme{Ty: types.TFun(types.TInt, types.TFun(types.TString, types.TString))},
		"drop":         types.Scheme{Ty: types.TFun(types.TInt, types.TFun(types.TString, types.TString))},
		"repeat":       types.Scheme{Ty: types.TFun(types.TInt, types.TFun(types.TString, types.TString))},
		"padLeft":      types.Scheme{Ty: types.TFun(types.TInt, types.TFun(types.TString, types.TFun(types.TString, types.TString)))},
		"padRight":     types.Scheme{Ty: types.TFun(types.TInt, types.TFun(types.TString, types.TFun(types.TString, types.TString)))},
		"words":        types.Scheme{Ty: types.TFun(types.TString, types.TList(types.TString))},
		"lines":        types.Scheme{Ty: types.TFun(types.TString, types.TList(types.TString))},
		"charCode":     types.Scheme{Ty: types.TFun(types.TString, types.TInt)},
		"fromCharCode": types.Scheme{Ty: types.TFun(types.TInt, types.TString)},
		"parseInt":     types.Scheme{Ty: types.TFun(types.TString, types.TMaybe(types.TInt))},
		"parseFloat":   types.Scheme{Ty: types.TFun(types.TString, types.TMaybe(types.TFloat))},
		"reverse":      types.Scheme{Ty: types.TFun(types.TString, types.TString)},
		"toList":       types.Scheme{Ty: types.TFun(types.TString, types.TList(types.TString))},
		"fromList":     types.Scheme{Ty: types.TFun(types.TList(types.TString), types.TString)},
		"trimLeft":     types.Scheme{Ty: types.TFun(types.TString, types.TString)},
		"trimRight":    types.Scheme{Ty: types.TFun(types.TString, types.TString)},
	}
}

func listTypeEnv() TypeEnv {
	a := types.TVar{Name: "a"}
	ordering := types.TCon{Name: "Ordering", Args: nil}
	return TypeEnv{
		"sortWith": types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TFun(a, types.TFun(a, ordering)), types.TFun(types.TList(a), types.TList(a)))},
	}
}

func ioTypeEnv() TypeEnv {
	return TypeEnv{
		"print":      types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TVar{Name: "a"}, types.TVar{Name: "a"})},
		"println":    types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TVar{Name: "a"}, types.TVar{Name: "a"})},
		"readLine":   types.Scheme{Ty: types.TFun(types.TString, types.TString)},
		"readFile":   types.Scheme{Ty: types.TFun(types.TString, types.TResult(types.TString, types.TString))},
		"writeFile":  types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TResult(types.TUnit, types.TString)))},
		"appendFile": types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TResult(types.TUnit, types.TString)))},
		"fileExists": types.Scheme{Ty: types.TFun(types.TString, types.TBool)},
		"listDir":    types.Scheme{Ty: types.TFun(types.TString, types.TResult(types.TList(types.TString), types.TString))},
	}
}

func envTypeEnv() TypeEnv {
	return TypeEnv{
		"getEnv":   types.Scheme{Ty: types.TFun(types.TString, types.TMaybe(types.TString))},
		"getEnvOr": types.Scheme{Ty: types.TFun(types.TString, types.TFun(types.TString, types.TString))},
		"args":     types.Scheme{Ty: types.TList(types.TString)},
	}
}

func jsonTypeEnv() TypeEnv {
	tJson := types.TCon{Name: "Json", Args: nil}
	return TypeEnv{
		"jsonParse": types.Scheme{Ty: types.TFun(types.TString, types.TResult(tJson, types.TString))},
	}
}

// InitialTypeEnv returns the type environment with only globally available builtins.
func processTypeEnv() TypeEnv {
	a := types.TVar{Name: "a"}
	b := types.TVar{Name: "b"}
	return TypeEnv{
		// spawn : (() -> b) -> Pid a
		"spawn": types.Scheme{Vars: []string{"a", "b"}, Ty: types.TFun(types.TFun(types.TUnit, b), types.TPid(a))},
		// send : Pid a -> a -> ()
		"send": types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TPid(a), types.TFun(a, types.TUnit))},
		// receive : () -> a
		"receive": types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TUnit, a)},
		// self : Pid a
		"self": types.Scheme{Vars: []string{"a"}, Ty: types.TPid(a)},
		// call : Pid b -> (Pid a -> b) -> a
		"call": types.Scheme{Vars: []string{"a", "b"}, Ty: types.TFun(types.TPid(b), types.TFun(types.TFun(types.TPid(a), b), a))},
	}
}

func InitialTypeEnv() TypeEnv {
	env := TypeEnv{
		"not":       types.Scheme{Ty: types.TFun(types.TBool, types.TBool)},
		"error":     types.Scheme{Vars: []string{"a"}, Ty: types.TFun(types.TString, types.TVar{Name: "a"})},
		"showInt":   types.Scheme{Ty: types.TFun(types.TInt, types.TString)},
		"showFloat": types.Scheme{Ty: types.TFun(types.TFloat, types.TString)},
	}
	for k, v := range processTypeEnv() {
		env[k] = v
	}
	return env
}

func typeEnvForModule(name string) TypeEnv {
	result := InitialTypeEnv()
	switch name {
	case "List":
		for k, v := range listTypeEnv() {
			result[k] = v
		}
	case "Math":
		for k, v := range mathTypeEnv() {
			result[k] = v
		}
	case "String":
		for k, v := range stringTypeEnv() {
			result[k] = v
		}
	case "IO":
		for k, v := range ioTypeEnv() {
			result[k] = v
		}
	case "Env":
		for k, v := range envTypeEnv() {
			result[k] = v
		}
	case "Json":
		for k, v := range jsonTypeEnv() {
			result[k] = v
		}
	case "Process":
		for k, v := range processTypeEnv() {
			result[k] = v
		}
	case "Parallel":
		for k, v := range parallelTypeEnv() {
			result[k] = v
		}
		for k, v := range processTypeEnv() {
			result[k] = v
		}
	}
	return result
}

func parallelTypeEnv() TypeEnv {
	return TypeEnv{
		"numCPU": types.Scheme{Ty: types.TInt},
	}
}

// TypeEnvForModule is the exported version of typeEnvForModule.
func TypeEnvForModule(name string) TypeEnv {
	return typeEnvForModule(name)
}

// ---------------------------------------------------------------------------
// Prelude cache
// ---------------------------------------------------------------------------

// PreludeTC holds the type-checking result of the Prelude.
type PreludeTC struct {
	Env      TypeEnv
	TypeDefs map[string]types.Type
}

type preludeTC = PreludeTC

var (
	preludeTCCache *preludeTC
	preludeTCMu    sync.Mutex
)

func loadPreludeTC() (*preludeTC, error) {
	preludeTCMu.Lock()
	defer preludeTCMu.Unlock()
	if preludeTCCache != nil {
		return preludeTCCache, nil
	}
	src, err := stdlib.Source("Prelude")
	if err != nil {
		return nil, err
	}
	exprs, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	tc := NewTypeChecker()
	env := InitialTypeEnv()
	typeDefs := PreregisterTypes(exprs, map[string]types.Type{})
	for _, expr := range exprs {
		res, err := tc.InferToplevel(env, typeDefs, types.Subst{}, expr)
		if err != nil {
			return nil, err
		}
		env = res.Env
		typeDefs = res.TypeDefs
	}
	// Register Pid as a builtin parameterized type so type annotations can reference it.
	typeDefs["Pid"] = types.TCon{Name: "Pid", Args: []types.Type{types.TVar{Name: "a"}}}
	preludeTCCache = &preludeTC{Env: env, TypeDefs: typeDefs}
	return preludeTCCache, nil
}

// LoadPreludeTC loads and caches the Prelude type environment.
func LoadPreludeTC() (*PreludeTC, error) {
	return loadPreludeTC()
}

// CopyTypeDefs copies a type definitions map.
func CopyTypeDefs(td map[string]types.Type) map[string]types.Type {
	return copyTypeDefs(td)
}

// CheckProgram type-checks a list of top-level expressions.
func CheckProgram(exprs []ast.Expr) (TypeEnv, error) {
	tc := NewTypeChecker()
	prelude, err := loadPreludeTC()
	if err != nil {
		return nil, err
	}
	env := prelude.Env.clone()
	typeDefs := PreregisterTypes(exprs, copyTypeDefs(prelude.TypeDefs))
	ordered, err := ReorderToplevel(exprs)
	if err != nil {
		return nil, err
	}
	for _, expr := range ordered {
		res, err := tc.InferToplevel(env, typeDefs, types.Subst{}, expr)
		if err != nil {
			return nil, err
		}
		env = res.Env
		typeDefs = res.TypeDefs
	}
	return env, nil
}

// ---------------------------------------------------------------------------
// Top-level reordering (dependency analysis)
// ---------------------------------------------------------------------------

// ReorderToplevel reorders top-level expressions so that:
//  1. Let/LetRec/LetPat bindings are topologically sorted by dependency,
//     filling the same positional slots they originally occupied
//  2. Non-binding exprs keep their original positions relative to bindings
//  3. Tests are moved to the end
//
// TypeAnnotations travel with their corresponding binding.
// Returns an error if mutually recursive bindings are detected.
func ReorderToplevel(exprs []ast.Expr) ([]ast.Expr, error) {
	type bindingEntry struct {
		expr       ast.Expr
		names      []string
		annotation *ast.TypeAnnotation
	}

	// First pass: collect all binding names so we know which annotations to pair
	bindingNameSet := map[string]bool{}
	for _, e := range exprs {
		switch e := e.(type) {
		case ast.Let:
			if e.InExpr == nil {
				bindingNameSet[e.Name] = true
			}
		case ast.LetRec:
			if e.InExpr == nil {
				for _, b := range e.Bindings {
					bindingNameSet[b.Name] = true
				}
			}
		case ast.LetPat:
			if e.InExpr == nil {
				for _, n := range patNames(e.Pat) {
					bindingNameSet[n] = true
				}
			}
		}
	}

	// Second pass: build entries list tracking which positions are binding slots
	const (
		slotFixed   = 0
		slotBinding = 1
	)
	type slot struct {
		kind int
		bidx int      // index into bindings (for slotBinding)
		expr ast.Expr // for slotFixed
	}

	var slots []slot
	var bindings []bindingEntry
	var tests []ast.Expr
	annotations := map[string]ast.TypeAnnotation{}

	for _, e := range exprs {
		switch e := e.(type) {
		case ast.TestDecl:
			tests = append(tests, e)
		case ast.TypeAnnotation:
			if bindingNameSet[e.Name] {
				annotations[e.Name] = e
			} else {
				slots = append(slots, slot{kind: slotFixed, expr: e})
			}
		case ast.Let:
			if e.InExpr == nil {
				var ann *ast.TypeAnnotation
				if a, ok := annotations[e.Name]; ok {
					ann = &a
					delete(annotations, e.Name)
				}
				bidx := len(bindings)
				bindings = append(bindings, bindingEntry{expr: e, names: []string{e.Name}, annotation: ann})
				slots = append(slots, slot{kind: slotBinding, bidx: bidx})
			} else {
				slots = append(slots, slot{kind: slotFixed, expr: e})
			}
		case ast.LetRec:
			if e.InExpr == nil {
				names := make([]string, len(e.Bindings))
				for j, b := range e.Bindings {
					names[j] = b.Name
				}
				// Check for annotations on any of the bindings
				var ann *ast.TypeAnnotation
				for _, n := range names {
					if a, ok := annotations[n]; ok {
						ann = &a
						delete(annotations, n)
						break // LetRec group gets at most one prepended annotation
					}
				}
				bidx := len(bindings)
				bindings = append(bindings, bindingEntry{expr: e, names: names, annotation: ann})
				slots = append(slots, slot{kind: slotBinding, bidx: bidx})
			} else {
				slots = append(slots, slot{kind: slotFixed, expr: e})
			}
		case ast.LetPat:
			if e.InExpr == nil {
				pnames := patNames(e.Pat)
				bidx := len(bindings)
				bindings = append(bindings, bindingEntry{expr: e, names: pnames})
				slots = append(slots, slot{kind: slotBinding, bidx: bidx})
			} else {
				slots = append(slots, slot{kind: slotFixed, expr: e})
			}
		default:
			slots = append(slots, slot{kind: slotFixed, expr: e})
		}
	}

	if len(bindings) <= 1 {
		// Nothing to reorder — just reassemble from slots + tests
		result := make([]ast.Expr, 0, len(exprs))
		for _, s := range slots {
			if s.kind == slotFixed {
				result = append(result, s.expr)
			} else {
				b := bindings[s.bidx]
				if b.annotation != nil {
					result = append(result, *b.annotation)
				}
				result = append(result, b.expr)
			}
		}
		result = append(result, tests...)
		return result, nil
	}

	// Build name → binding index map
	nameToIdx := map[string]int{}
	for i, b := range bindings {
		for _, n := range b.names {
			nameToIdx[n] = i
		}
	}

	// Compute dependencies for each binding
	deps := make([]map[int]bool, len(bindings))
	dependents := make([][]int, len(bindings))
	for i, b := range bindings {
		fv := bindingFreeVars(b.expr)
		for _, n := range b.names {
			delete(fv, n)
		}
		deps[i] = map[int]bool{}
		for v := range fv {
			if j, ok := nameToIdx[v]; ok && j != i {
				deps[i][j] = true
			}
		}
	}

	// Build reverse adjacency
	for i, d := range deps {
		for j := range d {
			dependents[j] = append(dependents[j], i)
		}
	}

	// Kahn's algorithm with stability (prefer lower index for determinism)
	inDegree := make([]int, len(bindings))
	for i := range bindings {
		inDegree[i] = len(deps[i])
	}

	var queue []int
	for i, d := range inDegree {
		if d == 0 {
			queue = append(queue, i)
		}
	}

	var sorted []int
	for len(queue) > 0 {
		minPos := 0
		for j := 1; j < len(queue); j++ {
			if queue[j] < queue[minPos] {
				minPos = j
			}
		}
		node := queue[minPos]
		queue[minPos] = queue[len(queue)-1]
		queue = queue[:len(queue)-1]

		sorted = append(sorted, node)

		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(sorted) != len(bindings) {
		var cycleNames []string
		for i, d := range inDegree {
			if d > 0 {
				cycleNames = append(cycleNames, strings.Join(bindings[i].names, ", "))
			}
		}
		return nil, &types.TypeError{Msg: fmt.Sprintf(
			"mutually recursive bindings: %s — use let rec ... and ...",
			strings.Join(cycleNames, ", "),
		)}
	}

	// Reassemble: walk slots, filling binding slots with sorted bindings in order
	result := make([]ast.Expr, 0, len(exprs))
	sortedPos := 0
	for _, s := range slots {
		if s.kind == slotFixed {
			result = append(result, s.expr)
		} else {
			b := bindings[sorted[sortedPos]]
			if b.annotation != nil {
				result = append(result, *b.annotation)
			}
			result = append(result, b.expr)
			sortedPos++
		}
	}
	result = append(result, tests...)
	return result, nil
}

// bindingFreeVars returns the free variables of a top-level binding's body.
func bindingFreeVars(expr ast.Expr) map[string]bool {
	result := map[string]bool{}
	switch e := expr.(type) {
	case ast.Let:
		bound := map[string]bool{}
		if e.Recursive {
			bound[e.Name] = true
		}
		freeVarsRec(e.Body, bound, result)
	case ast.LetRec:
		bound := map[string]bool{}
		for _, b := range e.Bindings {
			bound[b.Name] = true
		}
		for _, b := range e.Bindings {
			freeVarsRec(b.Body, bound, result)
		}
	case ast.LetPat:
		freeVarsRec(e.Body, map[string]bool{}, result)
	}
	return result
}

// freeVarsRec walks an expression collecting free variable references.
func freeVarsRec(expr ast.Expr, bound map[string]bool, free map[string]bool) {
	switch e := expr.(type) {
	case ast.Var:
		if !bound[e.Name] {
			free[e.Name] = true
		}
	case ast.IntLit, ast.FloatLit, ast.StringLit, ast.BoolLit, ast.UnitLit:
		// no free vars
	case ast.UnaryMinus:
		freeVarsRec(e.Expr, bound, free)
	case ast.Binop:
		freeVarsRec(e.Left, bound, free)
		freeVarsRec(e.Right, bound, free)
	case ast.If:
		freeVarsRec(e.Cond, bound, free)
		freeVarsRec(e.ThenExpr, bound, free)
		freeVarsRec(e.ElseExpr, bound, free)
	case ast.Let:
		if e.Recursive {
			newBound := copyStringSet(bound)
			newBound[e.Name] = true
			freeVarsRec(e.Body, newBound, free)
			if e.InExpr != nil {
				freeVarsRec(e.InExpr, newBound, free)
			}
		} else {
			freeVarsRec(e.Body, bound, free)
			if e.InExpr != nil {
				newBound := copyStringSet(bound)
				newBound[e.Name] = true
				freeVarsRec(e.InExpr, newBound, free)
			}
		}
	case ast.LetRec:
		newBound := copyStringSet(bound)
		for _, b := range e.Bindings {
			newBound[b.Name] = true
		}
		for _, b := range e.Bindings {
			freeVarsRec(b.Body, newBound, free)
		}
		if e.InExpr != nil {
			freeVarsRec(e.InExpr, newBound, free)
		}
	case ast.LetPat:
		freeVarsRec(e.Body, bound, free)
		if e.InExpr != nil {
			newBound := copyStringSet(bound)
			for _, n := range patNames(e.Pat) {
				newBound[n] = true
			}
			freeVarsRec(e.InExpr, newBound, free)
		}
	case ast.Fun:
		newBound := copyStringSet(bound)
		newBound[e.Param] = true
		freeVarsRec(e.Body, newBound, free)
	case ast.App:
		freeVarsRec(e.Func, bound, free)
		freeVarsRec(e.Arg, bound, free)
	case ast.Match:
		freeVarsRec(e.Scrutinee, bound, free)
		for _, arm := range e.Arms {
			newBound := copyStringSet(bound)
			for _, n := range patNames(arm.Pat) {
				newBound[n] = true
			}
			freeVarsRec(arm.Body, newBound, free)
		}
	case ast.ListLit:
		for _, item := range e.Items {
			freeVarsRec(item, bound, free)
		}
	case ast.TupleLit:
		for _, item := range e.Items {
			freeVarsRec(item, bound, free)
		}
	case ast.StringInterp:
		for _, part := range e.Parts {
			freeVarsRec(part, bound, free)
		}
	case ast.RecordCreate:
		for _, f := range e.Fields {
			freeVarsRec(f.Value, bound, free)
		}
	case ast.FieldAccess:
		freeVarsRec(e.Record, bound, free)
	case ast.RecordUpdate:
		freeVarsRec(e.Record, bound, free)
		for _, u := range e.Updates {
			freeVarsRec(u.Value, bound, free)
		}
	case ast.Assert:
		freeVarsRec(e.Expr, bound, free)
	case ast.ImplDecl:
		for _, m := range e.Methods {
			freeVarsRec(m.Body, bound, free)
		}
	case ast.TestDecl:
		for _, bodyExpr := range e.Body {
			freeVarsRec(bodyExpr, bound, free)
		}
		// DotAccess, TypeDecl, Import, Export, TraitDecl, TypeAnnotation: no free vars
	}
}

// patNames extracts all variable names bound by a pattern.
func patNames(pat ast.Pattern) []string {
	var names []string
	collectPatNames(pat, &names)
	return names
}

func collectPatNames(pat ast.Pattern, names *[]string) {
	switch p := pat.(type) {
	case ast.PVar:
		*names = append(*names, p.Name)
	case ast.PCons:
		collectPatNames(p.Head, names)
		collectPatNames(p.Tail, names)
	case ast.PTuple:
		for _, pp := range p.Pats {
			collectPatNames(pp, names)
		}
	case ast.PCtor:
		for _, pp := range p.Args {
			collectPatNames(pp, names)
		}
	case ast.PRecord:
		for _, f := range p.Fields {
			collectPatNames(f.Pat, names)
		}
	}
}

func copyStringSet(s map[string]bool) map[string]bool {
	out := make(map[string]bool, len(s))
	for k := range s {
		out[k] = true
	}
	return out
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func applySubstEnv(s types.Subst, env TypeEnv) TypeEnv {
	result := make(TypeEnv, len(env))
	for k, v := range env {
		if sc, ok := v.(types.Scheme); ok {
			result[k] = types.ApplySubstScheme(s, sc)
		} else {
			result[k] = v
		}
	}
	return result
}

func copyTypeDefs(td map[string]types.Type) map[string]types.Type {
	result := make(map[string]types.Type, len(td))
	for k, v := range td {
		result[k] = v
	}
	return result
}

func mergeFamilies(env TypeEnv, extra map[string]map[string]bool) map[string]map[string]bool {
	result := map[string]map[string]bool{}
	if cf, ok := env["__ctor_families__"]; ok {
		if cfm, ok := cf.(map[string]map[string]bool); ok {
			for k, v := range cfm {
				result[k] = v
			}
		}
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

func mergeTraits(env TypeEnv, extra map[string]TraitInfo) map[string]TraitInfo {
	result := map[string]TraitInfo{}
	if t, ok := env["__traits__"]; ok {
		if tm, ok := t.(map[string]TraitInfo); ok {
			for k, v := range tm {
				result[k] = v
			}
		}
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

func mergeInstances(env TypeEnv, extra map[string]map[string]bool) map[string]map[string]bool {
	result := map[string]map[string]bool{}
	if inst, ok := env["__trait_instances__"]; ok {
		if im, ok := inst.(map[string]map[string]bool); ok {
			for k, v := range im {
				result[k] = v
			}
		}
	}
	for k, v := range extra {
		result[k] = v
	}
	return result
}

// CheckProgramWithExtraEnv is like CheckProgram but with additional type env injected.
func CheckProgramWithExtraEnv(exprs []ast.Expr, extraEnv TypeEnv) (TypeEnv, error) {
	tc := NewTypeChecker()
	prelude, err := loadPreludeTC()
	if err != nil {
		return nil, err
	}
	env := prelude.Env.clone()
	for k, v := range extraEnv {
		env[k] = v
	}
	typeDefs := PreregisterTypes(exprs, copyTypeDefs(prelude.TypeDefs))
	ordered, err := ReorderToplevel(exprs)
	if err != nil {
		return nil, err
	}
	for _, expr := range ordered {
		res, err := tc.InferToplevel(env, typeDefs, types.Subst{}, expr)
		if err != nil {
			return nil, err
		}
		env = res.Env
		typeDefs = res.TypeDefs
	}
	return env, nil
}
