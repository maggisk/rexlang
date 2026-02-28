from ..values import (
    VBool,
    VBuiltin,
    Error,
    _as_bool,
    _check_str,
)


def _builtin_error(v):
    raise Error(_check_str("error", v))


def builtins() -> dict:
    return {
        "not": VBuiltin("not", lambda v: VBool(not _as_bool(v))),
        "error": VBuiltin("error", _builtin_error),
    }
