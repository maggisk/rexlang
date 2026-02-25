type t = {
  source : string;
  mutable pos : int;
}

exception Error of string

let create source = { source; pos = 0 }

let peek lexer =
  if lexer.pos >= String.length lexer.source then None
  else Some lexer.source.[lexer.pos]

let advance lexer =
  lexer.pos <- lexer.pos + 1

let current_char lexer =
  lexer.source.[lexer.pos]

let is_digit = function '0' .. '9' -> true | _ -> false
let is_alpha = function 'a' .. 'z' | 'A' .. 'Z' | '_' -> true | _ -> false
let is_alnum c = is_digit c || is_alpha c

let skip_whitespace lexer =
  while lexer.pos < String.length lexer.source
        && (current_char lexer = ' '
            || current_char lexer = '\n'
            || current_char lexer = '\t'
            || current_char lexer = '\r') do
    advance lexer
  done

let skip_comment lexer =
  (* skip '(' and '*' *)
  advance lexer;
  advance lexer;
  let depth = ref 1 in
  while !depth > 0 do
    if lexer.pos >= String.length lexer.source then
      raise (Error "unterminated comment")
    else
      let c = current_char lexer in
      advance lexer;
      if c = '(' && peek lexer = Some '*' then begin
        advance lexer;
        depth := !depth + 1
      end else if c = '*' && peek lexer = Some ')' then begin
        advance lexer;
        depth := !depth - 1
      end
  done

let skip_whitespace_and_comments lexer =
  let continue = ref true in
  while !continue do
    skip_whitespace lexer;
    if lexer.pos + 1 < String.length lexer.source
       && current_char lexer = '('
       && lexer.source.[lexer.pos + 1] = '*'
    then skip_comment lexer
    else continue := false
  done

let read_number lexer =
  let start = lexer.pos in
  while lexer.pos < String.length lexer.source && is_digit (current_char lexer) do
    advance lexer
  done;
  let s = String.sub lexer.source start (lexer.pos - start) in
  Token.Int (int_of_string s)

let read_string lexer =
  advance lexer; (* skip opening '"' *)
  let buf = Buffer.create 16 in
  let rec loop () =
    if lexer.pos >= String.length lexer.source then
      raise (Error "unterminated string")
    else
      let c = current_char lexer in
      advance lexer;
      match c with
      | '"' -> Token.String (Buffer.contents buf)
      | '\\' ->
        if lexer.pos >= String.length lexer.source then
          raise (Error "unterminated string escape");
        let esc = current_char lexer in
        advance lexer;
        (match esc with
         | 'n' -> Buffer.add_char buf '\n'
         | 't' -> Buffer.add_char buf '\t'
         | '\\' -> Buffer.add_char buf '\\'
         | '"' -> Buffer.add_char buf '"'
         | _ -> raise (Error (Printf.sprintf "unknown escape: \\%c" esc)));
        loop ()
      | c ->
        Buffer.add_char buf c;
        loop ()
  in
  loop ()

let keyword_or_ident = function
  | "let" -> Token.Let
  | "rec" -> Token.Rec
  | "in" -> Token.In
  | "if" -> Token.If
  | "then" -> Token.Then
  | "else" -> Token.Else
  | "fun" -> Token.Fun
  | "match" -> Token.Match
  | "with" -> Token.With
  | "true" -> Token.Bool true
  | "false" -> Token.Bool false
  | s -> Token.Ident s

let read_ident lexer =
  let start = lexer.pos in
  while lexer.pos < String.length lexer.source && is_alnum (current_char lexer) do
    advance lexer
  done;
  let s = String.sub lexer.source start (lexer.pos - start) in
  keyword_or_ident s

let next_token lexer =
  skip_whitespace_and_comments lexer;
  if lexer.pos >= String.length lexer.source then Token.Eof
  else
    let c = current_char lexer in
    match c with
    | '0' .. '9' -> read_number lexer
    | '"' -> read_string lexer
    | 'a' .. 'z' | 'A' .. 'Z' | '_' -> read_ident lexer
    | '+' -> advance lexer; Token.Plus
    | '*' -> advance lexer; Token.Star
    | '/' -> advance lexer; Token.Slash
    | '=' -> advance lexer; Token.Equal
    | '(' -> advance lexer; Token.Lparen
    | ')' -> advance lexer; Token.Rparen
    | '-' ->
      advance lexer;
      if peek lexer = Some '>' then begin
        advance lexer; Token.Arrow
      end else Token.Minus
    | '<' ->
      advance lexer;
      (match peek lexer with
       | Some '=' -> advance lexer; Token.Less_equal
       | Some '>' -> advance lexer; Token.Not_equal
       | _ -> Token.Less)
    | '>' ->
      advance lexer;
      if peek lexer = Some '=' then begin
        advance lexer; Token.Greater_equal
      end else Token.Greater
    | '|' -> advance lexer; Token.Pipe
    | ';' ->
      advance lexer;
      if peek lexer = Some ';' then begin
        advance lexer; Token.Semicolon2
      end else
        raise (Error "expected ';;', got single ';'")
    | c -> raise (Error (Printf.sprintf "unexpected character: %c" c))

let tokenize source =
  let lexer = create source in
  let rec loop acc =
    let tok = next_token lexer in
    match tok with
    | Token.Eof -> List.rev (Token.Eof :: acc)
    | _ -> loop (tok :: acc)
  in
  loop []
