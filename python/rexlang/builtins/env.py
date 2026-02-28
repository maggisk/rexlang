import os
import sys

from ..values import (
    VBuiltin,
    VCtor,
    VList,
    VString,
    _check_str,
)


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


def builtins() -> dict:
    return {
        "getEnv": VBuiltin("getEnv", _builtin_get_env),
        "getEnvOr": VBuiltin("getEnvOr", _builtin_get_env_or),
        "args": VList([VString(a) for a in sys.argv[1:]]),
    }
