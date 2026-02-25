(* Pattern matching examples *)

(* Fibonacci with match *)
let rec fib n =
  match n with
  | 0 -> 0
  | 1 -> 1
  | n -> fib (n - 1) + fib (n - 2)
;;

fib 10
;;

(* Wildcard pattern *)
let describe x =
  match x with
  | 0 -> "zero"
  | 1 -> "one"
  | _ -> "other"
;;

describe 0
;;
describe 1
;;
describe 42
;;

(* Boolean matching *)
let not b =
  match b with
  | true -> false
  | false -> true
;;

not true
;;
not false
