type pattern =
  | PWild
  | PVar of string
  | PInt of int
  | PString of string
  | PBool of bool

type binop =
  | Add | Sub | Mul | Div
  | Eq | Lt | Gt | Leq | Geq | Neq

type expr =
  | Int of int
  | String of string
  | Bool of bool
  | Var of string
  | Unary_minus of expr
  | Binop of binop * expr * expr
  | If of expr * expr * expr
  | Let of string * bool * expr * expr option
    (* name, recursive, body, in_expr *)
  | Fun of string * expr
  | App of expr * expr
  | Match of expr * (pattern * expr) list

let pattern_to_string = function
  | PWild -> "_"
  | PVar s -> s
  | PInt n -> string_of_int n
  | PString s -> Printf.sprintf "%S" s
  | PBool b -> string_of_bool b

let binop_to_string = function
  | Add -> "+" | Sub -> "-" | Mul -> "*" | Div -> "/"
  | Eq -> "=" | Lt -> "<" | Gt -> ">" | Leq -> "<=" | Geq -> ">=" | Neq -> "<>"

let rec to_string = function
  | Int n -> string_of_int n
  | String s -> Printf.sprintf "%S" s
  | Bool b -> string_of_bool b
  | Var s -> s
  | Unary_minus e -> Printf.sprintf "(-%s)" (to_string e)
  | Binop (op, l, r) ->
    Printf.sprintf "(%s %s %s)" (to_string l) (binop_to_string op) (to_string r)
  | If (cond, t, e) ->
    Printf.sprintf "(if %s then %s else %s)" (to_string cond) (to_string t) (to_string e)
  | Let (name, recur, body, in_expr) ->
    let r = if recur then " rec" else "" in
    (match in_expr with
     | None -> Printf.sprintf "(let%s %s = %s)" r name (to_string body)
     | Some e -> Printf.sprintf "(let%s %s = %s in %s)" r name (to_string body) (to_string e))
  | Fun (param, body) ->
    Printf.sprintf "(fun %s -> %s)" param (to_string body)
  | App (f, arg) ->
    Printf.sprintf "(%s %s)" (to_string f) (to_string arg)
  | Match (scrutinee, arms) ->
    let arms_str = List.map (fun (p, e) ->
      Printf.sprintf "| %s -> %s" (pattern_to_string p) (to_string e)
    ) arms |> String.concat " " in
    Printf.sprintf "(match %s with %s)" (to_string scrutinee) arms_str
