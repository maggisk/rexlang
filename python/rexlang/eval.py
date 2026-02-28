import os
from typing import Any, Optional

from . import ast
from . import parser
from .builtins import builtins_for_module, core_builtins
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
    VTraitMethod,
    Error,
    value_to_string,
    _as_bool,
)

STDLIB_DIR = os.path.join(os.path.dirname(__file__), "stdlib")


def _runtime_type_name(value):
    """Map a runtime value to its type name for trait dispatch."""
    if isinstance(value, VInt):
        return "Int"
    elif isinstance(value, VFloat):
        return "Float"
    elif isinstance(value, VString):
        return "String"
    elif isinstance(value, VBool):
        return "Bool"
    raise Error(f"no trait dispatch for {value_to_string(value)}")


def _structural_eq(l, r) -> bool:
    """Structural equality for any Rex value."""
    if type(l) is not type(r):
        return False
    if isinstance(l, (VInt, VFloat, VString, VBool)):
        return l.value == r.value
    if isinstance(l, VUnit):
        return True
    if isinstance(l, (VList, VTuple)):
        return len(l.items) == len(r.items) and all(
            _structural_eq(a, b) for a, b in zip(l.items, r.items)
        )
    if isinstance(l, VCtor):
        return (
            l.name == r.name
            and len(l.args) == len(r.args)
            and all(_structural_eq(a, b) for a, b in zip(l.args, r.args))
        )
    return False


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
        if isinstance(l, VString) and isinstance(r, VString):
            return VBool(l.value < r.value)
        if isinstance(l, VBool) and isinstance(r, VBool):
            return VBool(l.value < r.value)
    elif op == "Gt":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VBool(l.value > r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VBool(l.value > r.value)
        if isinstance(l, VString) and isinstance(r, VString):
            return VBool(l.value > r.value)
        if isinstance(l, VBool) and isinstance(r, VBool):
            return VBool(l.value > r.value)
    elif op == "Leq":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VBool(l.value <= r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VBool(l.value <= r.value)
        if isinstance(l, VString) and isinstance(r, VString):
            return VBool(l.value <= r.value)
        if isinstance(l, VBool) and isinstance(r, VBool):
            return VBool(l.value <= r.value)
    elif op == "Geq":
        if isinstance(l, VInt) and isinstance(r, VInt):
            return VBool(l.value >= r.value)
        if isinstance(l, VFloat) and isinstance(r, VFloat):
            return VBool(l.value >= r.value)
        if isinstance(l, VString) and isinstance(r, VString):
            return VBool(l.value >= r.value)
        if isinstance(l, VBool) and isinstance(r, VBool):
            return VBool(l.value >= r.value)
    elif op == "Eq":
        return VBool(_structural_eq(l, r))
    elif op == "Neq":
        return VBool(not _structural_eq(l, r))
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
            elif isinstance(func, VTraitMethod):
                type_name = _runtime_type_name(arg)
                instances = env.get("__instances__", {})
                key = (func.trait_name, type_name, func.method_name)
                impl_fn = instances.get(key)
                if impl_fn is None:
                    raise Error(f"no {func.trait_name} instance for {type_name}")
                if isinstance(impl_fn, VClosure):
                    env = {**impl_fn.env, impl_fn.param: arg}
                    expr = impl_fn.body
                elif isinstance(impl_fn, VBuiltin):
                    return impl_fn.fn(arg)
                else:
                    raise Error(f"invalid trait impl for {key}")
            else:
                raise Error(f"cannot apply {value_to_string(func)} as a function")

        elif isinstance(expr, ast.Match):
            value = eval(env, expr.scrutinee)
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

        elif isinstance(expr, ast.Assert):
            v = eval(env, expr.expr)
            if isinstance(v, VBool) and v.value:
                return VUnit()
            raise Error(f"assert failed at line {expr.line}")

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
    env = {**_load_prelude_eval(), **builtins_for_module(name)}
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
        return VBool(False), {**env, **ctor_env}

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
        if expr.alias:
            mod_bindings = {n: module_env[n] for n in module_exports if n in module_env}
            return VBool(False), {
                **env,
                expr.alias: VModule(expr.alias, mod_bindings),
            }
        new_bindings = {}
        for name in expr.names:
            if name not in module_exports:
                raise Error(f"'{name}' is not exported by module '{expr.module}'")
            new_bindings[name] = module_env[name]
        return VBool(False), {**env, **new_bindings}

    elif isinstance(expr, ast.Export):
        return VBool(False), env  # no-op outside module loading context

    elif isinstance(expr, ast.TraitDecl):
        new_env = dict(env)
        for mname, _mtype in expr.methods:
            new_env[mname] = VTraitMethod(expr.name, mname)
        return VBool(False), new_env

    elif isinstance(expr, ast.ImplDecl):
        new_env = dict(env)
        instances = dict(env.get("__instances__", {}))
        for mname, mbody in expr.methods:
            impl_val = eval(env, mbody)
            instances[(expr.trait_name, expr.target_type, mname)] = impl_val
        new_env["__instances__"] = instances
        return VBool(False), new_env

    elif isinstance(expr, ast.TestDecl):
        # In normal mode, test blocks are skipped
        return VUnit(), env

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
    return core_builtins()


_prelude_eval_cache: dict | None = None


def _load_prelude_eval():
    """Evaluate the Prelude and cache the resulting env."""
    global _prelude_eval_cache
    if _prelude_eval_cache is not None:
        return _prelude_eval_cache
    path = os.path.join(STDLIB_DIR, "Prelude.rex")
    with open(path) as f:
        source = f.read()
    exprs = parser.parse(source)
    env = initial_env()
    for expr in exprs:
        _, env = eval_toplevel(env, expr)
    _prelude_eval_cache = env
    return _prelude_eval_cache


def run_program(source: str):
    exprs = parser.parse(source)
    env = dict(_load_prelude_eval())
    last = VBool(False)
    for expr in exprs:
        last, env = eval_toplevel(env, expr)
    return last


def run_tests(
    source: str, _extra_type_env: dict = None, _extra_builtins: dict = None
) -> int:
    """Run test blocks in source. Returns number of failures."""
    from . import typecheck

    exprs = parser.parse(source)

    if _extra_type_env or _extra_builtins:
        # Module context: use module-specific type env + builtins
        checker = typecheck.TypeChecker()
        prelude = typecheck._load_prelude_tc()
        tc_env = {**prelude["env"], **(_extra_type_env or {})}
        type_defs = typecheck._preregister_types(exprs, dict(prelude["type_defs"]))
        for expr in exprs:
            _, _, tc_env, type_defs = checker.infer_toplevel(
                tc_env, type_defs, {}, expr
            )
    else:
        typecheck.check_program(exprs)

    env = {**dict(_load_prelude_eval()), **(_extra_builtins or {})}
    tests = []
    for expr in exprs:
        if isinstance(expr, ast.TestDecl):
            tests.append(expr)
        else:
            _, env = eval_toplevel(env, expr)
    passed = 0
    failed = 0
    for test in tests:
        try:
            test_env = dict(env)
            for body_expr in test.body:
                _, test_env = eval_toplevel(test_env, body_expr)
            passed += 1
            print(f"PASS  {test.name}")
        except Error as e:
            failed += 1
            print(f"FAIL  {test.name}")
            print(f"  {e}")
    print(f"\n{passed} passed, {failed} failed")
    return failed


def run(source: str):
    return eval(initial_env(), parser.parse_single(source))
