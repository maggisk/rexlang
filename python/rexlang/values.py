from dataclasses import dataclass
from typing import Any


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
    env: dict  # exported name -> value


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


def _check_str(name, v):
    if not isinstance(v, VString):
        raise Error(f"{name}: expected string, got {value_to_string(v)}")
    return v.value


def _display(v) -> str:
    """Human-readable: strings without quotes, everything else via value_to_string."""
    if isinstance(v, VString):
        return v.value
    return value_to_string(v)
