import sys
import os

# Allow running as  python bin/main.py  from the python/ directory
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from rexlang import ast as Ast
from rexlang import lexer as Lexer
from rexlang import parser as Parser
from rexlang import eval as Eval
from rexlang import typecheck as TypeCheck
from rexlang.types import type_to_string, TypeError as RexTypeError


def run_file(path: str):
    with open(path) as f:
        source = f.read()
    try:
        exprs = Parser.parse(source)
        TypeCheck.check_program(exprs)
        Eval.run_program(source)
    except Lexer.Error as e:
        print(f"Lexer error: {e}", file=sys.stderr)
        sys.exit(1)
    except Parser.Error as e:
        print(f"Parse error: {e}", file=sys.stderr)
        sys.exit(1)
    except RexTypeError as e:
        print(f"Type error: {e}", file=sys.stderr)
        sys.exit(1)
    except Eval.Error as e:
        print(f"Runtime error: {e}", file=sys.stderr)
        sys.exit(1)


def run_tests(path: str):
    with open(path) as f:
        source = f.read()
    # Inject module-specific builtins when testing a stdlib file directly
    extra_type_env = None
    extra_builtins = None
    stdlib_dir = os.path.normpath(
        os.path.join(os.path.dirname(__file__), "..", "rexlang", "stdlib")
    )
    norm_path = os.path.normpath(os.path.abspath(path))
    if norm_path.startswith(stdlib_dir) and norm_path.endswith(".rex"):
        module_name = os.path.basename(norm_path)[:-4]
        extra_type_env = TypeCheck._type_env_for_module(module_name)
        from rexlang.builtins import builtins_for_module

        extra_builtins = builtins_for_module(module_name)
    try:
        failures = Eval.run_tests(source, extra_type_env, extra_builtins)
        sys.exit(1 if failures else 0)
    except Lexer.Error as e:
        print(f"Lexer error: {e}", file=sys.stderr)
        sys.exit(1)
    except Parser.Error as e:
        print(f"Parse error: {e}", file=sys.stderr)
        sys.exit(1)
    except RexTypeError as e:
        print(f"Type error: {e}", file=sys.stderr)
        sys.exit(1)
    except Eval.Error as e:
        print(f"Runtime error: {e}", file=sys.stderr)
        sys.exit(1)


def repl():
    print("RexLang v0.1.0")
    print("Press Enter on a blank line to evaluate. Ctrl-D to exit.\n")
    prelude_tc = TypeCheck._load_prelude_tc()
    env = dict(Eval._load_prelude_eval())
    type_env = dict(prelude_tc["env"])
    type_defs = dict(prelude_tc["type_defs"])
    checker = TypeCheck.TypeChecker()
    buf = []
    try:
        while True:
            prompt = "rex> " if not buf else "  .. "
            try:
                line = input(prompt)
            except EOFError:
                print()
                return
            if line.strip():
                buf.append(line)
                continue
            if not buf:
                continue  # ignore leading blank lines
            source = "\n".join(buf) + "\n"
            buf.clear()
            try:
                exprs = Parser.parse(source)
            except Lexer.Error as e:
                print(f"Lexer error: {e}")
                continue
            except Parser.Error as e:
                print(f"Parse error: {e}")
                continue
            for expr in exprs:
                try:
                    _, ty, type_env, type_defs = checker.infer_toplevel(
                        type_env, type_defs, {}, expr
                    )
                except RexTypeError as e:
                    print(f"Type error: {e}")
                    break
                try:
                    value, env = Eval.eval_toplevel(env, expr)
                except Eval.Error as e:
                    print(f"Runtime error: {e}")
                    break

                # Display — skip TypeDecl/Import/Export/TraitDecl/ImplDecl/TestDecl (no interesting value)
                if isinstance(
                    expr,
                    (
                        Ast.TypeDecl,
                        Ast.Import,
                        Ast.Export,
                        Ast.TraitDecl,
                        Ast.ImplDecl,
                        Ast.TestDecl,
                    ),
                ):
                    continue

                ty_str = type_to_string(ty)
                if isinstance(expr, Ast.Let):
                    name = expr.name
                elif isinstance(expr, Ast.LetRec):
                    name = expr.bindings[-1][0]
                elif isinstance(expr, Ast.LetPat):
                    name = "_"
                else:
                    name = "it"
                print(f"{name} : {ty_str}")
                print(f"=> {Eval.value_to_string(value)}")
    except KeyboardInterrupt:
        print()


if __name__ == "__main__":
    if len(sys.argv) < 2:
        repl()
    elif sys.argv[1] == "--test":
        if len(sys.argv) < 3:
            print("Usage: rexlang --test <file.rex>", file=sys.stderr)
            sys.exit(1)
        run_tests(sys.argv[2])
    else:
        run_file(sys.argv[1])
