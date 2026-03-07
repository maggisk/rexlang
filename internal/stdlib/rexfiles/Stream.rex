import Std:Maybe (Just, Nothing)


-- # Type

export type Stream a = Empty | Cons a (() -> Stream a)


-- # Create


-- | Create a stream from a list.
--
--     fromList [1, 2, 3] |> toList == [1, 2, 3]
--
export
fromList : [a] -> Stream a
fromList lst =
    case lst of
        [] ->
            Empty
        [h|t] ->
            Cons h (\_ -> fromList t)

test "fromList" =
    assert (fromList [1, 2, 3] |> toList == [1, 2, 3])
    assert (fromList [1] |> toList == [1])


-- | Create a stream that repeats a value forever.
--
--     repeat 7 |> take 3 == [7, 7, 7]
--
export
repeat : a -> Stream a
repeat x = Cons x (\_ -> repeat x)

test "repeat" =
    assert (repeat 1 |> take 0 == [])
    assert (repeat 7 |> take 3 == [7, 7, 7])


-- | Create a stream by repeatedly applying a function.
--
--     iterate (\x -> x * 2) 1 |> take 5 == [1, 2, 4, 8, 16]
--
export
iterate : (a -> a) -> a -> Stream a
iterate f x = Cons x (\_ -> iterate f (f x))

test "iterate" =
    assert (iterate (\x -> x + 1) 0 |> take 5 == [0, 1, 2, 3, 4])
    assert (iterate (\x -> x * 2) 1 |> take 5 == [1, 2, 4, 8, 16])


-- | Create a stream of integers from start, incrementing by 1.
--
--     from 5 |> take 3 == [5, 6, 7]
--
export
from : Int -> Stream Int
from n = Cons n (\_ -> from (n + 1))

test "from" =
    assert (from 0 |> take 4 == [0, 1, 2, 3])
    assert (from 10 |> take 3 == [10, 11, 12])


-- | Create a stream of integers in a range (inclusive).
--
--     range 1 5 |> toList == [1, 2, 3, 4]
--
export
range : Int -> Int -> Stream Int
range lo hi =
    if lo >= hi then
        Empty
    else
        Cons lo (\_ -> range (lo + 1) hi)

test "range" =
    assert (range 1 5 |> toList == [1, 2, 3, 4])
    assert (range 5 3 |> toList == [])
    assert (range 1 1 |> toList == [])


-- # Transform


-- | Apply a function to every element in a stream.
--
--     fromList [1, 2, 3] |> map (\x -> x * 2) |> toList == [2, 4, 6]
--
export
map : (a -> b) -> Stream a -> Stream b
map f s =
    case s of
        Empty ->
            Empty
        Cons h tail ->
            Cons (f h) (\_ -> map f (tail ()))

test "map" =
    assert (fromList [1, 2, 3] |> map (\x -> x * 2) |> toList == [2, 4, 6])


-- | Keep only elements that satisfy the predicate.
--
--     fromList [1, 2, 3, 4, 5] |> filter (\x -> x % 2 == 0) |> toList == [2, 4]
--
export
filter : (a -> Bool) -> Stream a -> Stream a
filter pred s =
    case s of
        Empty ->
            Empty
        Cons h tail ->
            if pred h then
                Cons h (\_ -> filter pred (tail ()))
            else
                filter pred (tail ())

test "filter" =
    assert (fromList [1, 2, 3, 4, 5] |> filter (\x -> x % 2 == 0) |> toList == [2, 4])
    assert (fromList [1, 2, 3] |> filter (\x -> x > 10) |> toList == [])


-- | Apply a function that returns a stream to every element, then flatten.
--
--     fromList [1, 2, 3] |> flatMap (\x -> fromList [x, x * 10]) |> toList == [1, 10, 2, 20, 3, 30]
--
export
flatMap : (a -> Stream b) -> Stream a -> Stream b
flatMap f s =
    case s of
        Empty ->
            Empty
        Cons h tail ->
            append (f h) (\_ -> flatMap f (tail ()))

test "flatMap" =
    assert (fromList [1, 2, 3] |> flatMap (\x -> fromList [x, x * 10]) |> toList == [1, 10, 2, 20, 3, 30])


-- | Append two streams. Second stream is a thunk to preserve laziness.
append : Stream a -> (() -> Stream a) -> Stream a
append s1 thunk =
    case s1 of
        Empty ->
            thunk ()
        Cons h tail ->
            Cons h (\_ -> append (tail ()) thunk)


-- | Apply a function to each element along with its index.
--
--     fromList ["a", "b", "c"] |> indexedMap (\i x -> (i, x)) |> toList == [(0, "a"), (1, "b"), (2, "c")]
--
export
indexedMap : (Int -> a -> b) -> Stream a -> Stream b
indexedMap f s =
    let rec
        go i stream =
            case stream of
                Empty ->
                    Empty
                Cons h tail ->
                    Cons (f i h) (\_ -> go (i + 1) (tail ()))
    in
    go 0 s

test "indexedMap" =
    assert (fromList ["a", "b"] |> indexedMap (\i x -> (i, x)) |> toList == [(0, "a"), (1, "b")])


-- # Consume


-- | Take the first n elements as a list.
--
--     fromList [1, 2, 3, 4, 5] |> take 3 == [1, 2, 3]
--
export
take : Int -> Stream a -> [a]
take n s =
    if n <= 0 then
        []
    else
        case s of
            Empty ->
                []
            Cons h tail ->
                h :: take (n - 1) (tail ())

test "take" =
    assert (fromList [1, 2, 3, 4, 5] |> take 3 == [1, 2, 3])
    assert (fromList [1, 2] |> take 5 == [1, 2])
    assert (fromList [1, 2, 3] |> take 0 == [])


-- | Drop the first n elements.
--
--     fromList [1, 2, 3, 4, 5] |> drop 2 |> toList == [3, 4, 5]
--
export
drop : Int -> Stream a -> Stream a
drop n s =
    if n <= 0 then
        s
    else
        case s of
            Empty ->
                Empty
            Cons _ tail ->
                drop (n - 1) (tail ())

test "drop" =
    assert (fromList [1, 2, 3, 4, 5] |> drop 2 |> toList == [3, 4, 5])
    assert (fromList [1, 2] |> drop 5 |> toList == [])
    assert (fromList [1, 2, 3] |> drop 0 |> toList == [1, 2, 3])


-- | Take elements while the predicate holds.
--
--     from 1 |> takeWhile (\x -> x < 5) == [1, 2, 3, 4]
--
export
takeWhile : (a -> Bool) -> Stream a -> [a]
takeWhile pred s =
    case s of
        Empty ->
            []
        Cons h tail ->
            if pred h then
                h :: takeWhile pred (tail ())
            else
                []

test "takeWhile" =
    assert (fromList [1, 2, 3, 4, 5] |> takeWhile (\x -> x < 4) == [1, 2, 3])
    assert (fromList [10, 20] |> takeWhile (\x -> x < 5) == [])


-- | Drop elements while the predicate holds.
--
--     fromList [1, 2, 3, 4, 5] |> dropWhile (\x -> x < 3) |> toList == [3, 4, 5]
--
export
dropWhile : (a -> Bool) -> Stream a -> Stream a
dropWhile pred s =
    case s of
        Empty ->
            Empty
        Cons h tail ->
            if pred h then
                dropWhile pred (tail ())
            else
                s

test "dropWhile" =
    assert (fromList [1, 2, 3, 4, 5] |> dropWhile (\x -> x < 3) |> toList == [3, 4, 5])
    assert (fromList [1, 2, 3] |> dropWhile (\x -> x < 10) |> toList == [])


-- | Collect all elements into a list. Only use on finite streams!
--
--     fromList [1, 2, 3] |> toList == [1, 2, 3]
--
export
toList : Stream a -> [a]
toList s =
    case s of
        Empty ->
            []
        Cons h tail ->
            h :: toList (tail ())

test "toList" =
    assert (fromList [1, 2, 3] |> toList == [1, 2, 3])


-- | Reduce a stream from the left. Only use on finite streams!
--
--     fromList [1, 2, 3] |> foldl (\acc x -> acc + x) 0 == 6
--
export
foldl : (b -> a -> b) -> b -> Stream a -> b
foldl f acc s =
    case s of
        Empty ->
            acc
        Cons h tail ->
            foldl f (f acc h) (tail ())

test "foldl" =
    assert (fromList [1, 2, 3] |> foldl (\acc x -> acc + x) 0 == 6)
    assert (fromList [1, 2, 3] |> foldl (\acc x -> acc * x) 1 == 6)


-- | Get the first element, or Nothing if empty.
--
--     from 1 |> head == Just 1
--
export
head : Stream a -> Maybe a
head s =
    case s of
        Empty ->
            Nothing
        Cons h _ ->
            Just h

test "head" =
    assert (fromList [1, 2, 3] |> head == Just 1)


-- | Check if a stream is empty.
--
--     isEmpty Empty == true
--
export
isEmpty : Stream a -> Bool
isEmpty s =
    case s of
        Empty ->
            true
        _ ->
            false

test "isEmpty" =
    assert (fromList [1] |> isEmpty |> not)
    assert (fromList [1] |> drop 5 |> isEmpty)


-- # Combine


-- | Zip two streams into a stream of pairs. Stops when either is exhausted.
--
--     zip (fromList [1, 2, 3]) (fromList ["a", "b"]) |> toList == [(1, "a"), (2, "b")]
--
export
zip : Stream a -> Stream b -> Stream (a, b)
zip s1 s2 =
    case s1 of
        Empty ->
            Empty
        Cons h1 tail1 ->
            case s2 of
                Empty ->
                    Empty
                Cons h2 tail2 ->
                    Cons (h1, h2) (\_ -> zip (tail1 ()) (tail2 ()))

test "zip" =
    assert (zip (fromList [1, 2, 3]) (fromList ["a", "b"]) |> toList == [(1, "a"), (2, "b")])
    assert (zip (from 0) (fromList ["a", "b", "c"]) |> toList == [(0, "a"), (1, "b"), (2, "c")])


-- | Zip two streams using a combining function.
--
--     zipWith (\a b -> a + b) (fromList [1, 2, 3]) (fromList [10, 20, 30]) |> toList == [11, 22, 33]
--
export
zipWith : (a -> b -> c) -> Stream a -> Stream b -> Stream c
zipWith f s1 s2 =
    case s1 of
        Empty ->
            Empty
        Cons h1 tail1 ->
            case s2 of
                Empty ->
                    Empty
                Cons h2 tail2 ->
                    Cons (f h1 h2) (\_ -> zipWith f (tail1 ()) (tail2 ()))

test "zipWith" =
    assert (zipWith (\a b -> a + b) (fromList [1, 2, 3]) (fromList [10, 20, 30]) |> toList == [11, 22, 33])


-- # Laziness demonstration


test "lazy: map and filter don't traverse whole list" =
    let result =
        from 1
            |> map (\x -> x * x)
            |> filter (\x -> x % 2 == 0)
            |> take 5
    in
    assert (result == [4, 16, 36, 64, 100])


test "lazy: infinite fibonacci" =
    let rec fibs a b = Cons a (\_ -> fibs b (a + b))
    in
    assert (fibs 0 1 |> take 8 == [0, 1, 1, 2, 3, 5, 8, 13])
