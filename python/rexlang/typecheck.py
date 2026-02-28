# Hindley-Milner type inference (Algorithm W) for RexLang.
#
# Pipeline: source → lexer → parser → typecheck → eval
# Type errors are fatal: they prevent evaluation.

import os

from . import ast
from . import parser as parser_mod
from .types import (
    TVar,
    TCon,
    Scheme,
    TInt,
    TFloat,
    TString,
    TBool,
    TUnit,
    TFun,
    TList,
    TTuple,
    TMaybe,
    TResult,
    apply_subst,
    apply_subst_scheme,
    apply_subst_env,
    compose_subst,
    free_vars,
    unify,
    generalize,
    type_to_string,
    TypeError,
)

STDLIB_DIR = os.path.join(os.path.dirname(__file__), "stdlib")


# ---------------------------------------------------------------------------
# TypeChecker
# ---------------------------------------------------------------------------


class TypeChecker:
    def __init__(self):
        self._counter = 0

    def fresh(self) -> TVar:
        self._counter += 1
        return TVar(f"t{self._counter}")

    def instantiate(self, scheme: Scheme):
        """Replace quantified vars with fresh type variables."""
        subst = {v: self.fresh() for v in scheme.vars}
        return apply_subst(subst, scheme.ty)

    # -----------------------------------------------------------------------
    # Pattern inference
    # -----------------------------------------------------------------------

    def infer_pattern(self, pat, env: dict, type_defs: dict, subst: dict):
        """
        Infer the type of a pattern.
        Returns (subst, pat_type, bindings) where bindings is {name: Scheme}.
        """
        if isinstance(pat, ast.PWild):
            tv = self.fresh()
            return subst, tv, {}

        elif isinstance(pat, ast.PUnit):
            return subst, TUnit, {}

        elif isinstance(pat, ast.PVar):
            tv = self.fresh()
            return subst, tv, {pat.name: Scheme([], tv)}

        elif isinstance(pat, ast.PInt):
            return subst, TInt, {}

        elif isinstance(pat, ast.PFloat):
            return subst, TFloat, {}

        elif isinstance(pat, ast.PString):
            return subst, TString, {}

        elif isinstance(pat, ast.PBool):
            return subst, TBool, {}

        elif isinstance(pat, ast.PNil):
            tv = self.fresh()
            return subst, TList(tv), {}

        elif isinstance(pat, ast.PCons):
            tv = self.fresh()
            s1, th, hbinds = self.infer_pattern(pat.head, env, type_defs, subst)
            try:
                s2 = unify(apply_subst(s1, th), apply_subst(s1, tv))
            except TypeError as e:
                raise TypeError(f"in cons pattern head: {e}")
            s12 = compose_subst(s2, s1)
            s3, tt, tbinds = self.infer_pattern(pat.tail, env, type_defs, s12)
            list_tv = TList(apply_subst(compose_subst(s3, s12), tv))
            try:
                s4 = unify(apply_subst(s3, tt), list_tv)
            except TypeError as e:
                raise TypeError(f"in cons pattern tail: {e}")
            s_final = compose_subst(s4, compose_subst(s3, s12))
            return s_final, TList(apply_subst(s_final, tv)), {**hbinds, **tbinds}

        elif isinstance(pat, ast.PTuple):
            s = subst
            item_types = []
            all_binds = {}
            for p in pat.pats:
                s1, pt, binds = self.infer_pattern(p, env, type_defs, s)
                s = compose_subst(s1, s)
                item_types.append(pt)
                all_binds.update(binds)
            final_types = [apply_subst(s, t) for t in item_types]
            return s, TTuple(final_types), all_binds

        elif isinstance(pat, ast.PCtor):
            if pat.name not in env:
                raise TypeError(f"unknown constructor: {pat.name}")
            ctor_ty = self.instantiate(env[pat.name])
            try:
                arg_tys, result_ty = self._decompose_fun(ctor_ty, len(pat.args))
            except TypeError:
                raise TypeError(
                    f"constructor {pat.name} applied to wrong number of arguments"
                )
            s = subst
            all_binds = {}
            for arg_pat, expected_ty in zip(pat.args, arg_tys):
                s1, pat_ty, binds = self.infer_pattern(arg_pat, env, type_defs, s)
                try:
                    s2 = unify(apply_subst(s1, pat_ty), apply_subst(s1, expected_ty))
                except TypeError as e:
                    raise TypeError(f"in constructor pattern {pat.name}: {e}")
                s = compose_subst(s2, s1)
                all_binds.update(binds)
            return s, apply_subst(s, result_ty), all_binds

        raise TypeError(f"unknown pattern type: {type(pat)}")

    # -----------------------------------------------------------------------
    # Expression inference
    # -----------------------------------------------------------------------

    def infer(self, env: dict, type_defs: dict, subst: dict, expr):
        """
        Infer the type of an expression.
        Returns (subst, type).
        """
        if isinstance(expr, ast.Int):
            return subst, TInt

        elif isinstance(expr, ast.Float):
            return subst, TFloat

        elif isinstance(expr, ast.String):
            return subst, TString

        elif isinstance(expr, ast.Bool):
            return subst, TBool

        elif isinstance(expr, ast.Unit):
            return subst, TUnit

        elif isinstance(expr, ast.Var):
            if expr.name not in env:
                raise TypeError(f"unbound variable: {expr.name}")
            return subst, self.instantiate(env[expr.name])

        elif isinstance(expr, ast.Unary_minus):
            # Typed as a -> a (polymorphic numeric negation)
            s, t = self.infer(env, type_defs, subst, expr.expr)
            return s, t

        elif isinstance(expr, ast.Binop):
            return self._infer_binop(env, type_defs, subst, expr)

        elif isinstance(expr, ast.If):
            s1, tc = self.infer(env, type_defs, subst, expr.cond)
            try:
                s2 = unify(apply_subst(s1, tc), TBool)
            except TypeError:
                raise TypeError(
                    f"if condition must be Bool, got {type_to_string(apply_subst(s1, tc))}"
                )
            s12 = compose_subst(s2, s1)
            env12 = apply_subst_env(s12, env)
            s3, tt = self.infer(env12, type_defs, s12, expr.then_expr)
            s123 = compose_subst(s3, s12)
            env123 = apply_subst_env(s123, env)
            s4, te = self.infer(env123, type_defs, s123, expr.else_expr)
            s1234 = compose_subst(s4, s123)
            try:
                s5 = unify(apply_subst(s4, tt), apply_subst(s4, te))
            except TypeError:
                raise TypeError(
                    f"if branches have different types: "
                    f"{type_to_string(apply_subst(s4, tt))} vs "
                    f"{type_to_string(apply_subst(s4, te))}"
                )
            s_final = compose_subst(s5, s1234)
            return s_final, apply_subst(s_final, tt)

        elif isinstance(expr, ast.Fun):
            tv = self.fresh()
            env1 = {**env, expr.param: Scheme([], tv)}
            s1, t_body = self.infer(env1, type_defs, subst, expr.body)
            return s1, TFun(apply_subst(s1, tv), t_body)

        elif isinstance(expr, ast.App):
            s1, tf = self.infer(env, type_defs, subst, expr.func)
            s2, ta = self.infer(
                apply_subst_env(s1, env), type_defs, compose_subst(s1, subst), expr.arg
            )
            tr = self.fresh()
            try:
                s3 = unify(apply_subst(s2, tf), TFun(ta, tr))
            except TypeError:
                raise TypeError(
                    f"cannot apply {type_to_string(apply_subst(s2, tf))} to argument of type {type_to_string(ta)}"
                )
            s_final = compose_subst(s3, compose_subst(s2, s1))
            return s_final, apply_subst(s_final, tr)

        elif isinstance(expr, ast.Let):
            return self._infer_let(env, type_defs, subst, expr)

        elif isinstance(expr, ast.LetRec):
            return self._infer_letrec(env, type_defs, subst, expr)

        elif isinstance(expr, ast.LetPat):
            s1, t_body = self.infer(env, type_defs, subst, expr.body)
            env1 = apply_subst_env(s1, env)
            s2, pat_ty, bindings = self.infer_pattern(expr.pat, env1, type_defs, s1)
            s12 = compose_subst(s2, s1)
            try:
                s3 = unify(apply_subst(s12, t_body), apply_subst(s12, pat_ty))
            except TypeError as e:
                raise TypeError(f"in let pattern: {e}")
            s_final = compose_subst(s3, s12)
            if expr.in_expr is not None:
                applied_bindings = {
                    k: apply_subst_scheme(s_final, v) for k, v in bindings.items()
                }
                env2 = {**apply_subst_env(s_final, env), **applied_bindings}
                s4, t_in = self.infer(env2, type_defs, s_final, expr.in_expr)
                return compose_subst(s4, s_final), t_in
            else:
                return s_final, apply_subst(s_final, t_body)

        elif isinstance(expr, ast.Match):
            return self._infer_match(env, type_defs, subst, expr)

        elif isinstance(expr, ast.ListLit):
            tv = self.fresh()
            s = subst
            for item in expr.items:
                s1, ti = self.infer(apply_subst_env(s, env), type_defs, s, item)
                try:
                    s2 = unify(apply_subst(s1, ti), apply_subst(s1, tv))
                except TypeError:
                    raise TypeError(
                        f"list elements must all have the same type: "
                        f"expected {type_to_string(apply_subst(s1, tv))}, "
                        f"got {type_to_string(apply_subst(s1, ti))}"
                    )
                s = compose_subst(s2, s1)
                tv = apply_subst(s, tv)
            return s, TList(apply_subst(s, tv))

        elif isinstance(expr, ast.Tuple):
            s = subst
            item_types = []
            for item in expr.items:
                s1, ti = self.infer(apply_subst_env(s, env), type_defs, s, item)
                s = compose_subst(s1, s)
                item_types.append(ti)
            return s, TTuple([apply_subst(s, t) for t in item_types])

        elif isinstance(expr, ast.DotAccess):
            modules = env.get("__modules__", {})
            if expr.module_name not in modules:
                raise TypeError(f"'{expr.module_name}' is not a qualified module")
            mod_env = modules[expr.module_name]
            if expr.field_name not in mod_env:
                raise TypeError(
                    f"module '{expr.module_name}' does not export '{expr.field_name}'"
                )
            return subst, self.instantiate(mod_env[expr.field_name])

        elif isinstance(expr, ast.Assert):
            s1, t = self.infer(env, type_defs, subst, expr.expr)
            try:
                s2 = unify(apply_subst(s1, t), TBool)
            except TypeError:
                raise TypeError(
                    f"assert requires Bool, got {type_to_string(apply_subst(s1, t))}"
                )
            return compose_subst(s2, s1), TUnit

        elif isinstance(
            expr, (ast.TypeDecl, ast.Import, ast.Export, ast.TraitDecl, ast.ImplDecl)
        ):
            # Should be handled by infer_toplevel; treat as no-op here
            return subst, TUnit

        raise TypeError(f"unknown AST node: {type(expr)}")

    def _infer_binop(self, env, type_defs, subst, expr):
        op = expr.op
        if op in ("Add", "Sub", "Mul", "Div", "Mod"):
            s1, tl = self.infer(env, type_defs, subst, expr.left)
            s2, tr = self.infer(
                apply_subst_env(s1, env),
                type_defs,
                compose_subst(s1, subst),
                expr.right,
            )
            s12 = compose_subst(s2, s1)
            try:
                s3 = unify(apply_subst(s12, tl), apply_subst(s12, tr))
            except TypeError:
                raise TypeError(
                    f"arithmetic type mismatch: {type_to_string(apply_subst(s12, tl))} vs "
                    f"{type_to_string(apply_subst(s12, tr))}"
                )
            s_final = compose_subst(s3, s12)
            result_ty = apply_subst(s_final, tl)

            # Restrict arithmetic to numeric types (sound type system)
            if isinstance(result_ty, TVar):
                # Free type variable: default to Int (numeric defaulting)
                s_final = compose_subst({result_ty.name: TInt}, s_final)
                result_ty = TInt
            elif result_ty not in (TInt, TFloat):
                raise TypeError(
                    f"arithmetic requires Int or Float, got {type_to_string(result_ty)}"
                )

            # Mod is Int-only (no float modulo)
            if op == "Mod" and result_ty != TInt:
                raise TypeError(
                    f"(%) requires Int operands, got {type_to_string(result_ty)}"
                )

            return s_final, result_ty

        elif op == "Concat":
            s1, tl = self.infer(env, type_defs, subst, expr.left)
            try:
                s2 = unify(apply_subst(s1, tl), TString)
            except TypeError:
                raise TypeError(
                    f"(++) requires String, got {type_to_string(apply_subst(s1, tl))}"
                )
            s12 = compose_subst(s2, s1)
            s3, tr = self.infer(apply_subst_env(s12, env), type_defs, s12, expr.right)
            s123 = compose_subst(s3, s12)
            try:
                s4 = unify(apply_subst(s123, tr), TString)
            except TypeError:
                raise TypeError(
                    f"(++) requires String, got {type_to_string(apply_subst(s123, tr))}"
                )
            return compose_subst(s4, s123), TString

        elif op in ("And", "Or"):
            s1, tl = self.infer(env, type_defs, subst, expr.left)
            try:
                s2 = unify(apply_subst(s1, tl), TBool)
            except TypeError:
                raise TypeError(
                    f"({op}) requires Bool, got {type_to_string(apply_subst(s1, tl))}"
                )
            s12 = compose_subst(s2, s1)
            s3, tr = self.infer(apply_subst_env(s12, env), type_defs, s12, expr.right)
            s123 = compose_subst(s3, s12)
            try:
                s4 = unify(apply_subst(s123, tr), TBool)
            except TypeError:
                raise TypeError(
                    f"({op}) requires Bool, got {type_to_string(apply_subst(s123, tr))}"
                )
            return compose_subst(s4, s123), TBool

        elif op in ("Lt", "Gt", "Leq", "Geq", "Eq", "Neq"):
            s1, tl = self.infer(env, type_defs, subst, expr.left)
            s2, tr = self.infer(
                apply_subst_env(s1, env),
                type_defs,
                compose_subst(s1, subst),
                expr.right,
            )
            s12 = compose_subst(s2, s1)
            try:
                s3 = unify(apply_subst(s12, tl), apply_subst(s12, tr))
            except TypeError:
                raise TypeError(
                    f"comparison type mismatch: {type_to_string(apply_subst(s12, tl))} vs "
                    f"{type_to_string(apply_subst(s12, tr))}"
                )
            return compose_subst(s3, s12), TBool

        elif op == "Cons":
            s1, th = self.infer(env, type_defs, subst, expr.left)
            s2, tt = self.infer(
                apply_subst_env(s1, env),
                type_defs,
                compose_subst(s1, subst),
                expr.right,
            )
            s12 = compose_subst(s2, s1)
            list_th = TList(apply_subst(s12, th))
            try:
                s3 = unify(apply_subst(s12, tt), list_th)
            except TypeError:
                raise TypeError(
                    f"cons (::) type mismatch: tail must be [{type_to_string(apply_subst(s12, th))}], "
                    f"got {type_to_string(apply_subst(s12, tt))}"
                )
            s_final = compose_subst(s3, s12)
            return s_final, apply_subst(s_final, list_th)

        raise TypeError(f"unknown operator: {op}")

    def _infer_let(self, env, type_defs, subst, expr):
        if expr.recursive:
            tv = self.fresh()
            env1 = {**env, expr.name: Scheme([], tv)}
            s1, t1 = self.infer(env1, type_defs, subst, expr.body)
            try:
                s2 = unify(apply_subst(s1, tv), apply_subst(s1, t1))
            except TypeError as e:
                raise TypeError(f"in recursive let {expr.name}: {e}")
            s12 = compose_subst(s2, s1)
            env2 = apply_subst_env(s12, env)
            gen = generalize(env2, apply_subst(s12, t1))
            env3 = {**env2, expr.name: gen}
            if expr.in_expr is not None:
                s3, t3 = self.infer(env3, type_defs, s12, expr.in_expr)
                return compose_subst(s3, s12), t3
            else:
                return s12, apply_subst(s12, gen.ty)
        else:
            s1, t1 = self.infer(env, type_defs, subst, expr.body)
            env1 = apply_subst_env(s1, env)
            gen = generalize(env1, apply_subst(s1, t1))
            env2 = {**env1, expr.name: gen}
            if expr.in_expr is not None:
                s2, t2 = self.infer(
                    env2, type_defs, compose_subst(s1, subst), expr.in_expr
                )
                return compose_subst(s2, s1), t2
            else:
                return s1, apply_subst(s1, gen.ty)

    def _infer_letrec_core(self, env, type_defs, subst, bindings):
        """
        Shared logic for mutually recursive let. Returns (subst, gen_env, new_env).
        gen_env maps each name to its generalized Scheme.
        new_env is env extended with all generalized bindings.
        """
        tvs = {name: self.fresh() for name, _ in bindings}
        env1 = {**env, **{name: Scheme([], tv) for name, tv in tvs.items()}}
        s = subst
        for name, body in bindings:
            s1, t1 = self.infer(env1, type_defs, s, body)
            try:
                s2 = unify(apply_subst(s1, tvs[name]), apply_subst(s1, t1))
            except TypeError as e:
                raise TypeError(f"in mutually recursive let {name}: {e}")
            s = compose_subst(s2, s1)
        env2 = apply_subst_env(s, env)
        gen_env = {}
        for name, tv in tvs.items():
            gen = generalize(env2, apply_subst(s, tv))
            gen_env[name] = gen
        new_env = {**env2, **gen_env}
        return s, gen_env, new_env

    def _infer_letrec(self, env, type_defs, subst, expr):
        s, gen_env, env3 = self._infer_letrec_core(env, type_defs, subst, expr.bindings)
        if expr.in_expr is not None:
            s2, t2 = self.infer(env3, type_defs, s, expr.in_expr)
            return compose_subst(s2, s), t2
        else:
            last_name = expr.bindings[-1][0]
            return s, apply_subst(s, gen_env[last_name].ty)

    @staticmethod
    def _check_exhaustive(arms, ctor_families):
        pats = [pat for pat, _ in arms]

        if any(isinstance(p, (ast.PWild, ast.PVar)) for p in pats):
            return

        if any(isinstance(p, ast.PBool) for p in pats):
            covered = {p.value for p in pats if isinstance(p, ast.PBool)}
            missing = []
            if True not in covered:
                missing.append("true")
            if False not in covered:
                missing.append("false")
            if missing:
                raise TypeError(
                    f"non-exhaustive patterns: missing {', '.join(missing)}"
                )
            return

        has_nil = any(isinstance(p, ast.PNil) for p in pats)
        has_cons = any(isinstance(p, ast.PCons) for p in pats)
        if has_nil or has_cons:
            missing = []
            if not has_nil:
                missing.append("[]")
            if not has_cons:
                missing.append("[h|t]")
            if missing:
                raise TypeError(
                    f"non-exhaustive patterns: missing {', '.join(missing)}"
                )
            return

        ctor_pats = [p for p in pats if isinstance(p, ast.PCtor)]
        if ctor_pats:
            first_name = ctor_pats[0].name
            if first_name in ctor_families:
                all_ctors = ctor_families[first_name]
                covered = {p.name for p in ctor_pats}
                missing = sorted(all_ctors - covered)
                if missing:
                    raise TypeError(
                        f"non-exhaustive patterns: missing {', '.join(missing)}"
                    )

    def _infer_match(self, env, type_defs, subst, expr):
        s0, ts = self.infer(env, type_defs, subst, expr.scrutinee)
        result_tv = self.fresh()
        s = s0
        for pat, body in expr.arms:
            env_s = apply_subst_env(s, env)
            s1, pat_ty, bindings = self.infer_pattern(pat, env_s, type_defs, s)
            s = compose_subst(s1, s)
            try:
                s2 = unify(apply_subst(s, ts), apply_subst(s, pat_ty))
            except TypeError:
                raise TypeError(
                    f"pattern type mismatch: scrutinee is {type_to_string(apply_subst(s, ts))}, "
                    f"pattern expects {type_to_string(apply_subst(s, pat_ty))}"
                )
            s = compose_subst(s2, s)
            applied_bindings = {
                k: apply_subst_scheme(s, v) for k, v in bindings.items()
            }
            body_env = {**apply_subst_env(s, env), **applied_bindings}
            s3, body_ty = self.infer(body_env, type_defs, s, body)
            s = compose_subst(s3, s)
            try:
                s4 = unify(apply_subst(s, result_tv), apply_subst(s, body_ty))
            except TypeError:
                raise TypeError(
                    f"match arms have different types: "
                    f"{type_to_string(apply_subst(s, result_tv))} vs "
                    f"{type_to_string(apply_subst(s, body_ty))}"
                )
            s = compose_subst(s4, s)
        self._check_exhaustive(expr.arms, env.get("__ctor_families__", {}))
        return s, apply_subst(s, result_tv)

    # -----------------------------------------------------------------------
    # Helper utilities
    # -----------------------------------------------------------------------

    def _decompose_fun(self, ty, n: int):
        """Unwrap n layers of TFun. Returns (arg_types, result_type)."""
        arg_tys = []
        for _ in range(n):
            ty = apply_subst({}, ty)  # normalize
            if isinstance(ty, TCon) and ty.name == "Fun":
                arg_tys.append(ty.args[0])
                ty = ty.args[1]
            else:
                raise TypeError("constructor applied to too many arguments")
        return arg_tys, ty

    def _resolve_type_sig(self, node, type_defs: dict):
        """Convert a type syntax AST node to a types.py type."""
        if isinstance(node, ast.TyName):
            name = node.name
            # Lowercase → TVar
            if name[0].islower():
                return TVar(name)
            # Primitives
            primitives = {
                "Int": TInt,
                "Float": TFloat,
                "String": TString,
                "Bool": TBool,
                "Unit": TUnit,
            }
            if name in primitives:
                return primitives[name]
            # Known type defs
            if name in type_defs:
                return type_defs[name]
            # Nullary ADT
            return TCon(name, [])
        elif isinstance(node, ast.TyFun):
            arg = self._resolve_type_sig(node.arg, type_defs)
            ret = self._resolve_type_sig(node.ret, type_defs)
            return TFun(arg, ret)
        elif isinstance(node, ast.TyList):
            elem = self._resolve_type_sig(node.elem, type_defs)
            return TList(elem)
        elif isinstance(node, ast.TyTuple):
            elems = [self._resolve_type_sig(e, type_defs) for e in node.elems]
            return TTuple(elems)
        elif isinstance(node, ast.TyUnit):
            return TUnit
        elif isinstance(node, ast.TyApp):
            args = [self._resolve_type_sig(a, type_defs) for a in node.args]
            return TCon(node.name, args)
        raise TypeError(f"unknown type syntax node: {type(node)}")

    def _resolve_type(self, name: str, type_defs: dict, param_env: dict = None):
        """Resolve a type name to a type. Raises TypeError if unknown."""
        if param_env and name in param_env:
            return param_env[name]
        primitives = {
            "int": TInt,
            "float": TFloat,
            "string": TString,
            "bool": TBool,
        }
        if name in primitives:
            return primitives[name]
        if name in type_defs:
            return type_defs[name]
        raise TypeError(f"unknown type: {name}")

    # -----------------------------------------------------------------------
    # Top-level inference
    # -----------------------------------------------------------------------

    def infer_toplevel(self, env: dict, type_defs: dict, subst: dict, expr):
        """
        Infer at top level, updating env and type_defs.
        Returns (subst, ty, env, type_defs).
        """
        if isinstance(expr, ast.TypeDecl):
            param_vars = [TVar(p) for p in expr.params]
            adt_ty = TCon(expr.name, param_vars)
            new_type_defs = {**type_defs, expr.name: adt_ty}
            param_env = {p: TVar(p) for p in expr.params}
            new_env = dict(env)
            for cname, arg_type_names in expr.ctors:
                arg_types = [
                    self._resolve_type(n, new_type_defs, param_env)
                    for n in arg_type_names
                ]
                if not arg_types:
                    ctor_ty = adt_ty
                else:
                    ctor_ty = adt_ty
                    for a in reversed(arg_types):
                        ctor_ty = TFun(a, ctor_ty)
                new_env[cname] = Scheme(expr.params, ctor_ty)
            ctor_families = dict(new_env.get("__ctor_families__", {}))
            all_ctors = frozenset(cname for cname, _ in expr.ctors)
            for cname, _ in expr.ctors:
                ctor_families[cname] = all_ctors
            new_env["__ctor_families__"] = ctor_families
            return subst, TUnit, new_env, new_type_defs

        elif isinstance(expr, ast.Let) and expr.in_expr is None:
            s, ty, new_env, _ = self._toplevel_let(env, type_defs, subst, expr)
            return s, ty, new_env, type_defs

        elif isinstance(expr, ast.LetRec) and expr.in_expr is None:
            s, gen_env, new_env = self._infer_letrec_core(
                env, type_defs, subst, expr.bindings
            )
            last_name = expr.bindings[-1][0]
            return s, apply_subst(s, gen_env[last_name].ty), new_env, type_defs

        elif isinstance(expr, ast.LetPat) and expr.in_expr is None:
            s1, t_body = self.infer(env, type_defs, subst, expr.body)
            env1 = apply_subst_env(s1, env)
            s2, pat_ty, bindings = self.infer_pattern(expr.pat, env1, type_defs, s1)
            s12 = compose_subst(s2, s1)
            try:
                s3 = unify(apply_subst(s12, t_body), apply_subst(s12, pat_ty))
            except TypeError as e:
                raise TypeError(f"in let pattern: {e}")
            s_final = compose_subst(s3, s12)
            applied_bindings = {
                k: apply_subst_scheme(s_final, v) for k, v in bindings.items()
            }
            new_env = {**apply_subst_env(s_final, env), **applied_bindings}
            return s_final, apply_subst(s_final, t_body), new_env, type_defs

        elif isinstance(expr, ast.Import):
            mod_result = check_module(expr.module)
            mod_env = mod_result["env"]
            mod_ctor_families = mod_result.get("ctor_families", {})
            mod_traits = mod_result.get("traits", {})
            mod_instances = mod_result.get("trait_instances", {})
            if expr.alias:
                modules = dict(env.get("__modules__", {}))
                modules[expr.alias] = mod_env
                ctor_families = {
                    **env.get("__ctor_families__", {}),
                    **mod_ctor_families,
                }
                traits = {**env.get("__traits__", {}), **mod_traits}
                instances = {**env.get("__trait_instances__", {}), **mod_instances}
                new_env = {
                    **env,
                    "__modules__": modules,
                    "__ctor_families__": ctor_families,
                    "__traits__": traits,
                    "__trait_instances__": instances,
                }
                return subst, TUnit, new_env, type_defs
            new_env = dict(env)
            ctor_families = dict(env.get("__ctor_families__", {}))
            traits = {**env.get("__traits__", {}), **mod_traits}
            instances = {**env.get("__trait_instances__", {}), **mod_instances}
            for name in expr.names:
                if name not in mod_env:
                    raise TypeError(
                        f"'{name}' is not exported by module '{expr.module}'"
                    )
                new_env[name] = mod_env[name]
                if name in mod_ctor_families:
                    ctor_families[name] = mod_ctor_families[name]
            new_env["__ctor_families__"] = ctor_families
            new_env["__traits__"] = traits
            new_env["__trait_instances__"] = instances
            return subst, TUnit, new_env, type_defs

        elif isinstance(expr, ast.Export):
            return subst, TUnit, env, type_defs

        elif isinstance(expr, ast.TraitDecl):
            traits = dict(env.get("__traits__", {}))
            methods_dict = {}
            new_env = dict(env)
            for mname, mtype_node in expr.methods:
                ty = self._resolve_type_sig(mtype_node, type_defs)
                # Collect free vars that match the trait param
                fv = free_vars(ty)
                # Quantify over the trait param
                qvars = [expr.param] if expr.param in fv else []
                scheme = Scheme(qvars, ty)
                methods_dict[mname] = scheme
                new_env[mname] = scheme
            traits[expr.name] = {"param": expr.param, "methods": methods_dict}
            new_env["__traits__"] = traits
            return subst, TUnit, new_env, type_defs

        elif isinstance(expr, ast.TestDecl):
            # Type-check the test body in an isolated env (bindings don't leak)
            test_env = dict(env)
            test_type_defs = dict(type_defs)
            for body_expr in expr.body:
                _, _, test_env, test_type_defs = self.infer_toplevel(
                    test_env, test_type_defs, {}, body_expr
                )
            return subst, TUnit, env, type_defs

        elif isinstance(expr, ast.ImplDecl):
            traits = env.get("__traits__", {})
            if expr.trait_name not in traits:
                raise TypeError(f"unknown trait: {expr.trait_name}")
            trait_info = traits[expr.trait_name]
            trait_param = trait_info["param"]
            trait_methods = trait_info["methods"]
            # Resolve target type
            target_ty = self._resolve_type_sig(ast.TyName(expr.target_type), type_defs)
            # Check all trait methods are implemented
            impl_names = {mname for mname, _ in expr.methods}
            trait_names = set(trait_methods.keys())
            missing = trait_names - impl_names
            if missing:
                raise TypeError(
                    f"impl {expr.trait_name} {expr.target_type} is missing: "
                    f"{', '.join(sorted(missing))}"
                )
            # Type-check each method body against expected type
            for mname, mbody in expr.methods:
                if mname not in trait_methods:
                    raise TypeError(
                        f"'{mname}' is not a method of trait {expr.trait_name}"
                    )
                trait_scheme = trait_methods[mname]
                # Substitute trait param with target type
                param_subst = {trait_param: target_ty}
                expected_ty = apply_subst(param_subst, trait_scheme.ty)
                # Type-check the impl body
                s1, actual_ty = self.infer(env, type_defs, subst, mbody)
                try:
                    s2 = unify(apply_subst(s1, actual_ty), apply_subst(s1, expected_ty))
                except TypeError as e:
                    raise TypeError(
                        f"in impl {expr.trait_name} {expr.target_type}, "
                        f"method '{mname}': {e}"
                    )
            # Register instance
            instances = dict(env.get("__trait_instances__", {}))
            instances[(expr.trait_name, expr.target_type)] = impl_names
            new_env = dict(env)
            new_env["__trait_instances__"] = instances
            return subst, TUnit, new_env, type_defs

        else:
            # Regular expression
            s, ty = self.infer(env, type_defs, subst, expr)
            return s, ty, env, type_defs

    def _toplevel_let(self, env, type_defs, subst, expr):
        """Handle a top-level Let (in_expr=None) binding."""
        if expr.recursive:
            tv = self.fresh()
            env1 = {**env, expr.name: Scheme([], tv)}
            s1, t1 = self.infer(env1, type_defs, subst, expr.body)
            try:
                s2 = unify(apply_subst(s1, tv), apply_subst(s1, t1))
            except TypeError as e:
                raise TypeError(f"in recursive let {expr.name}: {e}")
            s12 = compose_subst(s2, s1)
            env2 = apply_subst_env(s12, env)
            gen = generalize(env2, apply_subst(s12, t1))
            new_env = {**env2, expr.name: gen}
        else:
            s1, t1 = self.infer(env, type_defs, subst, expr.body)
            env1 = apply_subst_env(s1, env)
            gen = generalize(env1, apply_subst(s1, t1))
            new_env = {**env1, expr.name: gen}
            s12 = s1
        return s12, apply_subst(s12, gen.ty), new_env, type_defs


# ---------------------------------------------------------------------------
# Module loading
# ---------------------------------------------------------------------------


def _preregister_types(exprs: list, type_defs: dict) -> dict:
    """Pre-register all TypeDecl names so mutually recursive types can resolve each other."""
    for expr in exprs:
        if isinstance(expr, ast.TypeDecl):
            param_vars = [TVar(p) for p in expr.params]
            type_defs = {**type_defs, expr.name: TCon(expr.name, param_vars)}
    return type_defs


_module_cache: dict = {}


def check_module(module_name: str) -> dict:
    """Type-check a stdlib module; return dict with 'env' and 'ctor_families'."""
    if module_name in _module_cache:
        return _module_cache[module_name]

    if ":" in module_name:
        namespace, name = module_name.split(":", 1)
        if namespace != "std":
            raise TypeError(
                f"unknown module namespace: '{namespace}' (only 'std:' is supported)"
            )
        path = os.path.join(STDLIB_DIR, f"{name}.rex")
    else:
        raise TypeError(
            f"bare module name '{module_name}': use 'std:{module_name}' for stdlib"
        )

    try:
        with open(path) as f:
            source = f.read()
    except FileNotFoundError:
        raise TypeError(f"unknown module: {module_name}")

    exprs = parser_mod.parse(source)
    checker = TypeChecker()
    prelude = _load_prelude_tc()
    env = {**prelude["env"], **_type_env_for_module(name)}
    type_defs = _preregister_types(exprs, dict(prelude["type_defs"]))
    exports: set = set()

    for expr in exprs:
        if isinstance(expr, ast.Export):
            exports.update(expr.names)
        elif isinstance(expr, ast.TestDecl):
            pass  # skip test blocks in imported modules
        else:
            _, _, env, type_defs = checker.infer_toplevel(env, type_defs, {}, expr)

    exported_env = {name: env[name] for name in exports if name in env}
    result = {
        "env": exported_env,
        "ctor_families": env.get("__ctor_families__", {}),
        "traits": env.get("__traits__", {}),
        "trait_instances": env.get("__trait_instances__", {}),
    }
    _module_cache[module_name] = result
    return result


# ---------------------------------------------------------------------------
# Initial type environment (builtins)
# ---------------------------------------------------------------------------


def _math_type_env() -> dict:
    return {
        "toFloat": Scheme([], TFun(TInt, TFloat)),
        "round": Scheme([], TFun(TFloat, TInt)),
        "floor": Scheme([], TFun(TFloat, TInt)),
        "ceiling": Scheme([], TFun(TFloat, TInt)),
        "truncate": Scheme([], TFun(TFloat, TInt)),
        "sqrt": Scheme([], TFun(TFloat, TFloat)),
        "abs": Scheme(["a"], TFun(TVar("a"), TVar("a"))),
        "min": Scheme(["a"], TFun(TVar("a"), TFun(TVar("a"), TVar("a")))),
        "max": Scheme(["a"], TFun(TVar("a"), TFun(TVar("a"), TVar("a")))),
        "pow": Scheme([], TFun(TFloat, TFun(TFloat, TFloat))),
        "sin": Scheme([], TFun(TFloat, TFloat)),
        "cos": Scheme([], TFun(TFloat, TFloat)),
        "tan": Scheme([], TFun(TFloat, TFloat)),
        "asin": Scheme([], TFun(TFloat, TFloat)),
        "acos": Scheme([], TFun(TFloat, TFloat)),
        "atan": Scheme([], TFun(TFloat, TFloat)),
        "atan2": Scheme([], TFun(TFloat, TFun(TFloat, TFloat))),
        "log": Scheme([], TFun(TFloat, TFloat)),
        "exp": Scheme([], TFun(TFloat, TFloat)),
        "pi": Scheme([], TFloat),
        "e": Scheme([], TFloat),
    }


def _string_type_env() -> dict:
    return {
        "length": Scheme([], TFun(TString, TInt)),
        "toUpper": Scheme([], TFun(TString, TString)),
        "toLower": Scheme([], TFun(TString, TString)),
        "trim": Scheme([], TFun(TString, TString)),
        "split": Scheme([], TFun(TString, TFun(TString, TList(TString)))),
        "join": Scheme([], TFun(TString, TFun(TList(TString), TString))),
        "toString": Scheme(["a"], TFun(TVar("a"), TString)),
        "contains": Scheme([], TFun(TString, TFun(TString, TBool))),
        "startsWith": Scheme([], TFun(TString, TFun(TString, TBool))),
        "endsWith": Scheme([], TFun(TString, TFun(TString, TBool))),
        "charAt": Scheme([], TFun(TInt, TFun(TString, TMaybe(TString)))),
        "substring": Scheme([], TFun(TInt, TFun(TInt, TFun(TString, TString)))),
        "indexOf": Scheme([], TFun(TString, TFun(TString, TMaybe(TInt)))),
        "replace": Scheme([], TFun(TString, TFun(TString, TFun(TString, TString)))),
        "repeat": Scheme([], TFun(TInt, TFun(TString, TString))),
        "padLeft": Scheme([], TFun(TInt, TFun(TString, TFun(TString, TString)))),
        "padRight": Scheme([], TFun(TInt, TFun(TString, TFun(TString, TString)))),
        "words": Scheme([], TFun(TString, TList(TString))),
        "lines": Scheme([], TFun(TString, TList(TString))),
        "charCode": Scheme([], TFun(TString, TInt)),
        "fromCharCode": Scheme([], TFun(TInt, TString)),
        "parseInt": Scheme([], TFun(TString, TMaybe(TInt))),
        "parseFloat": Scheme([], TFun(TString, TMaybe(TFloat))),
    }


def _io_type_env() -> dict:
    return {
        "print": Scheme(["a"], TFun(TVar("a"), TVar("a"))),
        "println": Scheme(["a"], TFun(TVar("a"), TVar("a"))),
        "readLine": Scheme([], TFun(TString, TString)),
        "readFile": Scheme([], TFun(TString, TResult(TString, TString))),
        "writeFile": Scheme([], TFun(TString, TFun(TString, TResult(TUnit, TString)))),
        "appendFile": Scheme([], TFun(TString, TFun(TString, TResult(TUnit, TString)))),
        "fileExists": Scheme([], TFun(TString, TBool)),
        "listDir": Scheme([], TFun(TString, TResult(TList(TString), TString))),
    }


def _env_type_env() -> dict:
    return {
        "getEnv": Scheme([], TFun(TString, TMaybe(TString))),
        "getEnvOr": Scheme([], TFun(TString, TFun(TString, TString))),
        "args": Scheme([], TList(TString)),
    }


def _json_type_env() -> dict:
    TJson = TCon("Json", [])
    return {
        "jsonParse": Scheme([], TFun(TString, TResult(TJson, TString))),
    }


_MODULE_TYPE_ENVS = {
    "Math": _math_type_env,
    "String": _string_type_env,
    "IO": _io_type_env,
    "Env": _env_type_env,
    "Json": _json_type_env,
}


def _type_env_for_module(name: str) -> dict:
    """Core + domain type env for a specific stdlib module."""
    extra = _MODULE_TYPE_ENVS.get(name, lambda: {})
    return {**initial_type_env(), **extra()}


def initial_type_env() -> dict:
    """Return a type environment with only globally available builtins."""
    return {
        "not": Scheme([], TFun(TBool, TBool)),
        "error": Scheme(["a"], TFun(TString, TVar("a"))),
    }


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------


_prelude_tc_cache: dict | None = None


def _load_prelude_tc():
    """Type-check the Prelude and cache the result."""
    global _prelude_tc_cache
    if _prelude_tc_cache is not None:
        return _prelude_tc_cache
    path = os.path.join(STDLIB_DIR, "Prelude.rex")
    with open(path) as f:
        source = f.read()
    exprs = parser_mod.parse(source)
    checker = TypeChecker()
    env = initial_type_env()
    type_defs = _preregister_types(exprs, {})
    for expr in exprs:
        _, _, env, type_defs = checker.infer_toplevel(env, type_defs, {}, expr)
    _prelude_tc_cache = {"env": env, "type_defs": type_defs}
    return _prelude_tc_cache


def check_program(exprs: list) -> dict:
    """
    Type-check a list of top-level expressions.
    Returns the final type environment.
    Raises TypeError on type errors.
    """
    checker = TypeChecker()
    prelude = _load_prelude_tc()
    env = dict(prelude["env"])
    type_defs = _preregister_types(exprs, dict(prelude["type_defs"]))
    for expr in exprs:
        _, _, env, type_defs = checker.infer_toplevel(env, type_defs, {}, expr)
    return env
