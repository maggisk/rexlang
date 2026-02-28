export Nothing, Just, isNothing, isSome, fromMaybe, map


type Maybe a = Nothing | Just a


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


-- # Transform


-- | Apply a function to the value inside Just, pass Nothing through.
--
--     map (\x -> x * 2) (Just 5) == Just 10
--     map (\x -> x * 2) Nothing == Nothing
--
let map f x =
    case x of
        Nothing ->
            Nothing
        Just v ->
            Just (f v)
