import Std:IO (print)

-- I/O builtins: print and readLine

let greet name =
    print ("Hello, " ++ name ++ "!")

export let main _ =
    let _ = greet "RexLang" in
    0
