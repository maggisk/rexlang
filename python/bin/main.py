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


def repl():
    print("RexLang v0.1.0")
    print("Press Enter on a blank line to evaluate. Ctrl-D to exit.\n")
    env = Eval.initial_env()
    type_env = TypeCheck.initial_type_env()
    type_defs: dict = {}
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

                # Display — skip TypeDecl/Import/Export (no interesting value)
                if isinstance(expr, (Ast.TypeDecl, Ast.Import, Ast.Export)):
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
    else:
        run_file(sys.argv[1])
