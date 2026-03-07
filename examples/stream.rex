-- Lazy streams: infinite sequences processed element-by-element.

import Std:Stream (from, iterate, repeat, range, fromList, map, filter, flatMap, take, drop, takeWhile, zip, toList, foldl, Cons)

test "infinite naturals" =
    assert (from 1 |> take 5 == [1, 2, 3, 4, 5])

test "infinite fibonacci" =
    let rec fibs a b = Cons a (\_ -> fibs b (a + b))
    in
    assert (fibs 0 1 |> take 10 == [0, 1, 1, 2, 3, 5, 8, 13, 21, 34])

test "lazy pipeline on infinite stream" =
    let result =
        from 1
            |> map (\x -> x * x)
            |> filter (\x -> x % 2 == 0)
            |> take 5
    in
    assert (result == [4, 16, 36, 64, 100])

test "powers of two" =
    assert (iterate (\x -> x * 2) 1 |> take 8 == [1, 2, 4, 8, 16, 32, 64, 128])

test "takeWhile on infinite stream" =
    assert (from 1 |> takeWhile (\x -> x < 6) == [1, 2, 3, 4, 5])

test "zip two infinite streams" =
    let pairs = zip (from 0) (repeat "x") |> take 3
    in
    assert (pairs == [(0, "x"), (1, "x"), (2, "x")])

test "sum first 100 naturals" =
    assert (range 1 100 |> foldl (\acc x -> acc + x) 0 == 5050)
