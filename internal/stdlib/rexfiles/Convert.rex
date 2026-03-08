import Std:Maybe (Just, Nothing)
import Std:Result (Ok, Err)


-- | Convert Maybe to Result with an error for Nothing.
--
--     toResult "missing" (Just 5) == Ok 5
--     toResult "missing" Nothing == Err "missing"
--
export
toResult : e -> Maybe a -> Result a e
toResult err x =
    match x
        when Just v ->
            Ok v
        when Nothing ->
            Err err

test "toResult" =
    assert (Just 5 |> toResult "missing" == Ok 5)
    assert (Nothing |> toResult "missing" == Err "missing")


-- | Convert Result to Maybe, discarding the error.
--
--     toMaybe (Ok 42) == Just 42
--     toMaybe (Err "oops") == Nothing
--
export
toMaybe : Result a e -> Maybe a
toMaybe r =
    match r
        when Ok x ->
            Just x
        when Err _ ->
            Nothing

test "toMaybe" =
    assert (Ok 42 |> toMaybe == Just 42)
    assert (Err "oops" |> toMaybe == Nothing)


-- | Convert Maybe to Result with a default error.
--
--     fromMaybe "missing" (Just 42) == Ok 42
--     fromMaybe "missing" Nothing == Err "missing"
--
export
fromMaybe : e -> Maybe a -> Result a e
fromMaybe err m =
    match m
        when Just x ->
            Ok x
        when Nothing ->
            Err err

test "fromMaybe" =
    assert (fromMaybe "missing" (Just 42) == Ok 42)
    assert (fromMaybe "missing" Nothing == Err "missing")
