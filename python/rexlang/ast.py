from dataclasses import dataclass
from typing import Any, Optional


# ---------------------------------------------------------------------------
# Patterns
# ---------------------------------------------------------------------------


@dataclass
class PWild:
    pass


@dataclass
class PUnit:
    pass


@dataclass
class PVar:
    name: str


@dataclass
class PInt:
    value: int


@dataclass
class PFloat:
    value: float


@dataclass
class PString:
    value: str


@dataclass
class PBool:
    value: bool


@dataclass
class PCtor:
    name: str
    args: list  # list of patterns


@dataclass
class PNil:
    pass


@dataclass
class PCons:
    head: Any
    tail: Any


@dataclass
class PTuple:
    pats: list  # list of patterns; len >= 2


# ---------------------------------------------------------------------------
# Binary operators
# ---------------------------------------------------------------------------

# Represented as plain strings: 'Add' 'Sub' 'Mul' 'Div' 'Mod'
#   'Eq' 'Lt' 'Gt' 'Leq' 'Geq' 'Neq' 'Concat' 'And' 'Or'

BINOP_SYM = {
    "Add": "+",
    "Sub": "-",
    "Mul": "*",
    "Div": "/",
    "Mod": "%",
    "Eq": "==",
    "Lt": "<",
    "Gt": ">",
    "Leq": "<=",
    "Geq": ">=",
    "Neq": "/=",
    "Concat": "++",
    "And": "&&",
    "Or": "||",
    "Cons": "::",
}


# ---------------------------------------------------------------------------
# Expressions
# ---------------------------------------------------------------------------


@dataclass
class Int:
    value: int


@dataclass
class Float:
    value: float


@dataclass
class String:
    value: str


@dataclass
class Bool:
    value: bool


@dataclass
class Unit:
    pass


@dataclass
class Var:
    name: str


@dataclass
class Unary_minus:
    expr: Any


@dataclass
class Binop:
    op: str
    left: Any
    right: Any


@dataclass
class If:
    cond: Any
    then_expr: Any
    else_expr: Any


@dataclass
class Let:
    name: str
    recursive: bool
    body: Any
    in_expr: Optional[Any]  # None = top-level binding


@dataclass
class Fun:
    param: str
    body: Any


@dataclass
class App:
    func: Any
    arg: Any


@dataclass
class Match:
    scrutinee: Any
    arms: list  # list of (pattern, expr) tuples


@dataclass
class TypeDecl:
    name: str
    params: list  # e.g. ['a'] for "type maybe a = ..."
    ctors: list  # list of (ctor_name: str, arg_type_names: list[str])


@dataclass
class ListLit:
    items: list  # list of Expr


@dataclass
class Tuple:
    items: list  # list of Expr; len >= 2


@dataclass
class LetPat:
    pat: Any  # PTuple (or any pattern in future)
    body: Any  # rhs expression
    in_expr: Optional[Any]  # None = top-level binding


@dataclass
class LetRec:
    bindings: list       # list of (name: str, body: Expr); len >= 2
    in_expr: Optional[Any]  # None = top-level


@dataclass
class Import:
    module: str  # e.g., "std:List"
    names: list  # list of str — names to bring into scope (empty when alias form)
    alias: str | None = None  # e.g. "L" from "import std:List as L"


@dataclass
class DotAccess:
    module_name: str  # e.g. "L"
    field_name: str  # e.g. "length"


@dataclass
class Export:
    names: list  # list of str — names this module makes public


# ---------------------------------------------------------------------------
# Pretty-printing
# ---------------------------------------------------------------------------


def pattern_to_string(pat) -> str:
    if isinstance(pat, PWild):
        return "_"
    elif isinstance(pat, PVar):
        return pat.name
    elif isinstance(pat, PInt):
        return str(pat.value)
    elif isinstance(pat, PFloat):
        return str(pat.value)
    elif isinstance(pat, PString):
        return repr(pat.value)
    elif isinstance(pat, PBool):
        return "true" if pat.value else "false"
    elif isinstance(pat, PCtor):
        if not pat.args:
            return pat.name
        args = " ".join(pattern_to_string(a) for a in pat.args)
        return f"({pat.name} {args})"
    elif isinstance(pat, PNil):
        return "[]"
    elif isinstance(pat, PCons):
        items = []
        p = pat
        while isinstance(p, PCons):
            items.append(pattern_to_string(p.head))
            p = p.tail
        if isinstance(p, PNil):
            return "[" + ", ".join(items) + "]"
        return "[" + ", ".join(items) + "|" + pattern_to_string(p) + "]"
    elif isinstance(pat, PTuple):
        return "(" + ", ".join(pattern_to_string(p) for p in pat.pats) + ")"
    return f"<unknown pattern {type(pat)}>"


def to_string(expr) -> str:
    if isinstance(expr, Int):
        return str(expr.value)
    elif isinstance(expr, Float):
        return str(expr.value)
    elif isinstance(expr, String):
        return repr(expr.value)
    elif isinstance(expr, Bool):
        return "true" if expr.value else "false"
    elif isinstance(expr, Var):
        return expr.name
    elif isinstance(expr, Unary_minus):
        return f"(-{to_string(expr.expr)})"
    elif isinstance(expr, Binop):
        sym = BINOP_SYM.get(expr.op, expr.op)
        return f"({to_string(expr.left)} {sym} {to_string(expr.right)})"
    elif isinstance(expr, If):
        return (
            f"(if {to_string(expr.cond)} "
            f"then {to_string(expr.then_expr)} "
            f"else {to_string(expr.else_expr)})"
        )
    elif isinstance(expr, Let):
        r = " rec" if expr.recursive else ""
        body = to_string(expr.body)
        if expr.in_expr is None:
            return f"(let{r} {expr.name} = {body})"
        return f"(let{r} {expr.name} = {body} in {to_string(expr.in_expr)})"
    elif isinstance(expr, Fun):
        return f"(fun {expr.param} -> {to_string(expr.body)})"
    elif isinstance(expr, App):
        return f"({to_string(expr.func)} {to_string(expr.arg)})"
    elif isinstance(expr, Match):
        arms = " ".join(
            f"| {pattern_to_string(p)} -> {to_string(e)}" for p, e in expr.arms
        )
        return f"(case {to_string(expr.scrutinee)} of {arms})"
    elif isinstance(expr, TypeDecl):
        def ctor_str(name, args):
            return name if not args else f"{name} of {' '.join(args)}"
        ctors = " | ".join(ctor_str(n, a) for n, a in expr.ctors)
        params_str = (" " + " ".join(expr.params)) if expr.params else ""
        return f"(type {expr.name}{params_str} = {ctors})"
    elif isinstance(expr, ListLit):
        return "[" + ", ".join(to_string(i) for i in expr.items) + "]"
    elif isinstance(expr, Tuple):
        return "(" + ", ".join(to_string(i) for i in expr.items) + ")"
    elif isinstance(expr, LetPat):
        pat_s = pattern_to_string(expr.pat)
        body_s = to_string(expr.body)
        if expr.in_expr is None:
            return f"(let {pat_s} = {body_s})"
        return f"(let {pat_s} = {body_s} in {to_string(expr.in_expr)})"
    elif isinstance(expr, LetRec):
        parts = " and ".join(f"{name} = {to_string(body)}" for name, body in expr.bindings)
        if expr.in_expr is None:
            return f"(let rec {parts})"
        return f"(let rec {parts} in {to_string(expr.in_expr)})"
    return f"<unknown expr {type(expr)}>"
