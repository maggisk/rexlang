from ..values import (
    VBool,
    VBuiltin,
    VFloat,
    VInt,
    VList,
    VString,
    Error,
    _check_str,
    value_to_string,
)


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


def builtins() -> dict:
    return {
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
    }
