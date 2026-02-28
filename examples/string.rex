import std:IO (print)
import std:String (toUpper, split, join)

let words = split " " "hello world rex"

print (join "-" words)
