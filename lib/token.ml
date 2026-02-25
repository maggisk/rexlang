type t =
  | Int of int
  | String of string
  | Bool of bool
  | Ident of string
  (* Keywords *)
  | Let
  | Rec
  | In
  | If
  | Then
  | Else
  | Fun
  | Match
  | With
  (* Operators *)
  | Plus
  | Minus
  | Star
  | Slash
  | Equal
  | Less
  | Greater
  | Less_equal
  | Greater_equal
  | Not_equal
  (* Delimiters *)
  | Lparen
  | Rparen
  | Arrow
  | Pipe
  | Semicolon2
  | Eof

let to_string = function
  | Int n -> Printf.sprintf "Int(%d)" n
  | String s -> Printf.sprintf "String(%S)" s
  | Bool b -> Printf.sprintf "Bool(%b)" b
  | Ident s -> Printf.sprintf "Ident(%s)" s
  | Let -> "Let"
  | Rec -> "Rec"
  | In -> "In"
  | If -> "If"
  | Then -> "Then"
  | Else -> "Else"
  | Fun -> "Fun"
  | Match -> "Match"
  | With -> "With"
  | Plus -> "Plus"
  | Minus -> "Minus"
  | Star -> "Star"
  | Slash -> "Slash"
  | Equal -> "Equal"
  | Less -> "Less"
  | Greater -> "Greater"
  | Less_equal -> "Less_equal"
  | Greater_equal -> "Greater_equal"
  | Not_equal -> "Not_equal"
  | Lparen -> "Lparen"
  | Rparen -> "Rparen"
  | Arrow -> "Arrow"
  | Pipe -> "Pipe"
  | Semicolon2 -> "Semicolon2"
  | Eof -> "Eof"
