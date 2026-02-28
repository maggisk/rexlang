import std:IO (print)

-- Built-in list examples

let xs = [1, 2, 3, 4, 5]

let rec sum lst =
    case lst of
        [] -> 0
        [h|t] -> h + sum t

print (sum xs)
