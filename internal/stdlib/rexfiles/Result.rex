export Ok, Err, isOk, isErr, withDefault, map, mapErr, andThen, toMaybe, fromMaybe


type Result a e = Ok a | Err e


-- # Query


-- | Return true if the result is Ok.
--
--     isOk (Ok 42) == true
--     isOk (Err "oops") == false
--
isOk : Result a e -> Bool
let isOk r =
    case r of
        Ok _ ->
            true
        Err _ ->
            false

test "isOk" =
    assert (isOk (Ok 42))
    assert (Err "oops" |> isOk |> not)


-- | Return true if the result is Err.
--
--     isErr (Err "oops") == true
--     isErr (Ok 42) == false
--
isErr : Result a e -> Bool
let isErr r =
    case r of
        Ok _ ->
            false
        Err _ ->
            true

test "isErr" =
    assert (isErr (Err "oops"))
    assert (Ok 42 |> isErr |> not)


-- # Extract


-- | Extract the Ok value or return a default.
--
--     withDefault 0 (Ok 42) == 42
--     withDefault 0 (Err "oops") == 0
--
withDefault : a -> Result a e -> a
let withDefault default r =
    case r of
        Ok x ->
            x
        Err _ ->
            default

test "withDefault" =
    assert (withDefault 0 (Ok 42) == 42)
    assert (withDefault 0 (Err "oops") == 0)


-- # Transform


-- | Apply a function to the Ok value; pass Err through unchanged.
--
--     map (fn x -> x * 2) (Ok 5) == Ok 10
--     map (fn x -> x * 2) (Err "oops") == Err "oops"
--
map : (a -> b) -> Result a e -> Result b e
let map f r =
    case r of
        Ok x ->
            Ok (f x)
        Err e ->
            Err e

test "map" =
    assert (Ok 5 |> map (fn x -> x * 2) |> withDefault 0 == 10)
    assert (Err "oops" |> map (fn x -> x * 2) |> isErr)


-- | Apply a function to the Err value; pass Ok through unchanged.
--
--     mapErr (fn e -> "error: " ++ e) (Err "oops") == Err "error: oops"
--     mapErr (fn e -> "error: " ++ e) (Ok 5) == Ok 5
--
mapErr : (e -> f) -> Result a e -> Result a f
let mapErr f r =
    case r of
        Ok x ->
            Ok x
        Err e ->
            Err (f e)

test "mapErr" =
    assert (Ok 5 |> mapErr (fn e -> e ++ "!") |> isOk)
    assert (Err "oops" |> mapErr (fn e -> e ++ "!") |> isErr)


-- | Chain Result-returning functions (flatMap/bind).
--
--     andThen (fn x -> Ok (x * 2)) (Ok 5) == Ok 10
--     andThen (fn x -> Ok (x * 2)) (Err "oops") == Err "oops"
--     andThen (fn x -> Err "nope") (Ok 5) == Err "nope"
--
andThen : (a -> Result b e) -> Result a e -> Result b e
let andThen f r =
    case r of
        Ok x ->
            f x
        Err e ->
            Err e

test "andThen" =
    assert (Ok 5 |> andThen (fn x -> Ok (x * 2)) |> withDefault 0 == 10)
    assert (Err "oops" |> andThen (fn x -> Ok (x * 2)) |> isErr)
    assert (Ok 5 |> andThen (fn _ -> Err "nope") |> isErr)


-- # Convert


-- | Convert Result to Maybe, discarding the error.
--
--     toMaybe (Ok 42) == Just 42
--     toMaybe (Err "oops") == Nothing
--
toMaybe : Result a e -> Maybe a
let toMaybe r =
    case r of
        Ok x ->
            Just x
        Err _ ->
            Nothing

test "toMaybe" =
    assert (Ok 42 |> toMaybe == Just 42)
    assert (Err "oops" |> toMaybe == Nothing)


-- | Convert Maybe to Result with a default error.
--
--     fromMaybe "missing" (Just 42) == Ok 42
--     fromMaybe "missing" Nothing == Err "missing"
--
fromMaybe : e -> Maybe a -> Result a e
let fromMaybe err m =
    case m of
        Just x ->
            Ok x
        Nothing ->
            Err err

test "fromMaybe" =
    assert (fromMaybe "missing" (Just 42) == Ok 42)
    assert (fromMaybe "missing" Nothing == Err "missing")
