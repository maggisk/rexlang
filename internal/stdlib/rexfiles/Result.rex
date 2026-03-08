export type Result a e = Ok a | Err e

export type RuntimeError = DivisionByZero | ModuloByZero


-- # Recovery


export try


-- # Query


-- | Return true if the result is Ok.
--
--     isOk (Ok 42) == true
--     isOk (Err "oops") == false
--
export
isOk : Result a e -> Bool
isOk r =
    match r
        when Ok _ ->
            true
        when Err _ ->
            false

test "isOk" =
    assert (isOk (Ok 42))
    assert (Err "oops" |> isOk |> not)


-- | Return true if the result is Err.
--
--     isErr (Err "oops") == true
--     isErr (Ok 42) == false
--
export
isErr : Result a e -> Bool
isErr r =
    match r
        when Ok _ ->
            false
        when Err _ ->
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
export
withDefault : a -> Result a e -> a
withDefault default r =
    match r
        when Ok x ->
            x
        when Err _ ->
            default

test "withDefault" =
    assert (withDefault 0 (Ok 42) == 42)
    assert (withDefault 0 (Err "oops") == 0)


-- # Transform


-- | Apply a function to the Ok value; pass Err through unchanged.
--
--     map (\x -> x * 2) (Ok 5) == Ok 10
--     map (\x -> x * 2) (Err "oops") == Err "oops"
--
export
map : (a -> b) -> Result a e -> Result b e
map f r =
    match r
        when Ok x ->
            Ok (f x)
        when Err e ->
            Err e

test "map" =
    assert (Ok 5 |> map (\x -> x * 2) |> withDefault 0 == 10)
    assert (Err "oops" |> map (\x -> x * 2) |> isErr)


-- | Apply a function to the Err value; pass Ok through unchanged.
--
--     mapErr (\e -> "error: " ++ e) (Err "oops") == Err "error: oops"
--     mapErr (\e -> "error: " ++ e) (Ok 5) == Ok 5
--
export
mapErr : (e -> f) -> Result a e -> Result a f
mapErr f r =
    match r
        when Ok x ->
            Ok x
        when Err e ->
            Err (f e)

test "mapErr" =
    assert (Ok 5 |> mapErr (\e -> e ++ "!") |> isOk)
    assert (Err "oops" |> mapErr (\e -> e ++ "!") |> isErr)


-- | Chain Result-returning functions (flatMap/bind).
--
--     andThen (\x -> Ok (x * 2)) (Ok 5) == Ok 10
--     andThen (\x -> Ok (x * 2)) (Err "oops") == Err "oops"
--     andThen (\x -> Err "nope") (Ok 5) == Err "nope"
--
export
andThen : (a -> Result b e) -> Result a e -> Result b e
andThen f r =
    match r
        when Ok x ->
            f x
        when Err e ->
            Err e

test "andThen" =
    assert (Ok 5 |> andThen (\x -> Ok (x * 2)) |> withDefault 0 == 10)
    assert (Err "oops" |> andThen (\x -> Ok (x * 2)) |> isErr)
    assert (Ok 5 |> andThen (\_ -> Err "nope") |> isErr)


test "try catches division by zero" =
    assert (try (\_ -> 10 / 0) == Err DivisionByZero)

test "try catches modulo by zero" =
    assert (try (\_ -> 10 % 0) == Err ModuloByZero)

test "try returns Ok on success" =
    assert (try (\_ -> 10 / 2) == Ok 5)
