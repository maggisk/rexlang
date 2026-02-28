import std:IO (print)
import std:List (map, filter, sum)

let xs = [1, 2, 3, 4, 5]
let doubled = map (fun x -> x * 2) xs
let evens = filter (fun x -> x % 2 == 0) xs

print (sum doubled)
