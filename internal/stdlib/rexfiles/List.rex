export length, append, reverse, map, filter, foldl, foldr, head, tail, take, drop, sum, product, any, all, isEmpty, repeat, range, zip, last, init, nth, find, partition, intersperse, concat, flatMap, indexedMap, maximum, minimum, sort, sortBy, sortWith, takeWhile, dropWhile, span, unique, uniqueBy, filterMap, zipWith, unzip, foldl1


-- # Query


-- | Determine the length of a list.
--
--     length [1, 2, 3] == 3
--
let rec length lst =
    case lst of
        [] ->
            0
        [_|t] ->
            1 + length t


-- | Return true if the list is empty.
--
--     isEmpty [] == true
--
let isEmpty lst =
    case lst of
        [] ->
            true
        _ ->
            false


-- # Fold


-- | Reduce a list from the left.
--
--     foldl (fn acc x -> acc + x) 0 [1, 2, 3] == 6
--
let rec foldl f acc lst =
    case lst of
        [] ->
            acc
        [h|t] ->
            foldl f (f acc h) t


-- | Reduce a list from the right.
--
--     foldr (fn x acc -> x :: acc) [] [1, 2, 3] == [1, 2, 3]
--
let rec foldr f acc lst =
    case lst of
        [] ->
            acc
        [h|t] ->
            f h (foldr f acc t)


-- # Create


-- | Create a list with n copies of a value.
--
--     repeat 3 0 == [0, 0, 0]
--
let rec repeat n x =
    if n <= 0 then
        []
    else
        x :: repeat (n - 1) x


-- | Create a list of integers from start (inclusive) to stop (exclusive).
--
--     range 1 5 == [1, 2, 3, 4]
--
let rec range start stop =
    if start >= stop then
        []
    else
        start :: range (start + 1) stop


-- # Transform


-- | Apply a function to every element of a list.
--
--     map (fn x -> x * 2) [1, 2, 3] == [2, 4, 6]
--
let rec map f lst =
    case lst of
        [] ->
            []
        [h|t] ->
            f h :: map f t


-- | Apply a function to every element along with its index.
--
--     indexedMap (fn i x -> i) ["a", "b", "c"] == [0, 1, 2]
--
let indexedMap f lst =
    let rec go i xs =
        case xs of
            [] ->
                []
            [h|t] ->
                f i h :: go (i + 1) t
    in
    go 0 lst


-- | Keep only elements that satisfy the predicate.
--
--     filter (fn x -> x > 2) [1, 2, 3, 4] == [3, 4]
--
let rec filter pred lst =
    case lst of
        [] ->
            []
        [h|t] ->
            if pred h then
                h :: filter pred t
            else
                filter pred t


-- # Combine


-- | Append two lists.
--
--     append [1, 2] [3, 4] == [1, 2, 3, 4]
--
let rec append lst1 lst2 =
    case lst1 of
        [] ->
            lst2
        [h|t] ->
            h :: append t lst2


-- | Flatten a list of lists.
--
--     concat [[1, 2], [3], [4, 5]] == [1, 2, 3, 4, 5]
--
let concat lsts =
    foldr append [] lsts


-- | Map then flatten.
--
--     flatMap (fn x -> [x, x]) [1, 2, 3] == [1, 1, 2, 2, 3, 3]
--
let flatMap f lst =
    concat (map f lst)


-- | Pair up elements from two lists. Stops at the shorter list.
--
--     zip [1, 2, 3] ["a", "b"] == [(1, "a"), (2, "b")]
--
let rec zip xs ys =
    case xs of
        [] ->
            []
        [x|xrest] ->
            case ys of
                [] ->
                    []
                [y|yrest] ->
                    (x, y) :: zip xrest yrest


-- | Put a separator between every element of a list.
--
--     intersperse 0 [1, 2, 3] == [1, 0, 2, 0, 3]
--
let rec intersperse sep lst =
    case lst of
        [] ->
            []
        [_] ->
            lst
        [h|t] ->
            h :: sep :: intersperse sep t


-- | Reverse a list.
--
--     reverse [1, 2, 3] == [3, 2, 1]
--
let rec reverse lst =
    let rec go acc xs =
        case xs of
            [] ->
                acc
            [h|t] ->
                go (h :: acc) t
    in
    go [] lst


-- # Deconstruct


-- | Extract the first element of a list.
--
--     head [1, 2, 3] == 1
--
let head lst =
    case lst of
        [h|_] ->
            h
        [] ->
            error "head: empty list"


-- | Extract the rest of the list after the first element.
--
--     tail [1, 2, 3] == [2, 3]
--
let tail lst =
    case lst of
        [_|t] ->
            t
        [] ->
            error "tail: empty list"


-- | Get the last element of a list.
--
--     last [1, 2, 3] == 3
--
let rec last lst =
    case lst of
        [x] ->
            x
        [_|t] ->
            last t
        [] ->
            error "last: empty list"


-- | Return all elements except the last.
--
--     init [1, 2, 3] == [1, 2]
--
let rec init lst =
    case lst of
        [_] ->
            []
        [h|t] ->
            h :: init t
        [] ->
            error "init: empty list"


-- | Get the element at a zero-based index, or Nothing if out of bounds.
--
--     nth 1 [10, 20, 30] == Just 20
--
let rec nth n lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            if n == 0 then
                Just h
            else
                nth (n - 1) t


-- | Take the first n elements of a list.
--
--     take 2 [1, 2, 3, 4] == [1, 2]
--
let rec take n lst =
    if n == 0 then
        []
    else
        case lst of
            [] ->
                []
            [h|t] ->
                h :: take (n - 1) t


-- | Drop the first n elements of a list.
--
--     drop 2 [1, 2, 3, 4] == [3, 4]
--
let rec drop n lst =
    if n == 0 then
        lst
    else
        case lst of
            [] ->
                []
            [_|t] ->
                drop (n - 1) t


-- | Find the first element satisfying a predicate, or Nothing.
--
--     find (fn x -> x > 2) [1, 2, 3, 4] == Just 3
--
let rec find pred lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            if pred h then
                Just h
            else
                find pred t


-- | Split a list into elements that pass and fail a predicate.
--
--     partition (fn x -> x > 2) [1, 2, 3, 4] == ([3, 4], [1, 2])
--
let partition pred lst =
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


-- # Aggregate


-- | Sum all numbers in a list.
--
--     sum [1, 2, 3, 4, 5] == 15
--
let sum lst =
    foldl (fn acc x -> acc + x) 0 lst


-- | Multiply all numbers in a list.
--
--     product [1, 2, 3, 4] == 24
--
let product lst =
    foldl (fn acc x -> acc * x) 1 lst


-- | Determine if any elements satisfy the predicate.
--
--     any (fn x -> x > 3) [1, 2, 3, 4] == true
--
let any pred lst =
    foldl (fn acc x -> acc || pred x) false lst


-- | Determine if all elements satisfy the predicate.
--
--     all (fn x -> x > 0) [1, 2, 3] == true
--
let all pred lst =
    foldl (fn acc x -> acc && pred x) true lst


-- | Return the largest element, or Nothing for empty lists. Requires Ord.
--
--     maximum [3, 1, 4, 1, 5] == Just 5
--
let maximum lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            Just (foldl (fn a b ->
                case compare a b of
                    GT ->
                        a
                    _ ->
                        b) h t)


-- | Return the smallest element, or Nothing for empty lists. Requires Ord.
--
--     minimum [3, 1, 4, 1, 5] == Just 1
--
let minimum lst =
    case lst of
        [] ->
            Nothing
        [h|t] ->
            Just (foldl (fn a b ->
                case compare a b of
                    LT ->
                        a
                    _ ->
                        b) h t)


-- # Sort


-- | Sort a list using the default compare function.
--
--     sort [3, 1, 2] == [1, 2, 3]
--
let sort lst =
    sortWith (fn a b -> compare a b) lst


-- | Sort a list by a derived key.
--
--     sortBy (fn x -> 0 - x) [3, 1, 2] == [3, 2, 1]
--
let sortBy f lst =
    sortWith (fn a b -> compare (f a) (f b)) lst


-- | Take elements while predicate holds.
--
--     takeWhile (fn x -> x < 3) [1, 2, 3, 4] == [1, 2]
--
let rec takeWhile pred lst =
    case lst of
        [] ->
            []
        [h|t] ->
            if pred h then
                h :: takeWhile pred t
            else
                []


-- | Drop elements while predicate holds.
--
--     dropWhile (fn x -> x < 3) [1, 2, 3, 4] == [3, 4]
--
let rec dropWhile pred lst =
    case lst of
        [] ->
            []
        [h|t] ->
            if pred h then
                dropWhile pred t
            else
                lst


-- | Split a list at the point where the predicate stops holding.
--
--     span (fn x -> x < 3) [1, 2, 3, 4] == ([1, 2], [3, 4])
--
let span pred lst =
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


-- | Remove duplicate elements (O(n²), uses ==).
--
--     unique [1, 2, 1, 3, 2] == [1, 2, 3]
--
let unique lst =
    let rec go acc xs =
        case xs of
            [] ->
                reverse acc
            [h|t] ->
                if any (fn x -> x == h) acc then
                    go acc t
                else
                    go (h :: acc) t
    in
    go [] lst


-- | Remove duplicates by a key function (O(n²), uses == on keys).
-- Keeps the first element for each distinct key.
--
--     uniqueBy (fn x -> x % 3) [1, 2, 3, 4, 5] == [1, 2, 3]
--
let uniqueBy f lst =
    let rec go seen acc xs =
        case xs of
            [] ->
                reverse acc
            [h|t] ->
                let key = f h in
                if any (fn k -> k == key) seen then
                    go seen acc t
                else
                    go (key :: seen) (h :: acc) t
    in
    go [] [] lst


-- | Map with a function returning Maybe, keeping only Just values.
--
--     filterMap (fn x -> if x > 2 then Just (x * 10) else Nothing) [1, 2, 3, 4] == [30, 40]
--
let rec filterMap f lst =
    case lst of
        [] ->
            []
        [h|t] ->
            case f h of
                Just v ->
                    v :: filterMap f t
                Nothing ->
                    filterMap f t


-- | Combine two lists element-wise with a function.
--
--     zipWith (fn a b -> a + b) [1, 2, 3] [10, 20, 30] == [11, 22, 33]
--
let rec zipWith f xs ys =
    case xs of
        [] ->
            []
        [x|xrest] ->
            case ys of
                [] ->
                    []
                [y|yrest] ->
                    f x y :: zipWith f xrest yrest


-- | Split a list of pairs into a pair of lists.
--
--     unzip [(1, "a"), (2, "b")] == ([1, 2], ["a", "b"])
--
let unzip pairs =
    let rec go xs ys zs =
        case zs of
            [] ->
                (reverse xs, reverse ys)
            [h|t] ->
                let (a, b) = h in
                go (a :: xs) (b :: ys) t
    in
    go [] [] pairs


-- | Reduce a list using the first element as the initial accumulator.
-- Crashes on empty list.
--
--     foldl1 (fn a b -> a + b) [1, 2, 3] == 6
--
let foldl1 f lst =
    case lst of
        [] ->
            error "foldl1: empty list"
        [h|t] ->
            foldl f h t


-- # Tests


test "length and isEmpty" =
    assert (length [] == 0)
    assert (length [1, 2, 3] == 3)
    assert (isEmpty [])
    assert (not (isEmpty [1]))

test "repeat and range" =
    assert (sum (repeat 5 1) == 5)
    assert (length (repeat 0 99) == 0)
    assert (sum (range 1 6) == 15)
    assert (length (range 3 3) == 0)

test "head and tail" =
    assert (head [1, 2, 3] == 1)
    assert (head (tail [1, 2, 3]) == 2)

test "last and init" =
    assert (last [1, 2, 3] == 3)
    assert (length (init [1, 2, 3]) == 2)
    assert (last (init [1, 2, 3]) == 2)

test "nth" =
    assert (nth 0 [10, 20, 30] == Just 10)
    assert (nth 2 [10, 20, 30] == Just 30)
    assert (nth 5 [10, 20, 30] == Nothing)

test "map and filter" =
    assert (sum (map (fn x -> x * 2) [1, 2, 3]) == 12)
    assert (length (filter (fn x -> x > 2) [1, 2, 3, 4, 5]) == 3)

test "indexedMap" =
    assert (sum (indexedMap (fn i x -> i) [10, 20, 30]) == 3)

test "foldl and foldr" =
    assert (foldl (fn acc x -> acc + x) 0 [1, 2, 3] == 6)
    assert (head (foldr (fn x acc -> x :: acc) [] [1, 2, 3]) == 1)

test "append and reverse" =
    assert (length (append [1, 2] [3, 4]) == 4)
    assert (sum (append [1, 2] [3, 4]) == 10)
    assert (head (reverse [1, 2, 3]) == 3)

test "take and drop" =
    assert (sum (take 2 [1, 2, 3, 4]) == 3)
    assert (sum (drop 2 [1, 2, 3, 4]) == 7)

test "zip" =
    let pairs = zip [1, 2, 3] [4, 5, 6]
    assert (length pairs == 3)
    assert (foldl (fn acc pair -> let (a, b) = pair in acc + a + b) 0 pairs == 21)

test "concat and flatMap" =
    assert (sum (concat [[1, 2], [3], [4, 5]]) == 15)
    assert (length (flatMap (fn x -> [x, x]) [1, 2, 3]) == 6)

test "intersperse" =
    assert (sum (intersperse 0 [1, 2, 3]) == 6)
    assert (length (intersperse 0 [1, 2, 3]) == 5)

test "find and partition" =
    assert (find (fn x -> x > 2) [1, 2, 3, 4] == Just 3)
    assert (find (fn x -> x > 10) [1, 2, 3] == Nothing)
    let (yes, no) = partition (fn x -> x > 2) [1, 2, 3, 4]
    assert (sum yes == 7)
    assert (sum no == 3)

test "sum, product, any, all" =
    assert (sum [1, 2, 3, 4, 5] == 15)
    assert (product [1, 2, 3, 4] == 24)
    assert (any (fn x -> x > 3) [1, 2, 3, 4])
    assert (not (any (fn x -> x > 10) [1, 2, 3]))
    assert (all (fn x -> x > 0) [1, 2, 3])
    assert (not (all (fn x -> x > 2) [1, 2, 3]))

test "maximum and minimum" =
    assert (maximum [3, 1, 4, 1, 5] == Just 5)
    assert (minimum [3, 1, 4, 1, 5] == Just 1)
    assert (maximum [] == Nothing)
    assert (minimum [] == Nothing)

test "flatMap" =
    assert (flatMap (fn x -> [x, x]) [1, 2, 3] == [1, 1, 2, 2, 3, 3])
    assert (flatMap (fn x -> []) [1, 2, 3] == [])

test "sort and sortBy" =
    assert (sort [3, 1, 4, 1, 5] == [1, 1, 3, 4, 5])
    assert (sort [] == [])
    assert (sortBy (fn x -> 0 - x) [3, 1, 2] == [3, 2, 1])

test "sortWith" =
    assert (sortWith (fn a b -> compare a b) [5, 3, 1, 4, 2] == [1, 2, 3, 4, 5])
    assert (sortWith (fn a b -> compare b a) [1, 2, 3] == [3, 2, 1])

test "takeWhile and dropWhile" =
    assert (takeWhile (fn x -> x < 3) [1, 2, 3, 4] == [1, 2])
    assert (takeWhile (fn x -> x < 0) [1, 2, 3] == [])
    assert (dropWhile (fn x -> x < 3) [1, 2, 3, 4] == [3, 4])
    assert (dropWhile (fn x -> x < 0) [1, 2, 3] == [1, 2, 3])

test "span" =
    let (a, b) = span (fn x -> x < 3) [1, 2, 3, 4]
    assert (a == [1, 2])
    assert (b == [3, 4])

test "unique" =
    assert (unique [1, 2, 1, 3, 2] == [1, 2, 3])
    assert (unique [] == [])
    assert (unique [1, 1, 1] == [1])

test "uniqueBy" =
    assert (uniqueBy (fn x -> x % 3) [1, 2, 3, 4, 5] == [1, 2, 3])
    assert (uniqueBy (fn x -> x) [1, 2, 1] == [1, 2])
    assert (uniqueBy (fn x -> 0) [1, 2, 3] == [1])

test "filterMap" =
    assert (filterMap (fn x -> if x > 2 then Just (x * 10) else Nothing) [1, 2, 3, 4] == [30, 40])
    assert (filterMap (fn x -> Nothing) [1, 2, 3] == [])

test "zipWith" =
    assert (zipWith (fn a b -> a + b) [1, 2, 3] [10, 20, 30] == [11, 22, 33])
    assert (zipWith (fn a b -> a + b) [1, 2] [10] == [11])
    assert (zipWith (fn a b -> a + b) [] [1, 2] == [])

test "unzip" =
    let (xs, ys) = unzip [(1, "a"), (2, "b"), (3, "c")]
    assert (xs == [1, 2, 3])
    assert (ys == ["a", "b", "c"])

test "foldl1" =
    assert (foldl1 (fn a b -> a + b) [1, 2, 3] == 6)
    assert (foldl1 (fn a b -> a + b) [42] == 42)
