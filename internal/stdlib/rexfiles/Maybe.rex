export Nothing, Just, isNothing, isSome, fromMaybe, map, andThen, withDefault, filter, orElse, toResult


type Maybe a = Nothing | Just a

import std:Result (Ok, Err)


-- # Query


-- | Return true if the value is Nothing.
--
--     isNothing Nothing == true
--     isNothing (Just 5) == false
--
let isNothing x =
    case x of
        Nothing ->
            true
        Just _ ->
            false

test "isNothing" =
    assert (isNothing Nothing)
    assert (not (isNothing (Just 5)))


-- | Return true if the value is Just.
--
--     isSome (Just 5) == true
--     isSome Nothing == false
--
let isSome x =
    case x of
        Nothing ->
            false
        Just _ ->
            true

test "isSome" =
    assert (isSome (Just 5))
    assert (not (isSome Nothing))


-- # Extract


-- | Extract the value or return a default.
--
--     fromMaybe 0 (Just 7) == 7
--     fromMaybe 0 Nothing == 0
--
let fromMaybe default x =
    case x of
        Nothing ->
            default
        Just v ->
            v

test "fromMaybe" =
    assert (fromMaybe 0 (Just 7) == 7)
    assert (fromMaybe 0 Nothing == 0)


-- | Alias for fromMaybe (Elm naming).
--
--     withDefault 0 (Just 7) == 7
--     withDefault 0 Nothing == 0
--
let withDefault = fromMaybe

test "withDefault" =
    assert (withDefault 0 (Just 7) == 7)
    assert (withDefault 0 Nothing == 0)


-- # Transform


-- | Apply a function to the value inside Just, pass Nothing through.
--
--     map (fn x -> x * 2) (Just 5) == Just 10
--     map (fn x -> x * 2) Nothing == Nothing
--
let map f x =
    case x of
        Nothing ->
            Nothing
        Just v ->
            Just (f v)

test "map" =
    assert (map (fn x -> x * 2) (Just 5) == Just 10)
    assert (map (fn x -> x * 2) Nothing == Nothing)


-- | Chain Maybe-returning functions (flatMap/bind).
--   The function receives the unwrapped value and returns a Maybe.
--
--     andThen (fn x -> Just (x * 2)) (Just 5) == Just 10
--     andThen (fn x -> Nothing) (Just 5) == Nothing
--     andThen (fn x -> Just (x * 2)) Nothing == Nothing
--
let andThen f x =
    case x of
        Nothing ->
            Nothing
        Just v ->
            f v

test "andThen" =
    assert (andThen (fn x -> Just (x * 2)) (Just 5) == Just 10)
    assert (andThen (fn x -> Nothing) (Just 5) == Nothing)
    assert (andThen (fn x -> Just (x * 2)) Nothing == Nothing)


-- | Keep Just if predicate holds, otherwise Nothing.
--
--     filter (fn x -> x > 3) (Just 5) == Just 5
--     filter (fn x -> x > 3) (Just 1) == Nothing
--     filter (fn x -> x > 3) Nothing == Nothing
--
let filter pred x =
    case x of
        Nothing ->
            Nothing
        Just v ->
            if pred v then
                Just v
            else
                Nothing

test "filter" =
    assert (filter (fn x -> x > 3) (Just 5) == Just 5)
    assert (filter (fn x -> x > 3) (Just 1) == Nothing)
    assert (filter (fn x -> x > 3) Nothing == Nothing)


-- | Return the first Just, or Nothing if both are Nothing.
--
--     orElse (Just 1) (Just 2) == Just 1
--     orElse Nothing (Just 2) == Just 2
--     orElse Nothing Nothing == Nothing
--
let orElse a b =
    case a of
        Just _ ->
            a
        Nothing ->
            b

test "orElse" =
    assert (orElse (Just 1) (Just 2) == Just 1)
    assert (orElse Nothing (Just 2) == Just 2)
    assert (orElse Nothing Nothing == Nothing)


-- # Convert


-- | Convert Maybe to Result with an error for Nothing.
--
--     toResult "missing" (Just 5) == Ok 5
--     toResult "missing" Nothing == Err "missing"
--
let toResult err x =
    case x of
        Just v ->
            Ok v
        Nothing ->
            Err err

test "toResult" =
    assert (toResult "missing" (Just 5) == Ok 5)
    assert (toResult "missing" Nothing == Err "missing")
