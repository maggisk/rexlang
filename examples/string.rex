-- String module examples

import std:String (toUpper, split, join)

test "split and join" =
    assert ("hello world rex" |> split " " |> join "-" == "hello-world-rex")

test "toUpper" =
    assert (toUpper "hello" == "HELLO")
