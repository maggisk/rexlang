import Std:Json (JNull, JBool, JStr, JNum, JArr, JObj, ObjNil, ObjCons, parse, getField, arrayToList)
import Std:Result (Ok, Err)
import Std:Math (round, toFloat)
import Std:Map as Map


-- # Types


-- | A decoder that extracts a value of type `a` from JSON.
export type Decoder a = | Decoder (Json -> Result a String)


-- # Runners


-- | Run a decoder on a Json value.
run : Decoder a -> Json -> Result a String
export let run decoder json =
    case decoder of
        Decoder f ->
            f json


-- | Parse a JSON string and run a decoder on the result.
decodeString : Decoder a -> String -> Result a String
export let decodeString decoder str =
    case parse str of
        Err e ->
            Err e
        Ok json ->
            run decoder json


-- # Base decoders


-- | Decode a JSON string.
string : Decoder String
export let string =
    Decoder (\json ->
        case json of
            JStr s ->
                Ok s
            _ ->
                Err "expected a String")


-- | Decode a JSON integer.
int : Decoder Int
export let int =
    Decoder (\json ->
        case json of
            JNum n ->
                if toFloat (round n) == n then
                    Ok (round n)
                else
                    Err "expected an Int but got a Float"
            _ ->
                Err "expected an Int")


-- | Decode a JSON float.
float : Decoder Float
export let float =
    Decoder (\json ->
        case json of
            JNum n ->
                Ok n
            _ ->
                Err "expected a Float")


-- | Decode a JSON boolean.
bool : Decoder Bool
export let bool =
    Decoder (\json ->
        case json of
            JBool b ->
                Ok b
            _ ->
                Err "expected a Bool")


-- | Decode a JSON null, succeeding with the given default value.
null : a -> Decoder a
export let null default =
    Decoder (\json ->
        case json of
            JNull ->
                Ok default
            _ ->
                Err "expected null")


-- # Object decoders


-- | Decode a field from a JSON object.
field : String -> Decoder a -> Decoder a
export let field key decoder =
    Decoder (\json ->
        case json of
            JObj obj ->
                case getField key obj of
                    Just val ->
                        run decoder val
                    Nothing ->
                        Err ("field '" ++ key ++ "' not found")
            _ ->
                Err "expected an Object")


-- | Decode a value at a nested path in a JSON object.
at : [String] -> Decoder a -> Decoder a
export let rec at keys decoder =
    case keys of
        [] ->
            decoder
        [k|rest] ->
            field k (at rest decoder)


-- # Array decoders


-- | Decode an element at a given index in a JSON array.
index : Int -> Decoder a -> Decoder a
export let index i decoder =
    let rec nth n lst =
        case lst of
            [] ->
                Err ("index " ++ show i ++ " out of range")
            [h|t] ->
                if n == 0 then
                    run decoder h
                else
                    nth (n - 1) t
    in
    Decoder (\json ->
        case json of
            JArr arr ->
                nth i (arrayToList arr)
            _ ->
                Err "expected an Array")


-- | Decode a JSON array, applying the given decoder to each element.
list : Decoder a -> Decoder [a]
export let list decoder =
    let rec decodeAll items =
        case items of
            [] ->
                Ok []
            [h|t] ->
                case run decoder h of
                    Err e ->
                        Err e
                    Ok val ->
                        case decodeAll t of
                            Err e ->
                                Err e
                            Ok rest ->
                                Ok (val :: rest)
    in
    Decoder (\json ->
        case json of
            JArr arr ->
                decodeAll (arrayToList arr)
            _ ->
                Err "expected an Array")


-- # Dict decoder


-- | Decode a JSON object into a Map String a.
dict : Decoder a -> Decoder (Map String a)
export let dict decoder =
    let rec decodeEntries obj =
        case obj of
            ObjNil ->
                Ok Map.empty
            ObjCons k v rest ->
                case run decoder v of
                    Err e ->
                        Err ("in key '" ++ k ++ "': " ++ e)
                    Ok val ->
                        case decodeEntries rest of
                            Err e ->
                                Err e
                            Ok m ->
                                Ok (Map.insert k val m)
    in
    Decoder (\json ->
        case json of
            JObj obj ->
                decodeEntries obj
            _ ->
                Err "expected an Object")


-- # Combinators


-- | Transform the result of a decoder.
map : (a -> b) -> Decoder a -> Decoder b
export let map f decoder =
    Decoder (\json ->
        case run decoder json of
            Ok val ->
                Ok (f val)
            Err e ->
                Err e)


-- | Combine two decoders.
map2 : (a -> b -> c) -> Decoder a -> Decoder b -> Decoder c
export let map2 f da db =
    Decoder (\json ->
        case run da json of
            Err e ->
                Err e
            Ok a ->
                case run db json of
                    Err e ->
                        Err e
                    Ok b ->
                        Ok (f a b))


-- | Combine three decoders.
map3 : (a -> b -> c -> d) -> Decoder a -> Decoder b -> Decoder c -> Decoder d
export let map3 f da db dc =
    Decoder (\json ->
        case run da json of
            Err e ->
                Err e
            Ok a ->
                case run db json of
                    Err e ->
                        Err e
                    Ok b ->
                        case run dc json of
                            Err e ->
                                Err e
                            Ok c ->
                                Ok (f a b c))


-- | Combine four decoders.
map4 : (a -> b -> c -> d -> e) -> Decoder a -> Decoder b -> Decoder c -> Decoder d -> Decoder e
export let map4 f da db dc dd =
    Decoder (\json ->
        case run da json of
            Err e ->
                Err e
            Ok a ->
                case run db json of
                    Err e ->
                        Err e
                    Ok b ->
                        case run dc json of
                            Err e ->
                                Err e
                            Ok c ->
                                case run dd json of
                                    Err e ->
                                        Err e
                                    Ok d ->
                                        Ok (f a b c d))


-- | Combine five decoders.
map5 : (a -> b -> c -> d -> e -> f) -> Decoder a -> Decoder b -> Decoder c -> Decoder d -> Decoder e -> Decoder f
export let map5 f da db dc dd de =
    Decoder (\json ->
        case run da json of
            Err ea ->
                Err ea
            Ok a ->
                case run db json of
                    Err eb ->
                        Err eb
                    Ok b ->
                        case run dc json of
                            Err ec ->
                                Err ec
                            Ok c ->
                                case run dd json of
                                    Err ed ->
                                        Err ed
                                    Ok d ->
                                        case run de json of
                                            Err ee ->
                                                Err ee
                                            Ok e ->
                                                Ok (f a b c d e))


-- | Chain decoders — use the result of one decoder to pick the next.
andThen : (a -> Decoder b) -> Decoder a -> Decoder b
export let andThen f decoder =
    Decoder (\json ->
        case run decoder json of
            Err e ->
                Err e
            Ok val ->
                run (f val) json)


-- | Try a list of decoders, succeeding with the first one that works.
oneOf : [Decoder a] -> Decoder a
export let oneOf decoders =
    let rec tryAll ds json =
        case ds of
            [] ->
                Err "oneOf: all decoders failed"
            [d|rest] ->
                case run d json of
                    Ok val ->
                        Ok val
                    Err _ ->
                        tryAll rest json
    in
    Decoder (\json -> tryAll decoders json)


-- | Try a decoder, wrapping the result in Maybe.
-- Note: swallows all errors. Prefer `nullable` or `optionalField` for
-- more precise semantics.
maybe : Decoder a -> Decoder (Maybe a)
export let maybe decoder =
    Decoder (\json ->
        case run decoder json of
            Ok val ->
                Ok (Just val)
            Err _ ->
                Ok Nothing)


-- | Decode a value that may be null. Succeeds with `Just val` if the
-- decoder succeeds, `Nothing` if the value is null, and fails on
-- type mismatches (unlike `maybe` which swallows all errors).
nullable : Decoder a -> Decoder (Maybe a)
export let nullable decoder =
    Decoder (\json ->
        case json of
            JNull ->
                Ok Nothing
            _ ->
                case run decoder json of
                    Ok val ->
                        Ok (Just val)
                    Err e ->
                        Err e)


-- | Decode a field that may be absent from the object. Returns `Nothing`
-- if the key is missing, `Just val` if present and decoded successfully,
-- and fails if the key exists but the decoder fails (type mismatch, etc.).
optionalField : String -> Decoder a -> Decoder (Maybe a)
export let optionalField key decoder =
    Decoder (\json ->
        case json of
            JObj obj ->
                case getField key obj of
                    Nothing ->
                        Ok Nothing
                    Just val ->
                        case run decoder val of
                            Ok v ->
                                Ok (Just v)
                            Err e ->
                                Err ("in field '" ++ key ++ "': " ++ e)
            _ ->
                Err "expected an Object")


-- | A decoder that always succeeds with the given value.
succeed : a -> Decoder a
export let succeed val =
    Decoder (\_ -> Ok val)


-- | A decoder that always fails with the given message.
fail : String -> Decoder a
export let fail msg =
    Decoder (\_ -> Err msg)


-- # Tests


test "string decoder" =
    assert (decodeString string "\"hello\"" == Ok "hello")
    assert (decodeString string "42" == Err "expected a String")

test "int decoder" =
    assert (decodeString int "42" == Ok 42)
    assert (decodeString int "3.14" == Err "expected an Int but got a Float")
    assert (decodeString int "\"hi\"" == Err "expected an Int")

test "float decoder" =
    assert (decodeString float "3.14" == Ok 3.14)
    assert (decodeString float "42" == Ok 42.0)
    assert (decodeString float "true" == Err "expected a Float")

test "bool decoder" =
    assert (decodeString bool "true" == Ok true)
    assert (decodeString bool "false" == Ok false)
    assert (decodeString bool "1" == Err "expected a Bool")

test "null decoder" =
    assert (decodeString (null 0) "null" == Ok 0)
    assert (decodeString (null "default") "null" == Ok "default")
    assert (decodeString (null 0) "42" == Err "expected null")

test "field decoder" =
    let json = """{"name": "Alice", "age": 30}"""
    assert (decodeString (field "name" string) json == Ok "Alice")
    assert (decodeString (field "age" int) json == Ok 30)
    assert (decodeString (field "missing" string) json == Err "field 'missing' not found")

test "at decoder" =
    let json = """{"user": {"name": "Bob"}}"""
    assert (decodeString (at ["user", "name"] string) json == Ok "Bob")

test "index decoder" =
    let json = "[10, 20, 30]"
    assert (decodeString (index 0 int) json == Ok 10)
    assert (decodeString (index 2 int) json == Ok 30)
    assert (decodeString (index 5 int) json == Err "index 5 out of range")

test "list decoder" =
    assert (decodeString (list int) "[1, 2, 3]" == Ok [1, 2, 3])
    assert (decodeString (list string) """["a", "b"]""" == Ok ["a", "b"])
    assert (decodeString (list int) "[]" == Ok [])
    assert (decodeString (list int) """[1, "x"]""" == Err "expected an Int")

test "dict decoder" =
    import Std:Result (withDefault)
    let result = decodeString (dict int) """{"a": 1, "b": 2}"""
    let m = withDefault Map.empty result
    assert (Map.lookup "a" m == Just 1)
    assert (Map.lookup "b" m == Just 2)

test "map decoder" =
    let decoder = map (\s -> s ++ "!") string
    assert (decodeString decoder "\"hi\"" == Ok "hi!")

test "map2 decoder" =
    let json = """{"x": 1, "y": 2}"""
    let decoder = map2 (\x y -> x + y) (field "x" int) (field "y" int)
    assert (decodeString decoder json == Ok 3)

test "map3 decoder" =
    let json = """{"a": 1, "b": 2, "c": 3}"""
    let decoder = map3 (\a b c -> a + b + c) (field "a" int) (field "b" int) (field "c" int)
    assert (decodeString decoder json == Ok 6)

test "andThen decoder" =
    let json = """{"type": "greeting", "message": "hello"}"""
    let decoder =
        field "type" string |> andThen (\t ->
            if t == "greeting" then
                field "message" string
            else
                fail "unknown type")
    assert (decodeString decoder json == Ok "hello")

test "oneOf decoder" =
    let decoder = oneOf [int |> map toFloat, float]
    assert (decodeString decoder "42" == Ok 42.0)
    assert (decodeString decoder "3.14" == Ok 3.14)

test "maybe decoder" =
    assert (decodeString (maybe int) "42" == Ok (Just 42))
    assert (decodeString (maybe int) "\"hi\"" == Ok Nothing)

test "nullable decoder" =
    assert (decodeString (nullable int) "42" == Ok (Just 42))
    assert (decodeString (nullable int) "null" == Ok Nothing)
    assert (decodeString (nullable int) "\"hi\"" == Err "expected an Int")

test "optionalField decoder" =
    let json1 = """{"name": "Alice", "age": 30}"""
    let json2 = """{"name": "Bob"}"""
    let json3 = """{"name": "Eve", "age": "not a number"}"""
    assert (decodeString (optionalField "age" int) json1 == Ok (Just 30))
    assert (decodeString (optionalField "age" int) json2 == Ok Nothing)
    assert (decodeString (optionalField "missing" int) json1 == Ok Nothing)
    assert (decodeString (optionalField "age" int) json3 == Err "in field 'age': expected an Int")

test "succeed and fail" =
    assert (decodeString (succeed 42) "null" == Ok 42)
    assert (decodeString (fail "nope") "null" == Err "nope")

test "nested object decoding with map2" =
    let json = """{"name": "Alice", "scores": [95, 87, 92]}"""
    let decoder = map2 (\name scores -> (name, scores)) (field "name" string) (field "scores" (list int))
    assert (decodeString decoder json == Ok ("Alice", [95, 87, 92]))

test "decodeString with invalid JSON" =
    assert (decodeString string "not json" == Err "invalid character 'o' in literal null (expecting 'u')")
