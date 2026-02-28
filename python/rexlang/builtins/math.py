import math as _math

from ..values import (
    VBuiltin,
    VFloat,
    VInt,
    Error,
    _as_float,
    value_to_string,
)


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
        return VFloat(_math.pow(float(x.value), float(y.value)))

    return VBuiltin("pow$1", inner)


def _builtin_atan2(y):
    def inner(x):
        return VFloat(_math.atan2(_as_float(y), _as_float(x)))

    return VBuiltin("atan2$1", inner)


def builtins() -> dict:
    return {
        "abs": VBuiltin("abs", _builtin_abs),
        "min": VBuiltin("min", _builtin_min),
        "max": VBuiltin("max", _builtin_max),
        "pow": VBuiltin("pow", _builtin_pow),
        "sqrt": VBuiltin("sqrt", lambda v: VFloat(_math.sqrt(_as_float(v)))),
        "sin": VBuiltin("sin", lambda v: VFloat(_math.sin(_as_float(v)))),
        "cos": VBuiltin("cos", lambda v: VFloat(_math.cos(_as_float(v)))),
        "tan": VBuiltin("tan", lambda v: VFloat(_math.tan(_as_float(v)))),
        "asin": VBuiltin("asin", lambda v: VFloat(_math.asin(_as_float(v)))),
        "acos": VBuiltin("acos", lambda v: VFloat(_math.acos(_as_float(v)))),
        "atan": VBuiltin("atan", lambda v: VFloat(_math.atan(_as_float(v)))),
        "atan2": VBuiltin("atan2", _builtin_atan2),
        "log": VBuiltin("log", lambda v: VFloat(_math.log(_as_float(v)))),
        "exp": VBuiltin("exp", lambda v: VFloat(_math.exp(_as_float(v)))),
        "pi": VFloat(_math.pi),
        "e": VFloat(_math.e),
    }
