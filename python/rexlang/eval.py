import math
import os
import sys
from dataclasses import dataclass
from typing import Any, Optional

from . import ast
from . import parser

STDLIB_DIR = os.path.join(os.path.dirname(__file__), "stdlib")


# ---------------------------------------------------------------------------
# Values
# ---------------------------------------------------------------------------


@dataclass
class VInt:
    value: int


@dataclass
class VFloat:
    value: float


@dataclass
class VString:
    value: str


@dataclass
class VBool:
    value: bool


@dataclass
class VClosure:
    param: str
    body: Any
    env: dict


@dataclass
class VCtor:
    name: str
    args: list


@dataclass
class VCtorFn:
    """Partially-applied constructor waiting for more arguments."""

    name: str
    remaining: int
    acc_args: list  # built in reverse; reversed when arity is satisfied


@dataclass
class VList:
    items: list


@dataclass
class VTuple:
    items: list


@dataclass
class VUnit:
    pass


@dataclass
class VBuiltin:
    name: str
    fn: Any


@dataclass
class VModule:
    name: str
    env: dict  # exported name → value


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


class Error(Exception):
    pass


def _escape_string(s: str) -> str:
    out = ['"']
    for c in s:
        if c == '"':
            out.append('\\"')
        elif c == "\\":
            out.append("\\\\")
        elif c == "\n":
            out.append("\\n")
        elif c == "\t":
            out.append("\\t")
        else:
            out.append(c)
    out.append('"')
    return "".join(out)


def value_to_string(v) -> str:
    if isinstance(v, VInt):
        return str(v.value)
    elif isinstance(v, VFloat):
        return str(v.value)
    elif isinstance(v, VString):
        return _escape_string(v.value)
    elif isinstance(v, VBool):
        return "true" if v.value else "false"
    elif isinstance(v, VClosure):
        return "<fun>"
    elif isinstance(v, VBuiltin):
        return f"<builtin {v.name}>"
    elif isinstance(v, VCtor):
        if not v.args:
            return v.name
        return f"({v.name} {' '.join(value_to_string(a) for a in v.args)})"
    elif isinstance(v, VCtorFn):
        return f"<ctor {v.name}>"
    elif isinstance(v, VList):
        return "[" + ", ".join(value_to_string(i) for i in v.items) + "]"
    elif isinstance(v, VTuple):
        return "(" + ", ".join(value_to_string(i) for i in v.items) + ")"
    elif isinstance(v, VUnit):
        return "()"
    elif isinstance(v, VModule):
        return f"<module {v.name}>"
    raise Error(f"unknown value type: {type(v)}")


def _as_int(v) -> int:
    if isinstance(v, VInt):
        return v.value
    raise Error(f"expected int, got {value_to_string(v)}")


def _as_float(v) -> float:
    if isinstance(v, VFloat):
        return v.value
    raise Error(f"expected float, got {value_to_string(v)}")


def _as_bool(v) -> bool:
    if isinstance(v, VBool):
        return v.value
    raise Error(f"expected bool, got {value_to_string(v)}")


# ---------------------------------------------------------------------------
# Math builtin helpers
# ---------------------------------------------------------------------------


def _builtin_abs(v):
    if isinstance(v, VInt):
        return VInt(abs(v.value))
    if isinstance(v, VFloat):
        return VFloat(abs(v.value))
    raise Error(f"abs: expected number, got {value_to_string(v)}")


def _builtin_min(x):
    def inner(y):
        if isinstance(x, VInt) and isinstance(y, VInt):
            return x if x.value <= y.value else y
        if isinstance(x, VFloat) and isinstance(y, VFloat):
            return x if x.value <= y.value else y
        raise Error("min: expected matching numeric types")

    return VBuiltin("min$1", inner)


def _builtin_max(x):
    def inner(y):
        if isinstance(x, VInt) and isinstance(y, VInt):
            return x if x.value >= y.value else y
        if isinstance(x, VFloat) and isinstance(y, VFloat):
            return x if x.value >= y.value else y
        raise Error("max: expected matching numeric types")

    return VBuiltin("max$1", inner)


def _builtin_pow(x):
    if not isinstance(x, (VInt, VFloat)):
        raise Error("pow: expected number")

    def inner(y):
        if not isinstance(y, (VInt, VFloat)):
            raise Error("pow: expected number")
        return VFloat(math.pow(float(x.value), float(y.value)))

    return VBuiltin("pow$1", inner)


def _builtin_atan2(y):
    def inner(x):
        return VFloat(math.atan2(_as_float(y), _as_float(x)))

    return VBuiltin("atan2$1", inner)


# ---------------------------------------------------------------------------
# String builtin helpers
# ---------------------------------------------------------------------------


def _check_str(name, v):
    if not isinstance(v, VString):
        raise Error(f"{name}: expected string, got {value_to_string(v)}")
    return v.value


def _builtin_str_length(v):
    return VInt(len(_check_str("length", v)))


def _builtin_to_upper(v):
    return VString(_check_str("toUpper", v).upper())


def _builtin_to_lower(v):
    return VString(_check_str("toLower", v).lower())


def _builtin_trim(v):
    return VString(_check_str("trim", v).strip())


def _builtin_split(sep):
    s = _check_str("split", sep)

    def inner(v):
        return VList([VString(p) for p in _check_str("split", v).split(s)])

    return VBuiltin("split$1", inner)


def _builtin_join(sep):
    s = _check_str("join", sep)

    def inner(lst):
        if not isinstance(lst, VList):
            raise Error("join: expected list")
        parts = [_check_str("join", item) for item in lst.items]
        return VString(s.join(parts))

    return VBuiltin("join$1", inner)


def _builtin_to_string(v):
    if isinstance(v, VInt):
        return VString(str(v.value))
    if isinstance(v, VFloat):
        return VString(str(v.value))
    if isinstance(v, VBool):
        return VString("true" if v.value else "false")
    if isinstance(v, VString):
        return v
    raise Error(f"toString: cannot convert {value_to_string(v)}")


def _builtin_contains(sub):
    s = _check_str("contains", sub)

    def inner(v):
        return VBool(s in _check_str("contains", v))

    return VBuiltin("contains$1", inner)


def _builtin_starts_with(prefix):
    s = _check_str("startsWith", prefix)

    def inner(v):
        return VBool(_check_str("startsWith", v).startswith(s))

    return VBuiltin("startsWith$1", inner)


def _builtin_ends_with(suffix):
    s = _check_str("endsWith", suffix)

    def inner(v):
        return VBool(_check_str("endsWith", v).endswith(s))

    return VBuiltin("endsWith$1", inner)


# ---------------------------------------------------------------------------
# I/O builtin helpers
# ---------------------------------------------------------------------------


def _display(v) -> str:
    """Human-readable: strings without quotes, everything else via value_to_string."""
    if isinstance(v, VString):
        return v.value
    return value_to_string(v)


def _builtin_print(v):
    print(_display(v), end="")
    return v


def _builtin_println(v):
    print(_display(v))
    return v


def _builtin_error(v):
    raise Error(_check_str("error", v))


def _builtin_readline(v):
    prompt = _check_str("readLine", v)
    try:
        return VString(input(prompt))
    except EOFError:
        return VString("")


# ---------------------------------------------------------------------------
# IO builtin helpers
# ---------------------------------------------------------------------------


def _builtin_read_file(v):
    path = _check_str("readFile", v)
    try:
        with open(path) as f:
            return VCtor("Ok", [VString(f.read())])
    except FileNotFoundError:
        return VCtor("Err", [VString(f"file not found: {path}")])
    except OSError as e:
        return VCtor("Err", [VString(str(e))])


def _builtin_write_file(path_v):
    path = _check_str("writeFile", path_v)

    def inner(content_v):
        content = _check_str("writeFile", content_v)
        try:
            with open(path, "w") as f:
                f.write(content)
            return VCtor("Ok", [VUnit()])
        except OSError as e:
            return VCtor("Err", [VString(str(e))])

    return VBuiltin("writeFile$1", inner)


def _builtin_append_file(path_v):
    path = _check_str("appendFile", path_v)

    def inner(content_v):
        content = _check_str("appendFile", content_v)
        try:
            with open(path, "a") as f:
                f.write(content)
            return VCtor("Ok", [VUnit()])
        except OSError as e:
            return VCtor("Err", [VString(str(e))])

    return VBuiltin("appendFile$1", inner)


def _builtin_file_exists(v):
    path = _check_str("fileExists", v)
    return VBool(os.path.exists(path))


def _builtin_list_dir(v):
    path = _check_str("listDir", v)
    try:
        return VCtor("Ok", [VList([VString(e) for e in sorted(os.listdir(path))])])
    except OSError as e:
        return VCtor("Err", [VString(str(e))])


# ---------------------------------------------------------------------------
# Env builtin helpers
# ---------------------------------------------------------------------------


def _builtin_get_env(v):
    name = _check_str("getEnv", v)
    val = os.environ.get(name)
    if val is None:
        return VCtor("Nothing", [])
    return VCtor("Just", [VString(val)])


def _builtin_get_env_or(name_v):
    name = _check_str("getEnvOr", name_v)

    def inner(default_v):
        default = _check_str("getEnvOr", default_v)
        return VString(os.environ.get(name, default))

    return VBuiltin("getEnvOr$1", inner)


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
                raw[bname] = VClosure(val.param, val.body, shared_env) if isinstance(val, VClosure) else val
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
            cname: (VCtor(cname, []) if len(arg_types) == 0 else VCtorFn(cname, len(arg_types), []))
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
            raw[bname] = VClosure(val.param, val.body, shared_env) if isinstance(val, VClosure) else val
        shared_env.update(raw)
        last_value = raw[expr.bindings[-1][0]]
        return last_value, {**env, **raw}

    else:
        return eval(env, expr), env


def initial_env() -> dict:
    return {
        "not": VBuiltin("not", lambda v: VBool(not _as_bool(v))),
        "toFloat": VBuiltin("toFloat", lambda v: VFloat(float(_as_int(v)))),
        "round": VBuiltin("round", lambda v: VInt(round(_as_float(v)))),
        "floor": VBuiltin("floor", lambda v: VInt(math.floor(_as_float(v)))),
        "ceiling": VBuiltin("ceiling", lambda v: VInt(math.ceil(_as_float(v)))),
        "truncate": VBuiltin("truncate", lambda v: VInt(int(_as_float(v)))),
        "sqrt": VBuiltin("sqrt", lambda v: VFloat(math.sqrt(_as_float(v)))),
        # Math builtins
        "abs": VBuiltin("abs", _builtin_abs),
        "min": VBuiltin("min", _builtin_min),
        "max": VBuiltin("max", _builtin_max),
        "pow": VBuiltin("pow", _builtin_pow),
        "sin": VBuiltin("sin", lambda v: VFloat(math.sin(_as_float(v)))),
        "cos": VBuiltin("cos", lambda v: VFloat(math.cos(_as_float(v)))),
        "tan": VBuiltin("tan", lambda v: VFloat(math.tan(_as_float(v)))),
        "asin": VBuiltin("asin", lambda v: VFloat(math.asin(_as_float(v)))),
        "acos": VBuiltin("acos", lambda v: VFloat(math.acos(_as_float(v)))),
        "atan": VBuiltin("atan", lambda v: VFloat(math.atan(_as_float(v)))),
        "atan2": VBuiltin("atan2", _builtin_atan2),
        "log": VBuiltin("log", lambda v: VFloat(math.log(_as_float(v)))),
        "exp": VBuiltin("exp", lambda v: VFloat(math.exp(_as_float(v)))),
        "pi": VFloat(math.pi),
        "e": VFloat(math.e),
        # String builtins
        "length": VBuiltin("length", _builtin_str_length),
        "toUpper": VBuiltin("toUpper", _builtin_to_upper),
        "toLower": VBuiltin("toLower", _builtin_to_lower),
        "trim": VBuiltin("trim", _builtin_trim),
        "split": VBuiltin("split", _builtin_split),
        "join": VBuiltin("join", _builtin_join),
        "toString": VBuiltin("toString", _builtin_to_string),
        "contains": VBuiltin("contains", _builtin_contains),
        "startsWith": VBuiltin("startsWith", _builtin_starts_with),
        "endsWith": VBuiltin("endsWith", _builtin_ends_with),
        # Error
        "error": VBuiltin("error", _builtin_error),
        # I/O builtins
        "print": VBuiltin("print", _builtin_print),
        "println": VBuiltin("println", _builtin_println),
        "readLine": VBuiltin("readLine", _builtin_readline),
        # Filesystem (std:IO)
        "readFile": VBuiltin("readFile", _builtin_read_file),
        "writeFile": VBuiltin("writeFile", _builtin_write_file),
        "appendFile": VBuiltin("appendFile", _builtin_append_file),
        "fileExists": VBuiltin("fileExists", _builtin_file_exists),
        "listDir": VBuiltin("listDir", _builtin_list_dir),
        # Environment (std:Env)
        "getEnv": VBuiltin("getEnv", _builtin_get_env),
        "getEnvOr": VBuiltin("getEnvOr", _builtin_get_env_or),
        "args": VList([VString(a) for a in sys.argv[1:]]),
    }


def run_program(source: str):
    exprs = parser.parse(source)
    env = initial_env()
    last = VBool(False)
    for expr in exprs:
        last, env = eval_toplevel(env, expr)
    return last


def run(source: str):
    return eval(initial_env(), parser.parse_single(source))
