let read_file path =
  let ic = open_in path in
  let n = in_channel_length ic in
  let s = really_input_string ic n in
  close_in ic;
  s

let run_file path =
  let source = read_file path in
  match Rexlang.Eval.run_program source with
  | v -> print_endline (Rexlang.Eval.value_to_string v)
  | exception Rexlang.Lexer.Error msg ->
    Printf.eprintf "Lexer error: %s\n" msg; exit 1
  | exception Rexlang.Parser.Error msg ->
    Printf.eprintf "Parse error: %s\n" msg; exit 1
  | exception Rexlang.Eval.Error msg ->
    Printf.eprintf "Runtime error: %s\n" msg; exit 1

let contains_double_semi s =
  let len = String.length s in
  let rec check i =
    if i >= len - 1 then false
    else if s.[i] = ';' && s.[i + 1] = ';' then true
    else check (i + 1)
  in
  len >= 2 && check 0

let repl () =
  Printf.printf "RexLang v0.1.0\n";
  Printf.printf "Type expressions followed by ;; to evaluate. Ctrl-D to exit.\n\n";
  let env = ref [] in
  let buf = Buffer.create 256 in
  let prompt () =
    if Buffer.length buf = 0 then print_string "rex> "
    else print_string "  .. ";
    flush stdout
  in
  prompt ();
  try while true do
    let line = input_line stdin in
    Buffer.add_string buf line;
    Buffer.add_char buf '\n';
    let input = Buffer.contents buf in
    if contains_double_semi input then begin
      Buffer.clear buf;
      match Rexlang.Parser.parse input with
      | exception Rexlang.Lexer.Error msg ->
        Printf.printf "Lexer error: %s\n" msg; prompt ()
      | exception Rexlang.Parser.Error msg ->
        Printf.printf "Parse error: %s\n" msg; prompt ()
      | exprs ->
        let rec loop local_env = function
          | [] -> local_env
          | expr :: rest ->
            match Rexlang.Eval.eval_toplevel local_env expr with
            | value, new_env ->
              Printf.printf "=> %s\n" (Rexlang.Eval.value_to_string value);
              loop new_env rest
            | exception Rexlang.Eval.Error msg ->
              Printf.printf "Runtime error: %s\n" msg;
              local_env
        in
        env := loop !env exprs;
        prompt ()
    end else
      prompt ()
  done with End_of_file ->
    print_newline ()

let () =
  if Array.length Sys.argv < 2 then
    repl ()
  else
    run_file Sys.argv.(1)
