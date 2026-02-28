export Ok, Err, isOk, isErr, withDefault, map, mapErr, andThen


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
--     map (fun x -> x * 2) (Ok 5) == Ok 10
--     map (fun x -> x * 2) (Err "oops") == Err "oops"
--
let map f r =
    case r of
        Ok x ->
            Ok (f x)
        Err e ->
            Err e


-- | Apply a function to the Err value; pass Ok through unchanged.
--
--     mapErr (fun e -> "error: " ++ e) (Err "oops") == Err "error: oops"
--     mapErr (fun e -> "error: " ++ e) (Ok 5) == Ok 5
--
let mapErr f r =
    case r of
        Ok x ->
            Ok x
        Err e ->
            Err (f e)


-- | Chain Result-returning functions (flatMap/bind).
--
--     andThen (fun x -> Ok (x * 2)) (Ok 5) == Ok 10
--     andThen (fun x -> Ok (x * 2)) (Err "oops") == Err "oops"
--     andThen (fun x -> Err "nope") (Ok 5) == Err "nope"
--
let andThen f r =
    case r of
        Ok x ->
            f x
        Err e ->
            Err e
