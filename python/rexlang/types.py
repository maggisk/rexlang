# Type representation for Hindley-Milner inference.

from dataclasses import dataclass, field
from typing import Any


class TypeError(Exception):
    """Type error raised during HM inference."""

    pass


@dataclass
class TVar:
    name: str

    def __eq__(self, other):
        return isinstance(other, TVar) and self.name == other.name

    def __hash__(self):
        return hash(self.name)


@dataclass
class TCon:
    name: str
    args: list = field(default_factory=list)

    def __eq__(self, other):
        return (
            isinstance(other, TCon)
            and self.name == other.name
            and self.args == other.args
        )

    def __hash__(self):
        return hash((self.name, tuple(self.args)))


@dataclass
class Scheme:
    vars: list  # list of str — quantified type variable names
    ty: Any


# ---------------------------------------------------------------------------
# Primitive type singletons
# ---------------------------------------------------------------------------

TInt = TCon("Int")
TFloat = TCon("Float")
TString = TCon("String")
TBool = TCon("Bool")
TUnit = TCon("Unit")


# ---------------------------------------------------------------------------
# Type constructors (functions that build TCon instances)
# ---------------------------------------------------------------------------


def TFun(a, b):
    return TCon("Fun", [a, b])


def TList(a):
    return TCon("List", [a])


def TTuple(ts):
    return TCon("Tuple", list(ts))


# ---------------------------------------------------------------------------
# Substitution operations
# ---------------------------------------------------------------------------


def free_vars(ty) -> set:
    """Collect all free type variable names in ty."""
    if isinstance(ty, TVar):
        return {ty.name}
    elif isinstance(ty, TCon):
        result = set()
        for arg in ty.args:
            result |= free_vars(arg)
        return result
    return set()


def free_vars_scheme(scheme: Scheme) -> set:
    """Free vars in a scheme minus the quantified ones."""
    return free_vars(scheme.ty) - set(scheme.vars)


def apply_subst(subst: dict, ty):
    """Apply a substitution to a type."""
    if isinstance(ty, TVar):
        if ty.name in subst:
            # Follow chains
            resolved = subst[ty.name]
            if resolved == ty:
                return ty
            return apply_subst(subst, resolved)
        return ty
    elif isinstance(ty, TCon):
        new_args = [apply_subst(subst, a) for a in ty.args]
        if new_args == ty.args:
            return ty
        return TCon(ty.name, new_args)
    return ty


def apply_subst_scheme(subst: dict, scheme: Scheme) -> Scheme:
    """Apply subst to a scheme, skipping bound variables."""
    restricted = {k: v for k, v in subst.items() if k not in scheme.vars}
    return Scheme(scheme.vars, apply_subst(restricted, scheme.ty))


def apply_subst_env(subst: dict, env: dict) -> dict:
    """Apply subst to every scheme in env; non-Scheme values are passed through."""
    return {
        k: apply_subst_scheme(subst, v) if isinstance(v, Scheme) else v
        for k, v in env.items()
    }


def compose_subst(s1: dict, s2: dict) -> dict:
    """Compose substitutions: result = s1 ∘ s2 (s2 applied first)."""
    result = {k: apply_subst(s1, v) for k, v in s2.items()}
    for k, v in s1.items():
        if k not in result:
            result[k] = v
    return result


def _occurs(var_name: str, ty) -> bool:
    """Check if var_name occurs in ty (occurs check for unification)."""
    if isinstance(ty, TVar):
        return ty.name == var_name
    elif isinstance(ty, TCon):
        return any(_occurs(var_name, a) for a in ty.args)
    return False


def unify(t1, t2) -> dict:
    """Unify two types, returning a substitution. Raises TypeError on failure."""
    if isinstance(t1, TVar):
        if isinstance(t2, TVar) and t1.name == t2.name:
            return {}
        if _occurs(t1.name, t2):
            raise TypeError(
                f"infinite type: {type_to_string(t1)} occurs in {type_to_string(t2)}"
            )
        return {t1.name: t2}
    elif isinstance(t2, TVar):
        return unify(t2, t1)
    elif isinstance(t1, TCon) and isinstance(t2, TCon):
        if t1.name != t2.name or len(t1.args) != len(t2.args):
            raise TypeError(
                f"type mismatch: {type_to_string(t1)} vs {type_to_string(t2)}"
            )
        subst = {}
        for a1, a2 in zip(t1.args, t2.args):
            s = unify(apply_subst(subst, a1), apply_subst(subst, a2))
            subst = compose_subst(s, subst)
        return subst
    raise TypeError(f"cannot unify {type_to_string(t1)} with {type_to_string(t2)}")


def generalize(env: dict, ty) -> Scheme:
    """Generalize ty over type variables not free in env."""
    env_free = set()
    for scheme in env.values():
        env_free |= free_vars_scheme(scheme)
    quantified = sorted(free_vars(ty) - env_free)
    return Scheme(quantified, ty)


# ---------------------------------------------------------------------------
# Pretty-printing
# ---------------------------------------------------------------------------


def type_to_string(ty) -> str:
    """Pretty-print a type, renaming TVars to a, b, c... in order of appearance."""
    mapping: dict = {}
    counter = [0]

    def name_for(var_name: str) -> str:
        if var_name not in mapping:
            idx = counter[0]
            counter[0] += 1
            mapping[var_name] = chr(ord("a") + idx) if idx < 26 else f"t{idx}"
        return mapping[var_name]

    def render(ty, in_fun_arg: bool = False) -> str:
        if isinstance(ty, TVar):
            return name_for(ty.name)
        elif isinstance(ty, TCon):
            if ty.name == "Fun":
                a, b = ty.args
                result = f"{render(a, in_fun_arg=True)} -> {render(b)}"
                if in_fun_arg:
                    return f"({result})"
                return result
            elif ty.name == "List" and len(ty.args) == 1:
                return f"[{render(ty.args[0])}]"
            elif ty.name == "Tuple":
                return "(" + ", ".join(render(a) for a in ty.args) + ")"
            elif not ty.args:
                return ty.name
            else:
                args_s = " ".join(render(a) for a in ty.args)
                return f"({ty.name} {args_s})"
        return str(ty)

    return render(ty)
