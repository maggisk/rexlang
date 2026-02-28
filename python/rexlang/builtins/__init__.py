from .core import builtins as core_builtins
from .math import builtins as math_builtins
from .string import builtins as string_builtins
from .io import builtins as io_builtins
from .env import builtins as env_builtins


def all_builtins() -> dict:
    return {
        **core_builtins(),
        **math_builtins(),
        **string_builtins(),
        **io_builtins(),
        **env_builtins(),
    }
