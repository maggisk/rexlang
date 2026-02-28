import os

from ..values import (
    VBool,
    VBuiltin,
    VCtor,
    VList,
    VString,
    VUnit,
    _check_str,
    _display,
)


def _builtin_print(v):
    print(_display(v), end="")
    return v


def _builtin_println(v):
    print(_display(v))
    return v


def _builtin_readline(v):
    prompt = _check_str("readLine", v)
    try:
        return VString(input(prompt))
    except EOFError:
        return VString("")


def _builtin_read_file(v):
    path = _check_str("readFile", v)
    try:
        with open(path) as f:
            return VCtor("Ok", [VString(f.read())])
    except FileNotFoundError:
        return VCtor("Err", [VString(f"file not found: {path}")])
    except OSError as e:
        return VCtor("Err", [VString(str(e))])


def _builtin_write_file(path_v):
    path = _check_str("writeFile", path_v)

    def inner(content_v):
        content = _check_str("writeFile", content_v)
        try:
            with open(path, "w") as f:
                f.write(content)
            return VCtor("Ok", [VUnit()])
        except OSError as e:
            return VCtor("Err", [VString(str(e))])

    return VBuiltin("writeFile$1", inner)


def _builtin_append_file(path_v):
    path = _check_str("appendFile", path_v)

    def inner(content_v):
        content = _check_str("appendFile", content_v)
        try:
            with open(path, "a") as f:
                f.write(content)
            return VCtor("Ok", [VUnit()])
        except OSError as e:
            return VCtor("Err", [VString(str(e))])

    return VBuiltin("appendFile$1", inner)


def _builtin_file_exists(v):
    path = _check_str("fileExists", v)
    return VBool(os.path.exists(path))


def _builtin_list_dir(v):
    path = _check_str("listDir", v)
    try:
        return VCtor("Ok", [VList([VString(e) for e in sorted(os.listdir(path))])])
    except OSError as e:
        return VCtor("Err", [VString(str(e))])


def builtins() -> dict:
    return {
        "readFile": VBuiltin("readFile", _builtin_read_file),
        "writeFile": VBuiltin("writeFile", _builtin_write_file),
        "appendFile": VBuiltin("appendFile", _builtin_append_file),
        "fileExists": VBuiltin("fileExists", _builtin_file_exists),
        "listDir": VBuiltin("listDir", _builtin_list_dir),
        "print": VBuiltin("print", _builtin_print),
        "println": VBuiltin("println", _builtin_println),
        "readLine": VBuiltin("readLine", _builtin_readline),
    }
