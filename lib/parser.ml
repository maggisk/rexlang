(* Recursive descent parser for RexLang.

   Grammar (highest to lowest precedence):
     atom       := INT | STRING | BOOL | IDENT | "(" expr ")"
     app        := atom { atom }                  (left-assoc)
     unary      := "-" unary | app
     mult       := unary { ("*" | "/") unary }    (left-assoc)
     add        := mult  { ("+" | "-") mult  }    (left-assoc)
     compare    := add [ ("<" | ">" | "<=" | ">=" | "=" | "<>") add ]
     expr       := let_expr | if_expr | fun_expr | match_expr | compare
     let_expr   := "let" ["rec"] IDENT { IDENT } "=" expr ["in" expr]
     if_expr    := "if" expr "then" expr "else" expr
     fun_expr   := "fun" IDENT { IDENT } "->" expr
     match_expr := "match" expr "with" ["|"] pattern "->" expr { "|" pattern "->" expr }
     pattern    := "_" | IDENT | INT | STRING | BOOL
*)

type t = {
  tokens : Token.t array;
  mutable pos : int;
}

exception Error of string

let create tokens =
  { tokens = Array.of_list tokens; pos = 0 }

let peek p =
  if p.pos >= Array.length p.tokens then Token.Eof
  else p.tokens.(p.pos)

let advance p =
  p.pos <- p.pos + 1

let expect p expected =
  let got = peek p in
  if got = expected then advance p
  else
    raise (Error (Printf.sprintf "expected %s, got %s"
                    (Token.to_string expected) (Token.to_string got)))

let expect_ident p =
  match peek p with
  | Token.Ident s -> advance p; s
  | tok -> raise (Error (Printf.sprintf "expected identifier, got %s"
                           (Token.to_string tok)))

(* --- atom --- *)
let rec parse_atom p =
  match peek p with
  | Token.Int n -> advance p; Ast.Int n
  | Token.String s -> advance p; Ast.String s
  | Token.Bool b -> advance p; Ast.Bool b
  | Token.Ident s -> advance p; Ast.Var s
  | Token.Lparen ->
    advance p;
    let e = parse_expr p in
    expect p Token.Rparen;
    e
  | tok ->
    raise (Error (Printf.sprintf "unexpected token: %s" (Token.to_string tok)))

(* --- application: f x y => App(App(f, x), y) --- *)
and parse_app p =
  let f = parse_atom p in
  let rec loop acc =
    match peek p with
    (* tokens that can start an atom = another argument *)
    | Token.Int _ | Token.String _ | Token.Bool _
    | Token.Ident _ | Token.Lparen ->
      let arg = parse_atom p in
      loop (Ast.App (acc, arg))
    | _ -> acc
  in
  loop f

(* --- unary minus --- *)
and parse_unary p =
  match peek p with
  | Token.Minus ->
    advance p;
    let e = parse_unary p in
    Ast.Unary_minus e
  | _ -> parse_app p

(* --- multiplicative --- *)
and parse_mult p =
  let lhs = parse_unary p in
  let rec loop acc =
    match peek p with
    | Token.Star -> advance p; loop (Ast.Binop (Mul, acc, parse_unary p))
    | Token.Slash -> advance p; loop (Ast.Binop (Div, acc, parse_unary p))
    | _ -> acc
  in
  loop lhs

(* --- additive --- *)
and parse_add p =
  let lhs = parse_mult p in
  let rec loop acc =
    match peek p with
    | Token.Plus -> advance p; loop (Ast.Binop (Add, acc, parse_mult p))
    | Token.Minus -> advance p; loop (Ast.Binop (Sub, acc, parse_mult p))
    | _ -> acc
  in
  loop lhs

(* --- comparison (non-associative: a single comparison) --- *)
and parse_compare p =
  let lhs = parse_add p in
  match peek p with
  | Token.Less -> advance p; Ast.Binop (Lt, lhs, parse_add p)
  | Token.Greater -> advance p; Ast.Binop (Gt, lhs, parse_add p)
  | Token.Less_equal -> advance p; Ast.Binop (Leq, lhs, parse_add p)
  | Token.Greater_equal -> advance p; Ast.Binop (Geq, lhs, parse_add p)
  | Token.Equal -> advance p; Ast.Binop (Eq, lhs, parse_add p)
  | Token.Not_equal -> advance p; Ast.Binop (Neq, lhs, parse_add p)
  | _ -> lhs

(* --- let expression --- *)
and parse_let p =
  advance p; (* consume 'let' *)
  let recursive = peek p = Token.Rec in
  if recursive then advance p;
  let name = expect_ident p in
  (* collect parameters: let f x y = body => let f = fun x -> fun y -> body *)
  let rec collect_params () =
    match peek p with
    | Token.Ident _ ->
      let param = expect_ident p in
      param :: collect_params ()
    | _ -> []
  in
  let params = collect_params () in
  expect p Token.Equal;
  let body = parse_expr p in
  (* wrap body in nested funs for parameters *)
  let body = List.fold_right (fun param acc -> Ast.Fun (param, acc)) params body in
  let in_expr =
    match peek p with
    | Token.In -> advance p; Some (parse_expr p)
    | _ -> None
  in
  Ast.Let (name, recursive, body, in_expr)

(* --- if expression --- *)
and parse_if p =
  advance p; (* consume 'if' *)
  let cond = parse_expr p in
  expect p Token.Then;
  let then_expr = parse_expr p in
  expect p Token.Else;
  let else_expr = parse_expr p in
  Ast.If (cond, then_expr, else_expr)

(* --- fun expression: fun x y -> body => Fun(x, Fun(y, body)) --- *)
and parse_fun p =
  advance p; (* consume 'fun' *)
  let rec collect_params () =
    match peek p with
    | Token.Arrow -> advance p; []
    | Token.Ident _ ->
      let param = expect_ident p in
      param :: collect_params ()
    | tok ->
      raise (Error (Printf.sprintf "expected parameter or '->', got %s"
                      (Token.to_string tok)))
  in
  let params = collect_params () in
  if params = [] then raise (Error "fun requires at least one parameter");
  let body = parse_expr p in
  List.fold_right (fun param acc -> Ast.Fun (param, acc)) params body

(* --- pattern --- *)
and parse_pattern p =
  match peek p with
  | Token.Ident "_" -> advance p; Ast.PWild
  | Token.Ident s -> advance p; Ast.PVar s
  | Token.Int n -> advance p; Ast.PInt n
  | Token.String s -> advance p; Ast.PString s
  | Token.Bool b -> advance p; Ast.PBool b
  | tok ->
    raise (Error (Printf.sprintf "expected pattern, got %s" (Token.to_string tok)))

(* --- match expression --- *)
and parse_match p =
  advance p; (* consume 'match' *)
  let scrutinee = parse_expr p in
  expect p Token.With;
  (* optional leading pipe *)
  if peek p = Token.Pipe then advance p;
  let rec parse_arms () =
    let pat = parse_pattern p in
    expect p Token.Arrow;
    let body = parse_expr p in
    let arm = (pat, body) in
    if peek p = Token.Pipe then begin
      advance p;
      arm :: parse_arms ()
    end else
      [arm]
  in
  let arms = parse_arms () in
  Ast.Match (scrutinee, arms)

(* --- top-level expression dispatch --- *)
and parse_expr p =
  match peek p with
  | Token.Let -> parse_let p
  | Token.If -> parse_if p
  | Token.Fun -> parse_fun p
  | Token.Match -> parse_match p
  | _ -> parse_compare p

let parse_program tokens =
  let p = create tokens in
  let rec loop acc =
    match peek p with
    | Token.Eof -> List.rev acc
    | Token.Semicolon2 -> advance p; loop acc
    | _ ->
      let expr = parse_expr p in
      loop (expr :: acc)
  in
  loop []

let parse source =
  let tokens = Lexer.tokenize source in
  parse_program tokens

let parse_single source =
  match parse source with
  | [e] -> e
  | _ -> raise (Error "expected a single expression")
