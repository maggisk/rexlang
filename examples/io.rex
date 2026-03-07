import Std:IO (print)

-- I/O builtins: print and readLine

greet name =
    print ("Hello, " ++ name ++ "!")

main _ =
    let _ = greet "RexLang" in
    0
