# Recursive descent parser for RexLang.
#
# Grammar (highest to lowest precedence):
#   atom       := INT | FLOAT | STRING | BOOL | IDENT | "(" expr ")"
#   app        := atom { atom }                        (left-assoc)
#   unary      := "-" unary | app
#   mult       := unary { ("*" | "/" | "%") unary }     (left-assoc)
#   add        := mult  { ("++" | "+" | "-") mult }    (left-assoc)
#   cons       := add [ "::" cons ]                    (right-assoc)
#   compare    := cons [ ("<"|">"|"<="|">="|"=="|"/=") cons ]   non-assoc
#   logic_and  := compare { "&&" compare }             (left-assoc)
#   logic_or   := logic_and { "||" logic_and }         (left-assoc)
#   pipe       := logic_or { "|>" logic_or }           (left-assoc; x |> f => App(f, x))
#   expr       := let_expr | if_expr | fun_expr | case_expr | type_decl | pipe
#   let_expr   := "let" ["rec"] IDENT { IDENT } "=" expr ["in" expr]
#   if_expr    := "if" expr "then" expr "else" expr
#   fun_expr   := "fun" IDENT { IDENT } "->" expr
#   case_expr  := "case" expr "of" ["|"] pattern "->" expr { "|" pattern "->" expr }
#   type_decl  := "type" UPPER_IDENT { LOWER_IDENT } "=" ["|"] UPPER_IDENT { IDENT } { "|" UPPER_IDENT { IDENT } }

from . import ast
from .lexer import tokenize


class Error(Exception):
    pass


def _is_uppercase(s: str) -> bool:
    return bool(s) and s[0].isupper()


def parse_tokens(tokens: list) -> list:
    pos = 0
    case_arm_col = -1  # column of current case arms (-1 = unrestricted)

    def peek():
        return tokens[pos] if pos < len(tokens) else tokens[-1]  # last is always Eof

    def advance():
        nonlocal pos
        pos += 1

    def expect(kind):
        tok = peek()
        if tok.kind == kind:
            advance()
        else:
            raise Error(
                f"expected '{kind}', got '{tok}' at line {tok.line}, col {tok.col + 1}"
            )

    def expect_ident():
        tok = peek()
        if tok.kind == "ident":
            advance()
            return tok.value
        raise Error(
            f"expected identifier, got '{tok}' at line {tok.line}, col {tok.col + 1}"
        )

    # --- atom ---
    def parse_atom():
        tok = peek()
        if tok.kind == "int":
            advance()
            return ast.Int(tok.value)
        elif tok.kind == "float":
            advance()
            return ast.Float(tok.value)
        elif tok.kind == "string":
            advance()
            return ast.String(tok.value)
        elif tok.kind == "bool":
            advance()
            return ast.Bool(tok.value)
        elif tok.kind == "ident":
            advance()
            if peek().kind == ".":
                advance()
                field = expect_ident()
                return ast.DotAccess(tok.value, field)
            return ast.Var(tok.value)
        elif tok.kind == "(":
            advance()
            if peek().kind == ")":
                advance()
                return ast.Unit()
            first = parse_expr()
            if peek().kind == ",":
                items = [first]
                while peek().kind == ",":
                    advance()
                    items.append(parse_expr())
                expect(")")
                return ast.Tuple(items)
            expect(")")
            return first
        elif tok.kind == "[":
            advance()
            if peek().kind == "]":
                advance()
                return ast.ListLit([])
            items = [parse_expr()]
            while peek().kind == ",":
                advance()
                items.append(parse_expr())
            expect("]")
            return ast.ListLit(items)
        else:
            raise Error(
                f"unexpected token: '{tok}' at line {tok.line}, col {tok.col + 1}"
            )

    # --- application: f x y  =>  App(App(f, x), y) ---
    def parse_app():
        f = parse_atom()
        while peek().kind in ("int", "float", "string", "bool", "ident", "(", "["):
            if case_arm_col >= 0 and peek().col <= case_arm_col:
                break
            f = ast.App(f, parse_atom())
        return f

    # --- unary minus ---
    def parse_unary():
        if peek().kind == "-":
            advance()
            return ast.Unary_minus(parse_unary())
        return parse_app()

    # --- multiplicative ---
    def parse_mult():
        lhs = parse_unary()
        while True:
            k = peek().kind
            if k == "*":
                advance()
                lhs = ast.Binop("Mul", lhs, parse_unary())
            elif k == "/":
                advance()
                lhs = ast.Binop("Div", lhs, parse_unary())
            elif k == "%":
                advance()
                lhs = ast.Binop("Mod", lhs, parse_unary())
            else:
                break
        return lhs

    # --- additive ---
    def parse_add():
        lhs = parse_mult()
        while True:
            k = peek().kind
            if k == "++":
                advance()
                lhs = ast.Binop("Concat", lhs, parse_mult())
            elif k == "+":
                advance()
                lhs = ast.Binop("Add", lhs, parse_mult())
            elif k == "-":
                advance()
                lhs = ast.Binop("Sub", lhs, parse_mult())
            else:
                break
        return lhs

    # --- cons: right-associative ---
    def parse_cons():
        lhs = parse_add()
        if peek().kind == "::":
            advance()
            return ast.Binop("Cons", lhs, parse_cons())
        return lhs

    # --- comparison (single, non-associative) ---
    def parse_compare():
        lhs = parse_cons()
        op_map = {
            "<": "Lt",
            ">": "Gt",
            "<=": "Leq",
            ">=": "Geq",
            "==": "Eq",
            "/=": "Neq",
        }
        k = peek().kind
        if k in op_map:
            advance()
            return ast.Binop(op_map[k], lhs, parse_cons())
        return lhs

    # --- logical and ---
    def parse_logic_and():
        lhs = parse_compare()
        while peek().kind == "&&":
            advance()
            lhs = ast.Binop("And", lhs, parse_compare())
        return lhs

    # --- logical or ---
    def parse_logic_or():
        lhs = parse_logic_and()
        while peek().kind == "||":
            advance()
            lhs = ast.Binop("Or", lhs, parse_logic_and())
        return lhs

    # --- pipe: x |> f  =>  App(f, x) ---
    def parse_pipe():
        lhs = parse_logic_or()
        while peek().kind == "|>":
            advance()
            rhs = parse_logic_or()
            lhs = ast.App(rhs, lhs)
        return lhs

    # --- let expression ---
    def parse_let():
        advance()  # consume 'let'
        if peek().kind == "(":
            pat = parse_atom_pattern()
            expect("=")
            body = parse_expr()
            in_expr = None
            if peek().kind == "in":
                advance()
                in_expr = parse_expr()
            return ast.LetPat(pat, body, in_expr)
        recursive = peek().kind == "rec"
        if recursive:
            advance()
        name = expect_ident()
        params = []
        while peek().kind == "ident":
            params.append(expect_ident())
        expect("=")
        body = parse_expr()
        # let f x y = body  =>  let f = fun x -> fun y -> body
        for param in reversed(params):
            body = ast.Fun(param, body)
        # mutual recursion: let rec f ... = ... and g ... = ...
        if recursive and peek().kind == "and":
            bindings = [(name, body)]
            while peek().kind == "and":
                advance()  # consume 'and'
                name2 = expect_ident()
                params2 = []
                while peek().kind == "ident":
                    params2.append(expect_ident())
                expect("=")
                body2 = parse_expr()
                for param in reversed(params2):
                    body2 = ast.Fun(param, body2)
                bindings.append((name2, body2))
            in_expr = None
            if peek().kind == "in":
                advance()
                in_expr = parse_expr()
            return ast.LetRec(bindings, in_expr)
        in_expr = None
        if peek().kind == "in":
            advance()
            in_expr = parse_expr()
        return ast.Let(name, recursive, body, in_expr)

    # --- if expression ---
    def parse_if():
        advance()  # consume 'if'
        cond = parse_expr()
        expect("then")
        then_expr = parse_expr()
        expect("else")
        else_expr = parse_expr()
        return ast.If(cond, then_expr, else_expr)

    # --- fun expression: fun x y -> body  =>  Fun(x, Fun(y, body)) ---
    def parse_fun():
        advance()  # consume 'fun'
        params = []
        while peek().kind != "->":
            if peek().kind == "ident":
                params.append(expect_ident())
            else:
                tok = peek()
                raise Error(
                    f"expected parameter or '->', got '{tok}' at line {tok.line}, col {tok.col + 1}"
                )
        arrow = peek()
        advance()  # consume '->'
        if not params:
            raise Error(
                f"fun requires at least one parameter at line {arrow.line}, col {arrow.col + 1}"
            )
        body = parse_expr()
        for param in reversed(params):
            body = ast.Fun(param, body)
        return body

    # --- atom pattern ---
    def parse_atom_pattern():
        tok = peek()
        if tok.kind == "ident" and tok.value == "_":
            advance()
            return ast.PWild()
        elif tok.kind == "ident" and not _is_uppercase(tok.value):
            advance()
            return ast.PVar(tok.value)
        elif tok.kind == "int":
            advance()
            return ast.PInt(tok.value)
        elif tok.kind == "float":
            advance()
            return ast.PFloat(tok.value)
        elif tok.kind == "string":
            advance()
            return ast.PString(tok.value)
        elif tok.kind == "bool":
            advance()
            return ast.PBool(tok.value)
        elif tok.kind == "(":
            advance()
            if peek().kind == ")":
                advance()
                return ast.PUnit()
            first = parse_pattern()
            if peek().kind == ",":
                pats = [first]
                while peek().kind == ",":
                    advance()
                    pats.append(parse_pattern())
                expect(")")
                return ast.PTuple(pats)
            expect(")")
            return first
        elif tok.kind == "[":
            advance()
            if peek().kind == "]":
                advance()
                return ast.PNil()
            items = [parse_atom_pattern()]
            while peek().kind == ",":
                advance()
                items.append(parse_atom_pattern())
            tail = ast.PNil()
            if peek().kind == "|":
                advance()
                tail = parse_atom_pattern()
            expect("]")
            result = tail
            for item in reversed(items):
                result = ast.PCons(item, result)
            return result
        else:
            raise Error(
                f"expected pattern, got '{tok}' at line {tok.line}, col {tok.col + 1}"
            )

    # --- pattern (constructor or atom) ---
    def parse_pattern():
        tok = peek()
        if tok.kind == "ident" and _is_uppercase(tok.value):
            advance()
            name = tok.value
            args = []
            while True:
                t = peek()
                if t.kind in ("int", "float", "string", "bool", "(", "["):
                    args.append(parse_atom_pattern())
                elif t.kind == "ident" and t.value == "_":
                    args.append(parse_atom_pattern())
                elif t.kind == "ident" and not _is_uppercase(t.value):
                    args.append(parse_atom_pattern())
                else:
                    break
            return ast.PCtor(name, args)
        return parse_atom_pattern()

    # --- case expression ---
    def parse_case():
        nonlocal case_arm_col
        advance()  # consume 'case'
        scrutinee = parse_expr()
        expect("of")
        arm_col = peek().col  # column of first arm's pattern (offside rule)
        saved = case_arm_col
        case_arm_col = arm_col
        arms = []
        while True:
            pat = parse_pattern()
            expect("->")
            body = parse_expr()
            arms.append((pat, body))
            tok = peek()
            if tok.kind == "eof":
                break
            elif tok.col == arm_col:
                continue  # same column → another arm
            else:
                break
        case_arm_col = saved
        return ast.Match(scrutinee, arms)

    # --- type declaration ---
    def parse_type_decl():
        advance()  # consume 'type'
        name_tok = peek()
        name = expect_ident()
        if not _is_uppercase(name):
            raise Error(
                f"type name must start with uppercase, got '{name}' "
                f"at line {name_tok.line}, col {name_tok.col + 1}"
            )
        params = []
        while peek().kind == "ident" and not _is_uppercase(peek().value):
            params.append(expect_ident())
        expect("=")
        if peek().kind == "|":
            advance()
        ctors = []
        while True:
            ctor_name = expect_ident()
            arg_types = []
            while peek().kind == "ident" and (
                case_arm_col < 0 or peek().col > case_arm_col
            ):
                arg_types.append(peek().value)
                advance()
            ctors.append((ctor_name, arg_types))
            if peek().kind == "|":
                advance()
            else:
                break
        return ast.TypeDecl(name, params, ctors)

    # --- import statement ---
    def parse_import():
        advance()  # consume 'import'
        ns_or_name = expect_ident()
        if peek().kind == ":":
            advance()
            module = ns_or_name + ":" + expect_ident()
        else:
            module = ns_or_name
        if peek().kind == "as":
            advance()
            alias = expect_ident()
            return ast.Import(module, [], alias)
        else:
            expect("(")
            names = [expect_ident()]
            while peek().kind == ",":
                advance()
                names.append(expect_ident())
            expect(")")
            return ast.Import(module, names, None)

    # --- export statement ---
    def parse_export():
        advance()  # consume 'export'
        names = [expect_ident()]
        while peek().kind == ",":
            advance()
            names.append(expect_ident())
        return ast.Export(names)

    # --- top-level dispatch ---
    def parse_expr():
        k = peek().kind
        if k == "let":
            return parse_let()
        elif k == "if":
            return parse_if()
        elif k == "fun":
            return parse_fun()
        elif k == "case":
            return parse_case()
        elif k == "type":
            return parse_type_decl()
        elif k == "import":
            return parse_import()
        elif k == "export":
            return parse_export()
        else:
            return parse_pipe()

    exprs = []
    while True:
        k = peek().kind
        if k == "eof":
            break
        else:
            case_arm_col = 0  # top-level: col-0 tokens start new expressions
            exprs.append(parse_expr())
    return exprs


def parse(source: str) -> list:
    return parse_tokens(tokenize(source))


def parse_single(source: str):
    exprs = parse(source)
    if len(exprs) != 1:
        raise Error("expected a single expression")
    return exprs[0]
