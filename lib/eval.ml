(* Values are the results of evaluating expressions. *)
type value =
  | VInt of int
  | VString of string
  | VBool of bool
  | VClosure of string * Ast.expr * env
    (* param, body, captured environment *)
and env = (string * value) list

exception Error of string

let value_to_string = function
  | VInt n -> string_of_int n
  | VString s -> Printf.sprintf "%S" s
  | VBool b -> string_of_bool b
  | VClosure _ -> "<fun>"

let lookup name env =
  match List.assoc_opt name env with
  | Some v -> v
  | None -> raise (Error (Printf.sprintf "unbound variable: %s" name))

let as_int = function
  | VInt n -> n
  | v -> raise (Error (Printf.sprintf "expected int, got %s" (value_to_string v)))

let as_bool = function
  | VBool b -> b
  | v -> raise (Error (Printf.sprintf "expected bool, got %s" (value_to_string v)))

let eval_binop op l r =
  match op, l, r with
  (* Arithmetic *)
  | Ast.Add, VInt a, VInt b -> VInt (a + b)
  | Ast.Sub, VInt a, VInt b -> VInt (a - b)
  | Ast.Mul, VInt a, VInt b -> VInt (a * b)
  | Ast.Div, VInt _, VInt 0 -> raise (Error "division by zero")
  | Ast.Div, VInt a, VInt b -> VInt (a / b)
  (* String concatenation *)
  | Ast.Add, VString a, VString b -> VString (a ^ b)
  (* Comparison on ints *)
  | Ast.Lt, VInt a, VInt b -> VBool (a < b)
  | Ast.Gt, VInt a, VInt b -> VBool (a > b)
  | Ast.Leq, VInt a, VInt b -> VBool (a <= b)
  | Ast.Geq, VInt a, VInt b -> VBool (a >= b)
  (* Equality on any matching types *)
  | Ast.Eq, VInt a, VInt b -> VBool (a = b)
  | Ast.Eq, VString a, VString b -> VBool (String.equal a b)
  | Ast.Eq, VBool a, VBool b -> VBool (a = b)
  | Ast.Neq, VInt a, VInt b -> VBool (a <> b)
  | Ast.Neq, VString a, VString b -> VBool (not (String.equal a b))
  | Ast.Neq, VBool a, VBool b -> VBool (a <> b)
  | _ ->
    raise (Error (Printf.sprintf "type error: %s %s %s"
                    (value_to_string l) (Ast.binop_to_string op) (value_to_string r)))

let match_pattern pat value =
  match pat, value with
  | Ast.PWild, _ -> Some []
  | Ast.PVar name, v -> Some [(name, v)]
  | Ast.PInt n, VInt v when n = v -> Some []
  | Ast.PString s, VString v when String.equal s v -> Some []
  | Ast.PBool b, VBool v when b = v -> Some []
  | _ -> None

let rec eval env = function
  | Ast.Int n -> VInt n
  | Ast.String s -> VString s
  | Ast.Bool b -> VBool b
  | Ast.Var name -> lookup name env
  | Ast.Unary_minus e -> VInt (- (as_int (eval env e)))
  | Ast.Binop (op, l, r) ->
    eval_binop op (eval env l) (eval env r)
  | Ast.If (cond, then_e, else_e) ->
    if as_bool (eval env cond) then eval env then_e
    else eval env else_e
  | Ast.Fun (param, body) ->
    VClosure (param, body, env)
  | Ast.App (func, arg) ->
    let closure = eval env func in
    let arg_val = eval env arg in
    apply closure arg_val
  | Ast.Match (scrutinee, arms) ->
    let value = eval env scrutinee in
    let rec try_arms = function
      | [] -> raise (Error "match failure: no pattern matched")
      | (pat, body) :: rest ->
        (match match_pattern pat value with
         | Some bindings -> eval (bindings @ env) body
         | None -> try_arms rest)
    in
    try_arms arms
  | Ast.Let (name, recursive, body, in_expr) ->
    let value =
      if recursive then
        (* Evaluate body in an env where name is bound to itself.
           We use a fixpoint trick: create the closure, then patch
           its environment to include itself. *)
        match eval env body with
        | VClosure (param, cbody, cenv) ->
          let rec fixed_env = (name, VClosure (param, cbody, fixed_env)) :: cenv in
          VClosure (param, cbody, fixed_env)
        | v -> v (* non-function rec binding, just bind normally *)
      else
        eval env body
    in
    (match in_expr with
     | Some e -> eval ((name, value) :: env) e
     | None -> value)

and apply func arg =
  match func with
  | VClosure (param, body, closed_env) ->
    eval ((param, arg) :: closed_env) body
  | v ->
    raise (Error (Printf.sprintf "cannot apply %s as a function" (value_to_string v)))

(* Evaluate a single expression, returning (value, updated_env).
   Top-level lets without 'in' extend the environment for subsequent expressions. *)
let eval_toplevel env expr =
  match expr with
  | Ast.Let (name, _, _, None) as e ->
    let value = eval env e in
    (value, (name, value) :: env)
  | e -> (eval env e, env)

let run_program source =
  let exprs = Parser.parse source in
  let rec loop env last = function
    | [] -> last
    | expr :: rest ->
      let value, env = eval_toplevel env expr in
      loop env value rest
  in
  loop [] (VBool false) exprs

let run source =
  let ast = Parser.parse_single source in
  eval [] ast
