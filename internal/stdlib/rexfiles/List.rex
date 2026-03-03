export sortWith


-- # Query


-- | Determine the length of a list.
--
--     length [1, 2, 3] == 3
--
length : [a] -> Int
export let rec length lst =
    case lst of
        [] ->
            0
        [_|t] ->
            1 + length t

test "length" =
    assert (length [] == 0)
    assert (length [1, 2, 3] == 3)


-- | Return true if the list is empty.
--
--     isEmpty [] == true
--
isEmpty : [a] -> Bool
export let isEmpty lst =
    case lst of
        [] ->
            true
        _ ->
            false

test "isEmpty" =
    assert (isEmpty [])
    assert ([1] |> isEmpty |> not)


-- # Deconstruct


-- | Extract the first element of a list, or Nothing if empty.
--
--     head [1, 2, 3] == Just 1
--     head [] == Nothing
--
head : [a] -> Maybe a
export let head lst =
    case lst of
        [h|_] ->
            Just h
        [] ->
            Nothing

test "head" =
    assert (head [1, 2, 3] == Just 1)
    assert (head [] == Nothing)


-- | Extract the rest of the list after the first element, or Nothing if empty.
--
--     tail [1, 2, 3] == Just [2, 3]
--     tail [] == Nothing
--
tail : [a] -> Maybe [a]
export let tail lst =
    case lst of
        [_|t] ->
            Just t
        [] ->
            Nothing

test "tail" =
    assert (tail [1, 2, 3] == Just [2, 3])
    assert (tail [1] == Just [])
    assert (tail [] == Nothing)


-- | Get the last element of a list, or Nothing if empty.
--
--     last [1, 2, 3] == Just 3
--     last [] == Nothing
--
last : [a] -> Maybe a
export let rec last lst =
    case lst of
        [x] ->
            Just x
        [_|t] ->
            last t
        [] ->
            Nothing

test "last" =
    assert (last [1, 2, 3] == Just 3)
    assert (last [1] == Just 1)
    assert (last [] == Nothing)


-- | Return all elements except the last, or Nothing if empty.
--
--     init [1, 2, 3] == Just [1, 2]
--     init [] == Nothing
--
init : [a] -> Maybe [a]
export let rec init lst =
    case lst of
        [_] ->
            Just []
        [h|t] ->
            case init t of
                Just rest ->
                    Just (h :: rest)
                Nothing ->
                    Nothing
        [] ->
            Nothing

test "init" =
    assert (init [1, 2, 3] == Just [1, 2])
    assert (init [1] == Just [])
    assert (init [] == Nothing)


-- | Get the element at a zero-based index, or Nothing if out of bounds.
--
--     nth 1 [10, 20, 30] == Just 20
--
nth : Int -> [a] -> Maybe a
export let rec nth n lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            if n == 0 then
                Just h
            else
                nth (n - 1) t

test "nth" =
    assert (nth 0 [10, 20, 30] == Just 10)
    assert (nth 2 [10, 20, 30] == Just 30)
    assert (nth 5 [10, 20, 30] == Nothing)


-- # Fold


-- | Reduce a list from the left.
--
--     foldl (\acc x -> acc + x) 0 [1, 2, 3] == 6
--
foldl : (b -> a -> b) -> b -> [a] -> b
export let rec foldl f acc lst =
    case lst of
        [] ->
            acc
        [h|t] ->
            foldl f (f acc h) t

test "foldl" =
    assert (foldl (\acc x -> acc + x) 0 [1, 2, 3] == 6)


-- | Reduce a list from the right.
--
--     foldr (\x acc -> x :: acc) [] [1, 2, 3] == [1, 2, 3]
--
foldr : (a -> b -> b) -> b -> [a] -> b
export let rec foldr f acc lst =
    case lst of
        [] ->
            acc
        [h|t] ->
            f h (foldr f acc t)

test "foldr" =
    assert ([1, 2, 3] |> foldr (\x acc -> x :: acc) [] |> head == Just 1)


-- | Reduce a list using the first element as the initial accumulator.
-- Crashes on empty list.
--
--     foldl1 (\a b -> a + b) [1, 2, 3] == 6
--
foldl1 : (a -> a -> a) -> [a] -> a
export let foldl1 f lst =
    case lst of
        [] ->
            error "foldl1: empty list"
        [h|t] ->
            foldl f h t

test "foldl1" =
    assert (foldl1 (\a b -> a + b) [1, 2, 3] == 6)
    assert (foldl1 (\a b -> a + b) [42] == 42)


-- # Aggregate


-- | Sum all numbers in a list.
--
--     sum [1, 2, 3, 4, 5] == 15
--
sum : [Int] -> Int
export let sum lst =
    foldl (\acc x -> acc + x) 0 lst

test "sum" =
    assert (sum [1, 2, 3, 4, 5] == 15)


-- | Multiply all numbers in a list.
--
--     product [1, 2, 3, 4] == 24
--
product : [Int] -> Int
export let product lst =
    foldl (\acc x -> acc * x) 1 lst

test "product" =
    assert (product [1, 2, 3, 4] == 24)


-- | Determine if any elements satisfy the predicate.
--
--     any (\x -> x > 3) [1, 2, 3, 4] == true
--
any : (a -> Bool) -> [a] -> Bool
export let any pred lst =
    foldl (\acc x -> acc || pred x) false lst

test "any" =
    assert (any (\x -> x > 3) [1, 2, 3, 4])
    assert ([1, 2, 3] |> any (\x -> x > 10) |> not)


-- | Determine if all elements satisfy the predicate.
--
--     all (\x -> x > 0) [1, 2, 3] == true
--
all : (a -> Bool) -> [a] -> Bool
export let all pred lst =
    foldl (\acc x -> acc && pred x) true lst

test "all" =
    assert (all (\x -> x > 0) [1, 2, 3])
    assert ([1, 2, 3] |> all (\x -> x > 2) |> not)


-- | Return the largest element, or Nothing for empty lists. Requires Ord.
--
--     maximum [3, 1, 4, 1, 5] == Just 5
--
maximum : [a] -> Maybe a
export let maximum lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            Just (foldl (\a b ->
                case compare a b of
                    GT ->
                        a
                    _ ->
                        b) h t)

test "maximum" =
    assert (maximum [3, 1, 4, 1, 5] == Just 5)
    assert (maximum [] == Nothing)


-- | Return the smallest element, or Nothing for empty lists. Requires Ord.
--
--     minimum [3, 1, 4, 1, 5] == Just 1
--
minimum : [a] -> Maybe a
export let minimum lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            Just (foldl (\a b ->
                case compare a b of
                    LT ->
                        a
                    _ ->
                        b) h t)

test "minimum" =
    assert (minimum [3, 1, 4, 1, 5] == Just 1)
    assert (minimum [] == Nothing)


-- # Create


-- | Create a list with n copies of a value.
--
--     repeat 3 0 == [0, 0, 0]
--
repeat : Int -> a -> [a]
export let rec repeat n x =
    if n <= 0 then
        []
    else
        x :: repeat (n - 1) x

test "repeat" =
    assert (sum (repeat 5 1) == 5)
    assert (length (repeat 0 99) == 0)


-- | Create a list of integers from start (inclusive) to stop (exclusive).
--
--     range 1 5 == [1, 2, 3, 4]
--
range : Int -> Int -> [Int]
export let rec range start stop =
    if start >= stop then
        []
    else
        start :: range (start + 1) stop

test "range" =
    assert (sum (range 1 6) == 15)
    assert (length (range 3 3) == 0)


-- # Transform


-- | Apply a function to every element of a list.
--
--     map (\x -> x * 2) [1, 2, 3] == [2, 4, 6]
--
map : (a -> b) -> [a] -> [b]
export let rec map f lst =
    case lst of
        [] ->
            []
        [h|t] ->
            f h :: map f t

test "map" =
    assert ([1, 2, 3] |> map (\x -> x * 2) |> sum == 12)


-- | Apply a function to every element along with its index.
--
--     indexedMap (\i x -> i) ["a", "b", "c"] == [0, 1, 2]
--
indexedMap : (Int -> a -> b) -> [a] -> [b]
export let indexedMap f lst =
    let rec go i xs =
        case xs of
            [] ->
                []
            [h|t] ->
                f i h :: go (i + 1) t
    in
    go 0 lst

test "indexedMap" =
    assert ([10, 20, 30] |> indexedMap (\i x -> i) |> sum == 3)


-- | Keep only elements that satisfy the predicate.
--
--     filter (\x -> x > 2) [1, 2, 3, 4] == [3, 4]
--
filter : (a -> Bool) -> [a] -> [a]
export let rec filter pred lst =
    case lst of
        [] ->
            []
        [h|t] ->
            if pred h then
                h :: filter pred t
            else
                filter pred t

test "filter" =
    assert ([1, 2, 3, 4, 5] |> filter (\x -> x > 2) |> length == 3)


-- | Map with a function returning Maybe, keeping only Just values.
--
--     filterMap (\x -> if x > 2 then Just (x * 10) else Nothing) [1, 2, 3, 4] == [30, 40]
--
filterMap : (a -> Maybe b) -> [a] -> [b]
export let rec filterMap f lst =
    case lst of
        [] ->
            []
        [h|t] ->
            case f h of
                Just v ->
                    v :: filterMap f t
                Nothing ->
                    filterMap f t

test "filterMap" =
    assert ([1, 2, 3, 4] |> filterMap (\x -> if x > 2 then Just (x * 10) else Nothing) == [30, 40])
    assert (filterMap (\x -> Nothing) [1, 2, 3] == [])


-- | Reverse a list.
--
--     reverse [1, 2, 3] == [3, 2, 1]
--
reverse : [a] -> [a]
export let rec reverse lst =
    let rec go acc xs =
        case xs of
            [] ->
                acc
            [h|t] ->
                go (h :: acc) t
    in
    go [] lst

test "reverse" =
    assert ([1, 2, 3] |> reverse |> head == Just 3)


-- # Combine


-- | Append two lists.
--
--     append [1, 2] [3, 4] == [1, 2, 3, 4]
--
append : [a] -> [a] -> [a]
export let rec append lst1 lst2 =
    case lst1 of
        [] ->
            lst2
        [h|t] ->
            h :: append t lst2

test "append" =
    assert (length (append [1, 2] [3, 4]) == 4)
    assert (sum (append [1, 2] [3, 4]) == 10)


-- | Flatten a list of lists.
--
--     concat [[1, 2], [3], [4, 5]] == [1, 2, 3, 4, 5]
--
concat : [[a]] -> [a]
export let concat lsts =
    foldr append [] lsts

test "concat" =
    assert ([[1, 2], [3], [4, 5]] |> concat |> sum == 15)


-- | Map then flatten.
--
--     flatMap (\x -> [x, x]) [1, 2, 3] == [1, 1, 2, 2, 3, 3]
--
flatMap : (a -> [b]) -> [a] -> [b]
export let flatMap f lst =
    map f lst |> concat

test "flatMap" =
    assert (flatMap (\x -> [x, x]) [1, 2, 3] == [1, 1, 2, 2, 3, 3])
    assert (flatMap (\x -> []) [1, 2, 3] == [])


-- | Pair up elements from two lists. Stops at the shorter list.
--
--     zip [1, 2, 3] ["a", "b"] == [(1, "a"), (2, "b")]
--
zip : [a] -> [b] -> [(a, b)]
export let rec zip xs ys =
    case xs of
        [] ->
            []
        [x|xrest] ->
            case ys of
                [] ->
                    []
                [y|yrest] ->
                    (x, y) :: zip xrest yrest

test "zip" =
    let pairs = zip [1, 2, 3] [4, 5, 6]
    assert (length pairs == 3)
    assert (pairs |> foldl (\acc pair -> let (a, b) = pair in acc + a + b) 0 == 21)


-- | Combine two lists element-wise with a function.
--
--     zipWith (\a b -> a + b) [1, 2, 3] [10, 20, 30] == [11, 22, 33]
--
zipWith : (a -> b -> c) -> [a] -> [b] -> [c]
export let rec zipWith f xs ys =
    case xs of
        [] ->
            []
        [x|xrest] ->
            case ys of
                [] ->
                    []
                [y|yrest] ->
                    f x y :: zipWith f xrest yrest

test "zipWith" =
    assert (zipWith (\a b -> a + b) [1, 2, 3] [10, 20, 30] == [11, 22, 33])
    assert (zipWith (\a b -> a + b) [1, 2] [10] == [11])
    assert (zipWith (\a b -> a + b) [] [1, 2] == [])


-- | Split a list of pairs into a pair of lists.
--
--     unzip [(1, "a"), (2, "b")] == ([1, 2], ["a", "b"])
--
unzip : [(a, b)] -> ([a], [b])
export let unzip pairs =
    let rec go xs ys zs =
        case zs of
            [] ->
                (reverse xs, reverse ys)
            [h|t] ->
                let (a, b) = h in
                go (a :: xs) (b :: ys) t
    in
    go [] [] pairs

test "unzip" =
    let (xs, ys) = [(1, "a"), (2, "b"), (3, "c")] |> unzip
    assert (xs == [1, 2, 3])
    assert (ys == ["a", "b", "c"])


-- | Put a separator between every element of a list.
--
--     intersperse 0 [1, 2, 3] == [1, 0, 2, 0, 3]
--
intersperse : a -> [a] -> [a]
export let rec intersperse sep lst =
    case lst of
        [] ->
            []
        [_] ->
            lst
        [h|t] ->
            h :: sep :: intersperse sep t

test "intersperse" =
    assert ([1, 2, 3] |> intersperse 0 |> sum == 6)
    assert ([1, 2, 3] |> intersperse 0 |> length == 5)


-- # Slice


-- | Take the first n elements of a list.
--
--     take 2 [1, 2, 3, 4] == [1, 2]
--
take : Int -> [a] -> [a]
export let rec take n lst =
    if n == 0 then
        []
    else
        case lst of
            [] ->
                []
            [h|t] ->
                h :: take (n - 1) t

test "take" =
    assert ([1, 2, 3, 4] |> take 2 |> sum == 3)


-- | Drop the first n elements of a list.
--
--     drop 2 [1, 2, 3, 4] == [3, 4]
--
drop : Int -> [a] -> [a]
export let rec drop n lst =
    if n == 0 then
        lst
    else
        case lst of
            [] ->
                []
            [_|t] ->
                drop (n - 1) t

test "drop" =
    assert ([1, 2, 3, 4] |> drop 2 |> sum == 7)


-- | Take elements while predicate holds.
--
--     takeWhile (\x -> x < 3) [1, 2, 3, 4] == [1, 2]
--
takeWhile : (a -> Bool) -> [a] -> [a]
export let rec takeWhile pred lst =
    case lst of
        [] ->
            []
        [h|t] ->
            if pred h then
                h :: takeWhile pred t
            else
                []

test "takeWhile" =
    assert ([1, 2, 3, 4] |> takeWhile (\x -> x < 3) == [1, 2])
    assert (takeWhile (\x -> x < 0) [1, 2, 3] == [])


-- | Drop elements while predicate holds.
--
--     dropWhile (\x -> x < 3) [1, 2, 3, 4] == [3, 4]
--
dropWhile : (a -> Bool) -> [a] -> [a]
export let rec dropWhile pred lst =
    case lst of
        [] ->
            []
        [h|t] ->
            if pred h then
                dropWhile pred t
            else
                lst

test "dropWhile" =
    assert ([1, 2, 3, 4] |> dropWhile (\x -> x < 3) == [3, 4])
    assert (dropWhile (\x -> x < 0) [1, 2, 3] == [1, 2, 3])


-- | Split a list at the point where the predicate stops holding.
--
--     span (\x -> x < 3) [1, 2, 3, 4] == ([1, 2], [3, 4])
--
span : (a -> Bool) -> [a] -> ([a], [a])
export let span pred lst =
    let rec go acc xs =
        case xs of
            [] ->
                (reverse acc, [])
            [h|t] ->
                if pred h then
                    go (h :: acc) t
                else
                    (reverse acc, xs)
    in
    go [] lst

test "span" =
    let (a, b) = [1, 2, 3, 4] |> span (\x -> x < 3)
    assert (a == [1, 2])
    assert (b == [3, 4])


-- # Search


-- | Find the first element satisfying a predicate, or Nothing.
--
--     find (\x -> x > 2) [1, 2, 3, 4] == Just 3
--
find : (a -> Bool) -> [a] -> Maybe a
export let rec find pred lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            if pred h then
                Just h
            else
                find pred t

test "find" =
    assert ([1, 2, 3, 4] |> find (\x -> x > 2) == Just 3)
    assert (find (\x -> x > 10) [1, 2, 3] == Nothing)


-- | Split a list into elements that pass and fail a predicate.
--
--     partition (\x -> x > 2) [1, 2, 3, 4] == ([3, 4], [1, 2])
--
partition : (a -> Bool) -> [a] -> ([a], [a])
export let partition pred lst =
    let rec go yes no xs =
        case xs of
            [] ->
                (reverse yes, reverse no)
            [h|t] ->
                if pred h then
                    go (h :: yes) no t
                else
                    go yes (h :: no) t
    in
    go [] [] lst

test "partition" =
    let (yes, no) = [1, 2, 3, 4] |> partition (\x -> x > 2)
    assert (sum yes == 7)
    assert (sum no == 3)


-- # Deduplicate


-- | Remove duplicate elements (O(n²), uses ==).
--
--     unique [1, 2, 1, 3, 2] == [1, 2, 3]
--
unique : [a] -> [a]
export let unique lst =
    let rec go acc xs =
        case xs of
            [] ->
                reverse acc
            [h|t] ->
                if acc |> any (\x -> x == h) then
                    go acc t
                else
                    go (h :: acc) t
    in
    go [] lst

test "unique" =
    assert ([1, 2, 1, 3, 2] |> unique == [1, 2, 3])
    assert (unique [] == [])
    assert ([1, 1, 1] |> unique == [1])


-- | Remove duplicates by a key function (O(n²), uses == on keys).
-- Keeps the first element for each distinct key.
--
--     uniqueBy (\x -> x % 3) [1, 2, 3, 4, 5] == [1, 2, 3]
--
uniqueBy : (a -> b) -> [a] -> [a]
export let uniqueBy f lst =
    let rec go seen acc xs =
        case xs of
            [] ->
                reverse acc
            [h|t] ->
                let key = f h in
                if seen |> any (\k -> k == key) then
                    go seen acc t
                else
                    go (key :: seen) (h :: acc) t
    in
    go [] [] lst

test "uniqueBy" =
    assert (uniqueBy (\x -> x % 3) [1, 2, 3, 4, 5] == [1, 2, 3])
    assert (uniqueBy (\x -> x) [1, 2, 1] == [1, 2])
    assert (uniqueBy (\x -> 0) [1, 2, 3] == [1])


-- # Sort


-- sortWith is a builtin

test "sortWith" =
    assert ([5, 3, 1, 4, 2] |> sortWith (\a b -> compare a b) == [1, 2, 3, 4, 5])
    assert ([1, 2, 3] |> sortWith (\a b -> compare b a) == [3, 2, 1])


-- | Sort a list using the default compare function.
--
--     sort [3, 1, 2] == [1, 2, 3]
--
sort : [a] -> [a]
export let sort lst =
    sortWith (\a b -> compare a b) lst

test "sort" =
    assert ([3, 1, 4, 1, 5] |> sort == [1, 1, 3, 4, 5])
    assert (sort [] == [])


-- | Sort a list by a derived key.
--
--     sortBy (\x -> 0 - x) [3, 1, 2] == [3, 2, 1]
--
sortBy : (a -> b) -> [a] -> [a]
export let sortBy f lst =
    sortWith (\a b -> compare (f a) (f b)) lst

test "sortBy" =
    assert ([3, 1, 2] |> sortBy (\x -> 0 - x) == [3, 2, 1])
