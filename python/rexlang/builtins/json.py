import json as _json

from ..values import VBool, VBuiltin, VCtor, VFloat, VInt, VString, _check_str, Error


def _python_to_json_val(v):
    """Recursively convert a Python JSON value to Rex Json ADT VCtor values."""
    if v is None:
        return VCtor("JNull", [])
    if isinstance(v, bool):
        return VCtor("JBool", [VBool(v)])
    if isinstance(v, (int, float)):
        return VCtor("JNum", [VFloat(float(v))])
    if isinstance(v, str):
        return VCtor("JStr", [VString(v)])
    if isinstance(v, list):
        arr = VCtor("ArrNil", [])
        for item in reversed(v):
            arr = VCtor("ArrCons", [_python_to_json_val(item), arr])
        return VCtor("JArr", [arr])
    if isinstance(v, dict):
        obj = VCtor("ObjNil", [])
        for k, val in reversed(list(v.items())):
            obj = VCtor("ObjCons", [VString(k), _python_to_json_val(val), obj])
        return VCtor("JObj", [obj])
    raise Error(f"jsonParse: unexpected Python type {type(v)}")


def _builtin_json_parse(v):
    s = _check_str("jsonParse", v)
    try:
        py_val = _json.loads(s)
        return VCtor("Ok", [_python_to_json_val(py_val)])
    except _json.JSONDecodeError as e:
        return VCtor("Err", [VString(str(e))])


def builtins() -> dict:
    return {
        "jsonParse": VBuiltin("jsonParse", _builtin_json_parse),
    }
