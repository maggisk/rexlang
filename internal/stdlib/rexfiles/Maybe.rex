import Std:Result (Ok, Err)

export type Maybe a = Nothing | Just a


-- # Query


-- | Return true if the value is Nothing.
--
--     isNothing Nothing == true
--     isNothing (Just 5) == false
--
export
isNothing : Maybe a -> Bool
isNothing x =
    case x of
        Nothing ->
            true
        Just _ ->
            false

test "isNothing" =
    assert (isNothing Nothing)
    assert (Just 5 |> isNothing |> not)


-- | Return true if the value is Just.
--
--     isSome (Just 5) == true
--     isSome Nothing == false
--
export
isSome : Maybe a -> Bool
isSome x =
    case x of
        Nothing ->
            false
        Just _ ->
            true

test "isSome" =
    assert (isSome (Just 5))
    assert (Nothing |> isSome |> not)


-- # Extract


-- | Extract the value or return a default.
--
--     fromMaybe 0 (Just 7) == 7
--     fromMaybe 0 Nothing == 0
--
export
fromMaybe : a -> Maybe a -> a
fromMaybe default x =
    case x of
        Nothing ->
            default
        Just v ->
            v

test "fromMaybe" =
    assert (Just 7 |> fromMaybe 0 == 7)
    assert (fromMaybe 0 Nothing == 0)


-- | Alias for fromMaybe (Elm naming).
--
--     withDefault 0 (Just 7) == 7
--     withDefault 0 Nothing == 0
--
export
withDefault : a -> Maybe a -> a
withDefault = fromMaybe

test "withDefault" =
    assert (Just 7 |> withDefault 0 == 7)
    assert (withDefault 0 Nothing == 0)


-- # Transform


-- | Apply a function to the value inside Just, pass Nothing through.
--
--     map (\x -> x * 2) (Just 5) == Just 10
--     map (\x -> x * 2) Nothing == Nothing
--
export
map : (a -> b) -> Maybe a -> Maybe b
map f x =
    case x of
        Nothing ->
            Nothing
        Just v ->
            Just (f v)

test "map" =
    assert (Just 5 |> map (\x -> x * 2) == Just 10)
    assert (map (\x -> x * 2) Nothing == Nothing)


-- | Chain Maybe-returning functions (flatMap/bind).
--   The function receives the unwrapped value and returns a Maybe.
--
--     andThen (\x -> Just (x * 2)) (Just 5) == Just 10
--     andThen (\x -> Nothing) (Just 5) == Nothing
--     andThen (\x -> Just (x * 2)) Nothing == Nothing
--
export
andThen : (a -> Maybe b) -> Maybe a -> Maybe b
andThen f x =
    case x of
        Nothing ->
            Nothing
        Just v ->
            f v

test "andThen" =
    assert (Just 5 |> andThen (\x -> Just (x * 2)) == Just 10)
    assert (Just 5 |> andThen (\x -> Nothing) == Nothing)
    assert (andThen (\x -> Just (x * 2)) Nothing == Nothing)


-- | Keep Just if predicate holds, otherwise Nothing.
--
--     filter (\x -> x > 3) (Just 5) == Just 5
--     filter (\x -> x > 3) (Just 1) == Nothing
--     filter (\x -> x > 3) Nothing == Nothing
--
export
filter : (a -> Bool) -> Maybe a -> Maybe a
filter pred x =
    case x of
        Nothing ->
            Nothing
        Just v ->
            if pred v then
                Just v
            else
                Nothing

test "filter" =
    assert (Just 5 |> filter (\x -> x > 3) == Just 5)
    assert (Just 1 |> filter (\x -> x > 3) == Nothing)
    assert (filter (\x -> x > 3) Nothing == Nothing)


-- | Return the first Just, or Nothing if both are Nothing.
--
--     orElse (Just 1) (Just 2) == Just 1
--     orElse Nothing (Just 2) == Just 2
--     orElse Nothing Nothing == Nothing
--
export
orElse : Maybe a -> Maybe a -> Maybe a
orElse a b =
    case a of
        Just _ ->
            a
        Nothing ->
            b

test "orElse" =
    assert (orElse (Just 1) (Just 2) == Just 1)
    assert (orElse Nothing (Just 2) == Just 2)
    assert (orElse Nothing Nothing == Nothing)
