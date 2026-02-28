from .core import builtins as _core_builtins
from .math import builtins as _math_builtins
from .string import builtins as _string_builtins
from .io import builtins as _io_builtins
from .env import builtins as _env_builtins

_MODULE_BUILTINS = {
    "Math": _math_builtins,
    "String": _string_builtins,
    "IO": _io_builtins,
    "Env": _env_builtins,
}


def core_builtins() -> dict:
    """Only error + not — for user programs."""
    return _core_builtins()


def builtins_for_module(name: str) -> dict:
    """Core + domain builtins for a specific stdlib module."""
    extra = _MODULE_BUILTINS.get(name, lambda: {})
    return {**_core_builtins(), **extra()}
