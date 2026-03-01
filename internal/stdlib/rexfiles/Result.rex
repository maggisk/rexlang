export Ok, Err, isOk, isErr, withDefault, map, mapErr, andThen, toMaybe, fromMaybe


type Result a e = Ok a | Err e


-- # Query


-- | Return true if the result is Ok.
--
--     isOk (Ok 42) == true
--     isOk (Err "oops") == false
--
let isOk r =
    case r of
        Ok _ ->
            true
        Err _ ->
            false


-- | Return true if the result is Err.
--
--     isErr (Err "oops") == true
--     isErr (Ok 42) == false
--
let isErr r =
    case r of
        Ok _ ->
            false
        Err _ ->
            true


-- # Extract


-- | Extract the Ok value or return a default.
--
--     withDefault 0 (Ok 42) == 42
--     withDefault 0 (Err "oops") == 0
--
let withDefault default r =
    case r of
        Ok x ->
            x
        Err _ ->
            default


-- # Transform


-- | Apply a function to the Ok value; pass Err through unchanged.
--
--     map (fn x -> x * 2) (Ok 5) == Ok 10
--     map (fn x -> x * 2) (Err "oops") == Err "oops"
--
let map f r =
    case r of
        Ok x ->
            Ok (f x)
        Err e ->
            Err e


-- | Apply a function to the Err value; pass Ok through unchanged.
--
--     mapErr (fn e -> "error: " ++ e) (Err "oops") == Err "error: oops"
--     mapErr (fn e -> "error: " ++ e) (Ok 5) == Ok 5
--
let mapErr f r =
    case r of
        Ok x ->
            Ok x
        Err e ->
            Err (f e)


-- | Chain Result-returning functions (flatMap/bind).
--
--     andThen (fn x -> Ok (x * 2)) (Ok 5) == Ok 10
--     andThen (fn x -> Ok (x * 2)) (Err "oops") == Err "oops"
--     andThen (fn x -> Err "nope") (Ok 5) == Err "nope"
--
let andThen f r =
    case r of
        Ok x ->
            f x
        Err e ->
            Err e


-- | Convert Result to Maybe, discarding the error.
--
--     toMaybe (Ok 42) == Just 42
--     toMaybe (Err "oops") == Nothing
--
let toMaybe r =
    case r of
        Ok x ->
            Just x
        Err _ ->
            Nothing


-- | Convert Maybe to Result with a default error.
--
--     fromMaybe "missing" (Just 42) == Ok 42
--     fromMaybe "missing" Nothing == Err "missing"
--
let fromMaybe err m =
    case m of
        Just x ->
            Ok x
        Nothing ->
            Err err


-- # Tests


test "isOk and isErr" =
    assert (isOk (Ok 42))
    assert (not (isOk (Err "oops")))
    assert (isErr (Err "oops"))
    assert (not (isErr (Ok 42)))

test "withDefault" =
    assert (withDefault 0 (Ok 42) == 42)
    assert (withDefault 0 (Err "oops") == 0)

test "map" =
    assert (withDefault 0 (map (fn x -> x * 2) (Ok 5)) == 10)
    assert (isErr (map (fn x -> x * 2) (Err "oops")))

test "mapErr" =
    assert (isOk (mapErr (fn e -> e ++ "!") (Ok 5)))
    assert (isErr (mapErr (fn e -> e ++ "!") (Err "oops")))

test "andThen" =
    assert (withDefault 0 (andThen (fn x -> Ok (x * 2)) (Ok 5)) == 10)
    assert (isErr (andThen (fn x -> Ok (x * 2)) (Err "oops")))
    assert (isErr (andThen (fn _ -> Err "nope") (Ok 5)))

test "toMaybe" =
    assert (toMaybe (Ok 42) == Just 42)
    assert (toMaybe (Err "oops") == Nothing)

test "fromMaybe" =
    assert (fromMaybe "missing" (Just 42) == Ok 42)
    assert (fromMaybe "missing" Nothing == Err "missing")

