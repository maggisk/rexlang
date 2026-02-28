import math

from ..values import (
    VBool,
    VBuiltin,
    VFloat,
    VInt,
    VString,
    Error,
    _as_bool,
    _as_float,
    _as_int,
    _check_str,
    _display,
)


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


def builtins() -> dict:
    return {
        "not": VBuiltin("not", lambda v: VBool(not _as_bool(v))),
        "toFloat": VBuiltin("toFloat", lambda v: VFloat(float(_as_int(v)))),
        "round": VBuiltin("round", lambda v: VInt(round(_as_float(v)))),
        "floor": VBuiltin("floor", lambda v: VInt(math.floor(_as_float(v)))),
        "ceiling": VBuiltin("ceiling", lambda v: VInt(math.ceil(_as_float(v)))),
        "truncate": VBuiltin("truncate", lambda v: VInt(int(_as_float(v)))),
        "error": VBuiltin("error", _builtin_error),
        "print": VBuiltin("print", _builtin_print),
        "println": VBuiltin("println", _builtin_println),
        "readLine": VBuiltin("readLine", _builtin_readline),
    }
