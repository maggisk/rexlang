-- Multi-binding let blocks using `and`

import Std:List (map, foldl)

-- Multiple simple bindings
let result =
    let width = 10
    and height = 20
    and area = width * height
    in
    area

-- With function bindings
let hypotenuse =
    let square x = x * x
    and a = 3
    and b = 4
    in
    square a + square b

-- Nested multi-binding
let nested =
    let x = 1
    and y = 2
    in
    let a = x + y
    and b = a * 2
    in
    b

-- Bindings can reference earlier ones
let chained =
    let a = 10
    and b = a + 10
    in
    a + b

-- Using pipe to chain list operations
let total =
    [1, 2, 3, 4, 5]
        |> map (\x -> x * 2)
        |> foldl (\acc x -> acc + x) 0

test "multi-binding let blocks" =
    assert (result == 200)
    assert (hypotenuse == 25)
    assert (nested == 6)
    assert (chained == 30)
    assert (total == 30)
