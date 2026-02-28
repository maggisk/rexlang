from .core import builtins as _core_builtins
from .math import builtins as _math_builtins
from .string import builtins as _string_builtins
from .io import builtins as _io_builtins
from .env import builtins as _env_builtins


def core_builtins() -> dict:
    """Only error + not — for user programs."""
    return _core_builtins()


def all_builtins() -> dict:
    """Full set — for stdlib module loading."""
    return {
        **_core_builtins(),
        **_math_builtins(),
        **_string_builtins(),
        **_io_builtins(),
        **_env_builtins(),
    }
