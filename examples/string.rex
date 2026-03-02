-- String module examples

import std:String (toUpper, split, join)

test "split and join" =
    let words = split " " "hello world rex"
    assert (join "-" words == "hello-world-rex")

test "toUpper" =
    assert (toUpper "hello" == "HELLO")

true
