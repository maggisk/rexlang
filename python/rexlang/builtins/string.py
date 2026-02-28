from ..values import (
    VBool,
    VBuiltin,
    VCtor,
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


def _builtin_char_at(idx_v):
    if not isinstance(idx_v, VInt):
        raise Error("charAt: expected Int index")
    idx = idx_v.value

    def inner(str_v):
        s = _check_str("charAt", str_v)
        if 0 <= idx < len(s):
            return VCtor("Just", [VString(s[idx])])
        return VCtor("Nothing", [])

    return VBuiltin("charAt$1", inner)


def _builtin_substring(start_v):
    if not isinstance(start_v, VInt):
        raise Error("substring: expected Int start")
    start = start_v.value

    def inner1(end_v):
        if not isinstance(end_v, VInt):
            raise Error("substring: expected Int end")
        end = end_v.value

        def inner2(str_v):
            s = _check_str("substring", str_v)
            n = len(s)
            s_c = max(0, min(start, n))
            e_c = max(0, min(end, n))
            return VString(s[s_c:e_c])

        return VBuiltin("substring$2", inner2)

    return VBuiltin("substring$1", inner1)


def _builtin_index_of(needle_v):
    needle = _check_str("indexOf", needle_v)

    def inner(haystack_v):
        haystack = _check_str("indexOf", haystack_v)
        i = haystack.find(needle)
        if i == -1:
            return VCtor("Nothing", [])
        return VCtor("Just", [VInt(i)])

    return VBuiltin("indexOf$1", inner)


def _builtin_replace(find_v):
    find = _check_str("replace", find_v)

    def inner1(repl_v):
        repl = _check_str("replace", repl_v)

        def inner2(str_v):
            s = _check_str("replace", str_v)
            return VString(s.replace(find, repl))

        return VBuiltin("replace$2", inner2)

    return VBuiltin("replace$1", inner1)


def _builtin_str_repeat(n_v):
    if not isinstance(n_v, VInt):
        raise Error("repeat: expected Int")
    n = max(0, n_v.value)

    def inner(str_v):
        s = _check_str("repeat", str_v)
        return VString(s * n)

    return VBuiltin("repeat$1", inner)


def _builtin_pad_left(width_v):
    if not isinstance(width_v, VInt):
        raise Error("padLeft: expected Int width")
    width = width_v.value

    def inner1(pad_v):
        pad = _check_str("padLeft", pad_v)
        if len(pad) != 1:
            raise Error("padLeft: fill must be a single character")

        def inner2(str_v):
            s = _check_str("padLeft", str_v)
            return VString(s.rjust(width, pad))

        return VBuiltin("padLeft$2", inner2)

    return VBuiltin("padLeft$1", inner1)


def _builtin_pad_right(width_v):
    if not isinstance(width_v, VInt):
        raise Error("padRight: expected Int width")
    width = width_v.value

    def inner1(pad_v):
        pad = _check_str("padRight", pad_v)
        if len(pad) != 1:
            raise Error("padRight: fill must be a single character")

        def inner2(str_v):
            s = _check_str("padRight", str_v)
            return VString(s.ljust(width, pad))

        return VBuiltin("padRight$2", inner2)

    return VBuiltin("padRight$1", inner1)


def _builtin_words(v):
    s = _check_str("words", v)
    return VList([VString(w) for w in s.split()])


def _builtin_lines(v):
    s = _check_str("lines", v)
    return VList([VString(line) for line in s.splitlines()])


def _builtin_char_code(v):
    s = _check_str("charCode", v)
    if not s:
        raise Error("charCode: empty string")
    return VInt(ord(s[0]))


def _builtin_from_char_code(v):
    if not isinstance(v, VInt):
        raise Error("fromCharCode: expected Int")
    try:
        return VString(chr(v.value))
    except (ValueError, OverflowError):
        raise Error(f"fromCharCode: invalid code point {v.value}")


def _builtin_parse_int(v):
    s = _check_str("parseInt", v).strip()
    try:
        return VCtor("Just", [VInt(int(s))])
    except ValueError:
        return VCtor("Nothing", [])


def _builtin_parse_float(v):
    s = _check_str("parseFloat", v).strip()
    try:
        return VCtor("Just", [VFloat(float(s))])
    except ValueError:
        return VCtor("Nothing", [])


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
        "charAt": VBuiltin("charAt", _builtin_char_at),
        "substring": VBuiltin("substring", _builtin_substring),
        "indexOf": VBuiltin("indexOf", _builtin_index_of),
        "replace": VBuiltin("replace", _builtin_replace),
        "repeat": VBuiltin("repeat", _builtin_str_repeat),
        "padLeft": VBuiltin("padLeft", _builtin_pad_left),
        "padRight": VBuiltin("padRight", _builtin_pad_right),
        "words": VBuiltin("words", _builtin_words),
        "lines": VBuiltin("lines", _builtin_lines),
        "charCode": VBuiltin("charCode", _builtin_char_code),
        "fromCharCode": VBuiltin("fromCharCode", _builtin_from_char_code),
        "parseInt": VBuiltin("parseInt", _builtin_parse_int),
        "parseFloat": VBuiltin("parseFloat", _builtin_parse_float),
    }
