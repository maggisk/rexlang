export length, append, reverse, map, filter, foldl, foldr, head, tail, take, drop, sum, product, any, all


-- # Create


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


-- # Transform


-- | Apply a function to every element of a list.
--
--     map (\x -> x * 2) [1, 2, 3] == [2, 4, 6]
--
let rec map f lst =
    case lst of
        [] ->
            []
        [h|t] ->
            f h :: map f t


-- | Keep only elements that satisfy the predicate.
--
--     filter (\x -> x > 2) [1, 2, 3, 4] == [3, 4]
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


-- # Fold


-- | Reduce a list from the left.
--
--     foldl (\acc x -> acc + x) 0 [1, 2, 3] == 6
--
let rec foldl f acc lst =
    case lst of
        [] ->
            acc
        [h|t] ->
            foldl f (f acc h) t


-- | Reduce a list from the right.
--
--     foldr (\x acc -> x :: acc) [] [1, 2, 3] == [1, 2, 3]
--
let rec foldr f acc lst =
    case lst of
        [] ->
            acc
        [h|t] ->
            f h (foldr f acc t)


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


-- # Aggregate


-- | Sum all numbers in a list.
--
--     sum [1, 2, 3, 4, 5] == 15
--
let sum lst =
    foldl (fun acc x -> acc + x) 0 lst


-- | Multiply all numbers in a list.
--
--     product [1, 2, 3, 4] == 24
--
let product lst =
    foldl (fun acc x -> acc * x) 1 lst


-- | Determine if any elements satisfy the predicate.
--
--     any (\x -> x > 3) [1, 2, 3, 4] == true
--
let any pred lst =
    foldl (fun acc x -> acc || pred x) false lst


-- | Determine if all elements satisfy the predicate.
--
--     all (\x -> x > 0) [1, 2, 3] == true
--
let all pred lst =
    foldl (fun acc x -> acc && pred x) true lst
