type Maybe a = Nothing | Just a

let isNothing x =
    case x of
        Nothing ->
            true
        Just _ ->
            false

let fromMaybe default x =
    case x of
        Nothing ->
            default
        Just v ->
            v

print (fromMaybe 0 (Just 7))
