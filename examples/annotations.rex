import std:IO (println)

double : Int -> Int
let double x = x * 2

identity : a -> a
let identity x = x

greet : String -> String
let greet name = "Hello, ${name}!"

println (double 21)
println (identity "hello")
println (greet "Rex")

"done"
