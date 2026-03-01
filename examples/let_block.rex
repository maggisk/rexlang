-- Multi-binding let blocks (Elm-style)

import std:List (map, foldl)
import std:IO (print)

-- Multiple simple bindings
let result =
    let width = 10
        height = 20
        area = width * height
    in
    area

-- With function bindings
let hypotenuse =
    let square x = x * x
        a = 3
        b = 4
    in
    square a + square b

-- Nested multi-binding
let nested =
    let x = 1
        y = 2
    in
    let a = x + y
        b = a * 2
    in
    b

-- Bindings can reference earlier ones
let chained =
    let a = 10
        b = a + 10
    in
    a + b

-- Using multi-binding with list operations
let total =
    let xs = [1, 2, 3, 4, 5]
        doubled = map (fn x -> x * 2) xs
        sum = foldl (fn acc x -> acc + x) 0 doubled
    in
    sum

test "multi-binding let blocks" =
    assert result == 200
    assert hypotenuse == 25
    assert nested == 6
    assert chained == 30
    assert total == 30

print (result + hypotenuse + nested + chained + total)
