import os
from typing import Any, Optional

from . import ast
from . import parser
from .builtins import all_builtins
from .values import (
    VInt,
    VFloat,
    VString,
    VBool,
    VClosure,
    VCtor,
    VCtorFn,
    VList,
    VTuple,
    VUnit,
    VBuiltin,
    VModule,
    Error,
    value_to_string,
    _as_bool,
)

STDLIB_DIR = os.path.join(os.path.dirname(__file__), "stdlib")


def _eval_binop(op: str, l, r):
    if op == "Add":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VInt(l.value + r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VFloat(l.value + r.value)
    elif op == "Sub":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VInt(l.value - r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VFloat(l.value - r.value)
    elif op == "Mul":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VInt(l.value * r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VFloat(l.value * r.value)
    elif op == "Div":
        if isinstance(l, VInt) and isinstance(r, VInt):
            if r.value == 0:
                raise Error("division by zero")
            return VInt(l.value // r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VFloat(l.value / r.value)
    elif op == "Mod":
        if isinstance(l, VInt) and isinstance(r, VInt):
            if r.value == 0:
                raise Error("modulo by zero")
            return VInt(l.value % r.value)
    elif op == "Concat":
        if isinstance(l, VString) and isinstance(r, VString):
            return VString(l.value + r.value)
    elif op == "Lt":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VBool(l.value < r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VBool(l.value < r.value)
    elif op == "Gt":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VBool(l.value > r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VBool(l.value > r.value)
    elif op == "Leq":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VBool(l.value <= r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VBool(l.value <= r.value)
    elif op == "Geq":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VBool(l.value >= r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VBool(l.value >= r.value)
    elif op == "Eq":
        if type(l) == type(r) and isinstance(l, (VInt, VFloat, VString, VBool)):
            return VBool(l.value == r.value)
    elif op == "Neq":
        if type(l) == type(r) and isinstance(l, (VInt, VFloat, VString, VBool)):
            return VBool(l.value != r.value)
    elif op == "And":
        if isinstance(l, VBool) and isinstance(r, VBool):
            return VBool(l.value and r.value)
    elif op == "Or":
        if isinstance(l, VBool) and isinstance(r, VBool):
            return VBool(l.value or r.value)
    elif op == "Cons":
        if isinstance(r, VList):
            return VList([l] + r.items)
    sym = ast.BINOP_SYM.get(op, op)
    raise Error(f"type error: {value_to_string(l)} {sym} {value_to_string(r)}")


def _match_pattern(pat, value) -> Optional[dict]:
    """Return a binding dict if pat matches value, else None."""
    if isinstance(pat, ast.PWild):
        return {}
    elif isinstance(pat, ast.PUnit):
        return {} if isinstance(value, VUnit) else None
    elif isinstance(pat, ast.PVar):
        return {pat.name: value}
    elif isinstance(pat, ast.PInt):
        return {} if isinstance(value, VInt) and pat.value == value.value else None
    elif isinstance(pat, ast.PFloat):
        return {} if isinstance(value, VFloat) and pat.value == value.value else None
    elif isinstance(pat, ast.PString):
        return {} if isinstance(value, VString) and pat.value == value.value else None
    elif isinstance(pat, ast.PBool):
        return {} if isinstance(value, VBool) and pat.value == value.value else None
    elif isinstance(pat, ast.PCtor):
        if (
            isinstance(value, VCtor)
            and pat.name == value.name
            and len(pat.args) == len(value.args)
        ):
            bindings = {}
            for p, v in zip(pat.args, value.args):
                b = _match_pattern(p, v)
                if b is None:
                    return None
                bindings.update(b)
            return bindings
    elif isinstance(pat, ast.PNil):
        return {} if isinstance(value, VList) and len(value.items) == 0 else None
    elif isinstance(pat, ast.PCons):
        if isinstance(value, VList) and len(value.items) > 0:
            hb = _match_pattern(pat.head, value.items[0])
            if hb is None:
                return None
            tb = _match_pattern(pat.tail, VList(value.items[1:]))
            if tb is None:
                return None
            return {**hb, **tb}
        return None
    elif isinstance(pat, ast.PTuple):
        if isinstance(value, VTuple) and len(pat.pats) == len(value.items):
            bindings = {}
            for p, v in zip(pat.pats, value.items):
                b = _match_pattern(p, v)
                if b is None:
                    return None
                bindings.update(b)
            return bindings
        return None
    return None


def _check_exhaustive(arms, type_registry):
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
            raise Error(f"non-exhaustive patterns: missing {', '.join(missing)}")
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
            raise Error(f"non-exhaustive patterns: missing {', '.join(missing)}")
        return

    ctor_pats = [p for p in pats if isinstance(p, ast.PCtor)]
    if ctor_pats:
        first_name = ctor_pats[0].name
        if first_name in type_registry:
            all_ctors = type_registry[first_name]
            covered = {p.name for p in ctor_pats}
            missing = sorted(all_ctors - covered)
            if missing:
                raise Error(f"non-exhaustive patterns: missing {', '.join(missing)}")


# ---------------------------------------------------------------------------
# Evaluator
# ---------------------------------------------------------------------------


def eval(env: dict, expr) -> Any:
    """Evaluate expr in env, with a trampoline loop for tail calls."""
    while True:
        if isinstance(expr, ast.Int):
            return VInt(expr.value)
        elif isinstance(expr, ast.Float):
            return VFloat(expr.value)
        elif isinstance(expr, ast.String):
            return VString(expr.value)
        elif isinstance(expr, ast.Bool):
            return VBool(expr.value)

        elif isinstance(expr, ast.Unit):
            return VUnit()

        elif isinstance(expr, ast.Var):
            if expr.name in env:
                return env[expr.name]
            raise Error(f"unbound variable: {expr.name}")

        elif isinstance(expr, ast.Unary_minus):
            v = eval(env, expr.expr)
            if isinstance(v, VInt):
                return VInt(-v.value)
            if isinstance(v, VFloat):
                return VFloat(-v.value)
            raise Error(f"type error: unary minus on {value_to_string(v)}")

        elif isinstance(expr, ast.Binop):
            return _eval_binop(expr.op, eval(env, expr.left), eval(env, expr.right))

        elif isinstance(expr, ast.If):
            # tail call: continue loop with the chosen branch
            expr = expr.then_expr if _as_bool(eval(env, expr.cond)) else expr.else_expr

        elif isinstance(expr, ast.Fun):
            return VClosure(expr.param, expr.body, env)

        elif isinstance(expr, ast.App):
            func = eval(env, expr.func)
            arg = eval(env, expr.arg)
            if isinstance(func, VClosure):
                # tail call: extend closure's env and continue
                env = {**func.env, func.param: arg}
                expr = func.body
            elif isinstance(func, VCtorFn):
                if func.remaining == 1:
                    return VCtor(func.name, list(reversed([arg] + func.acc_args)))
                return VCtorFn(func.name, func.remaining - 1, [arg] + func.acc_args)
            elif isinstance(func, VBuiltin):
                return func.fn(arg)
            else:
                raise Error(f"cannot apply {value_to_string(func)} as a function")

        elif isinstance(expr, ast.Match):
            value = eval(env, expr.scrutinee)
            _check_exhaustive(expr.arms, env.get("__types__", {}))
            for pat, body in expr.arms:
                bindings = _match_pattern(pat, value)
                if bindings is not None:
                    # tail call: extend env with bindings and continue
                    env = {**env, **bindings}
                    expr = body
                    break
            else:
                raise Error("match failure: no pattern matched")

        elif isinstance(expr, ast.ListLit):
            return VList([eval(env, item) for item in expr.items])

        elif isinstance(expr, ast.Tuple):
            return VTuple([eval(env, item) for item in expr.items])

        elif isinstance(expr, ast.LetPat):
            value = eval(env, expr.body)
            bindings = _match_pattern(expr.pat, value)
            if bindings is None:
                raise Error("let pattern match failure")
            if expr.in_expr is not None:
                env = {**env, **bindings}
                expr = expr.in_expr
            else:
                return value  # top-level: fallthrough to eval_toplevel

        elif isinstance(expr, ast.DotAccess):
            val = env.get(expr.module_name)
            if not isinstance(val, VModule):
                raise Error(f"'{expr.module_name}' is not a module")
            if expr.field_name not in val.env:
                raise Error(
                    f"module '{expr.module_name}' does not export '{expr.field_name}'"
                )
            return val.env[expr.field_name]

        elif isinstance(expr, ast.TypeDecl):
            return VBool(False)  # handled properly in eval_toplevel

        elif isinstance(expr, ast.Let):
            if expr.recursive:
                body_val = eval(env, expr.body)
                if isinstance(body_val, VClosure):
                    fixed_env = dict(body_val.env)
                    closure = VClosure(body_val.param, body_val.body, fixed_env)
                    fixed_env[expr.name] = closure
                    value = closure
                else:
                    value = body_val
            else:
                value = eval(env, expr.body)

            if expr.in_expr is not None:
                # tail call: bind name and continue in in_expr
                env = {**env, expr.name: value}
                expr = expr.in_expr
            else:
                return value

        elif isinstance(expr, ast.LetRec):
            shared_env = dict(env)
            raw = {}
            for bname, bbody in expr.bindings:
                val = eval(env, bbody)
                raw[bname] = (
                    VClosure(val.param, val.body, shared_env)
                    if isinstance(val, VClosure)
                    else val
                )
            shared_env.update(raw)
            last_value = raw[expr.bindings[-1][0]]
            if expr.in_expr is not None:
                env = shared_env
                expr = expr.in_expr
            else:
                return last_value

        else:
            raise Error(f"unknown AST node: {type(expr)}")


def _load_module(module_name: str) -> tuple:
    """Evaluate a module file; return (env, exported_names)."""
    if ":" in module_name:
        namespace, name = module_name.split(":", 1)
        if namespace != "std":
            raise Error(
                f"unknown module namespace: '{namespace}' (only 'std:' is supported)"
            )
        path = os.path.join(STDLIB_DIR, f"{name}.rex")
    else:
        raise Error(
            f"bare module name '{module_name}': use 'std:{module_name}' for stdlib"
        )
    try:
        with open(path) as f:
            source = f.read()
    except FileNotFoundError:
        raise Error(f"unknown module: {module_name}")
    exprs = parser.parse(source)
    env = initial_env()
    exports = set()
    for expr in exprs:
        if isinstance(expr, ast.Export):
            exports.update(expr.names)
        else:
            _, env = eval_toplevel(env, expr)
    undefined = exports - env.keys()
    if undefined:
        names = ", ".join(sorted(undefined))
        raise Error(f"module '{module_name}' exports undefined name(s): {names}")
    return env, exports


def eval_toplevel(env: dict, expr) -> tuple:
    if isinstance(expr, ast.TypeDecl):
        ctor_env = {
            cname: (
                VCtor(cname, [])
                if len(arg_types) == 0
                else VCtorFn(cname, len(arg_types), [])
            )
            for cname, arg_types in expr.ctors
        }
        types = dict(env.get("__types__", {}))
        all_ctors = frozenset(cname for cname, _ in expr.ctors)
        for cname, _ in expr.ctors:
            types[cname] = all_ctors
        return VBool(False), {**env, **ctor_env, "__types__": types}

    elif isinstance(expr, ast.Let) and expr.in_expr is None:
        value = eval(env, expr)
        return value, {**env, expr.name: value}

    elif isinstance(expr, ast.LetPat) and expr.in_expr is None:
        value = eval(env, expr.body)
        bindings = _match_pattern(expr.pat, value)
        if bindings is None:
            raise Error("let pattern match failure")
        return value, {**env, **bindings}

    elif isinstance(expr, ast.Import):
        module_env, module_exports = _load_module(expr.module)
        module_types = module_env.get("__types__", {})
        if expr.alias:
            mod_bindings = {n: module_env[n] for n in module_exports if n in module_env}
            types = {**env.get("__types__", {}), **module_types}
            return VBool(False), {
                **env,
                expr.alias: VModule(expr.alias, mod_bindings),
                "__types__": types,
            }
        new_bindings = {}
        types = dict(env.get("__types__", {}))
        imported_ctor = False
        for name in expr.names:
            if name not in module_exports:
                raise Error(f"'{name}' is not exported by module '{expr.module}'")
            new_bindings[name] = module_env[name]
            if name in module_types:
                types[name] = module_types[name]
                imported_ctor = True
        if imported_ctor:
            new_bindings["__types__"] = types
        return VBool(False), {**env, **new_bindings}

    elif isinstance(expr, ast.Export):
        return VBool(False), env  # no-op outside module loading context

    elif isinstance(expr, ast.LetRec) and expr.in_expr is None:
        shared_env = dict(env)
        raw = {}
        for bname, bbody in expr.bindings:
            val = eval(env, bbody)
            raw[bname] = (
                VClosure(val.param, val.body, shared_env)
                if isinstance(val, VClosure)
                else val
            )
        shared_env.update(raw)
        last_value = raw[expr.bindings[-1][0]]
        return last_value, {**env, **raw}

    else:
        return eval(env, expr), env


def initial_env() -> dict:
    return all_builtins()


def run_program(source: str):
    exprs = parser.parse(source)
    env = initial_env()
    last = VBool(False)
    for expr in exprs:
        last, env = eval_toplevel(env, expr)
    return last


def run(source: str):
    return eval(initial_env(), parser.parse_single(source))
