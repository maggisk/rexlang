import std:IO (print)

let rec fact n =
    case n of
        0 ->
            1
        _ ->
            n * fact (n - 1)


print (fact 10)
