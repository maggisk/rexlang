import Std:Json (JNull, JBool, JStr, JNum, JArr, JObj, ObjNil, ObjCons, parse, getField, arrayToList)
import Std:Result (Ok, Err)
import Std:Math (round, toFloat)
import Std:Map as Map
import Std:String (join)


-- # Types


-- | A structured decode error with path, message, and the JSON value that failed.
export type DecodeError = { path : [String], message : String, value : Json }


-- | Convert a DecodeError to a human-readable string.
errorToString : DecodeError -> String
export let errorToString err =
    let pathStr = if err.path == [] then
            ""
        else
            join "." err.path ++ ": "
    in pathStr ++ err.message


-- | A decoder that extracts a value of type `a` from JSON.
export type Decoder a = Decoder (Json -> Result a DecodeError)


-- # Runners


-- | Run a decoder on a Json value.
run : Decoder a -> Json -> Result a DecodeError
export let run decoder json =
    case decoder of
        Decoder f ->
            f json


-- | Parse a JSON string and run a decoder on the result.
decodeString : Decoder a -> String -> Result a DecodeError
export let decodeString decoder str =
    case parse str of
        Err e ->
            Err (DecodeError { path = [], message = e, value = JNull })
        Ok json ->
            run decoder json

test "decodeString with invalid JSON" =
    assert (decodeString string "not json" == Err (DecodeError { path = [], message = "invalid character 'o' in literal null (expecting 'u')", value = JNull }))


-- # Base decoders


-- | Decode a JSON string.
string : Decoder String
export let string =
    Decoder (\json ->
        case json of
            JStr s ->
                Ok s
            _ ->
                Err (DecodeError { path = [], message = "expected a String", value = json }))

test "string decoder" =
    assert (decodeString string "\"hello\"" == Ok "hello")
    assert (decodeString string "42" == Err (DecodeError { path = [], message = "expected a String", value = JNum 42.0 }))


-- | Decode a JSON integer.
int : Decoder Int
export let int =
    Decoder (\json ->
        case json of
            JNum n ->
                if toFloat (round n) == n then
                    Ok (round n)
                else
                    Err (DecodeError { path = [], message = "expected an Int but got a Float", value = json })
            _ ->
                Err (DecodeError { path = [], message = "expected an Int", value = json }))

test "int decoder" =
    assert (decodeString int "42" == Ok 42)
    assert (decodeString int "3.14" == Err (DecodeError { path = [], message = "expected an Int but got a Float", value = JNum 3.14 }))
    assert (decodeString int "\"hi\"" == Err (DecodeError { path = [], message = "expected an Int", value = JStr "hi" }))


-- | Decode a JSON float.
float : Decoder Float
export let float =
    Decoder (\json ->
        case json of
            JNum n ->
                Ok n
            _ ->
                Err (DecodeError { path = [], message = "expected a Float", value = json }))

test "float decoder" =
    assert (decodeString float "3.14" == Ok 3.14)
    assert (decodeString float "42" == Ok 42.0)
    assert (decodeString float "true" == Err (DecodeError { path = [], message = "expected a Float", value = JBool true }))


-- | Decode a JSON boolean.
bool : Decoder Bool
export let bool =
    Decoder (\json ->
        case json of
            JBool b ->
                Ok b
            _ ->
                Err (DecodeError { path = [], message = "expected a Bool", value = json }))

test "bool decoder" =
    assert (decodeString bool "true" == Ok true)
    assert (decodeString bool "false" == Ok false)
    assert (decodeString bool "1" == Err (DecodeError { path = [], message = "expected a Bool", value = JNum 1.0 }))


-- | Decode a JSON null, succeeding with the given default value.
null : a -> Decoder a
export let null default =
    Decoder (\json ->
        case json of
            JNull ->
                Ok default
            _ ->
                Err (DecodeError { path = [], message = "expected null", value = json }))

test "null decoder" =
    assert (decodeString (null 0) "null" == Ok 0)
    assert (decodeString (null "default") "null" == Ok "default")
    assert (decodeString (null 0) "42" == Err (DecodeError { path = [], message = "expected null", value = JNum 42.0 }))


-- # Object decoders


-- | Decode a field from a JSON object.
field : String -> Decoder a -> Decoder a
export let field key decoder =
    Decoder (\json ->
        case json of
            JObj obj ->
                case getField key obj of
                    Just val ->
                        case run decoder val of
                            Ok v ->
                                Ok v
                            Err e ->
                                Err ({ e | path = key :: e.path })
                    Nothing ->
                        Err (DecodeError { path = [key], message = "field '${key}' not found", value = json })
            _ ->
                Err (DecodeError { path = [], message = "expected an Object", value = json }))

test "field decoder" =
    let json = """{"name": "Alice", "age": 30}"""
    assert (decodeString (field "name" string) json == Ok "Alice")
    assert (decodeString (field "age" int) json == Ok 30)
    let missingResult =
        case decodeString (field "missing" string) json of
            Err e ->
                e.message == "field 'missing' not found" && e.path == ["missing"]
            _ ->
                false
    assert missingResult

test "field path tracking" =
    let json = """{"user": {"name": 42}}"""
    let result = decodeString (field "user" (field "name" string)) json
    let pathOk =
        case result of
            Err e ->
                e.path == ["user", "name"] && e.message == "expected a String"
            _ ->
                false
    assert pathOk
    let strOk =
        case result of
            Err e ->
                errorToString e == "user.name: expected a String"
            _ ->
                false
    assert strOk


-- | Decode a value at a nested path in a JSON object.
at : [String] -> Decoder a -> Decoder a
export let rec at keys decoder =
    case keys of
        [] ->
            decoder
        [k|rest] ->
            field k (at rest decoder)

test "at decoder" =
    let json = """{"user": {"name": "Bob"}}"""
    assert (decodeString (at ["user", "name"] string) json == Ok "Bob")

test "at path tracking" =
    let json = """{"user": {"name": 42}}"""
    let result = decodeString (at ["user", "name"] string) json
    let ok =
        case result of
            Err e ->
                e.path == ["user", "name"]
            _ ->
                false
    assert ok


-- # Array decoders


-- | Decode an element at a given index in a JSON array.
index : Int -> Decoder a -> Decoder a
export let index i decoder =
    let rec nth n lst =
        case lst of
            [] ->
                Err (DecodeError { path = ["[${i}]"], message = "index ${i} out of range", value = JNull })
            [h|t] ->
                if n == 0 then
                    case run decoder h of
                        Ok v ->
                            Ok v
                        Err e ->
                            Err ({ e | path = "[${i}]" :: e.path })
                else
                    nth (n - 1) t
    in
    Decoder (\json ->
        case json of
            JArr arr ->
                nth i (arrayToList arr)
            _ ->
                Err (DecodeError { path = [], message = "expected an Array", value = json }))

test "index decoder" =
    let json = "[10, 20, 30]"
    assert (decodeString (index 0 int) json == Ok 10)
    assert (decodeString (index 2 int) json == Ok 30)
    let indexErr =
        case decodeString (index 5 int) json of
            Err e ->
                e.message == "index 5 out of range" && e.path == ["[5]"]
            _ ->
                false
    assert indexErr

test "index path tracking" =
    let json = """[1, "bad", 3]"""
    let result = decodeString (index 1 int) json
    let ok =
        case result of
            Err e ->
                e.path == ["[1]"] && e.message == "expected an Int"
            _ ->
                false
    assert ok


-- | Decode a JSON array, applying the given decoder to each element.
list : Decoder a -> Decoder [a]
export let list decoder =
    let rec decodeAll idx items =
        case items of
            [] ->
                Ok []
            [h|t] ->
                case run decoder h of
                    Err e ->
                        Err ({ e | path = "[${idx}]" :: e.path })
                    Ok val ->
                        case decodeAll (idx + 1) t of
                            Err e ->
                                Err e
                            Ok rest ->
                                Ok (val :: rest)
    in
    Decoder (\json ->
        case json of
            JArr arr ->
                decodeAll 0 (arrayToList arr)
            _ ->
                Err (DecodeError { path = [], message = "expected an Array", value = json }))

test "list decoder" =
    assert (decodeString (list int) "[1, 2, 3]" == Ok [1, 2, 3])
    assert (decodeString (list string) """["a", "b"]""" == Ok ["a", "b"])
    assert (decodeString (list int) "[]" == Ok [])
    let listErr =
        case decodeString (list int) """[1, "x"]""" of
            Err e ->
                e.path == ["[1]"] && e.message == "expected an Int"
            _ ->
                false
    assert listErr

test "list path tracking" =
    let json = """[{"name": "Alice"}, {"name": 42}]"""
    let decoder = list (field "name" string)
    let result = decodeString decoder json
    let pathOk =
        case result of
            Err e ->
                e.path == ["[1]", "name"] && e.message == "expected a String"
            _ ->
                false
    assert pathOk
    let strOk =
        case result of
            Err e ->
                errorToString e == "[1].name: expected a String"
            _ ->
                false
    assert strOk


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
                        Err ({ e | path = k :: e.path })
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
                Err (DecodeError { path = [], message = "expected an Object", value = json }))

test "dict decoder" =
    import Std:Result (withDefault)
    let result = decodeString (dict int) """{"a": 1, "b": 2}"""
    let m = withDefault Map.empty result
    assert (Map.lookup "a" m == Just 1)
    assert (Map.lookup "b" m == Just 2)

test "dict path tracking" =
    let result = decodeString (dict int) """{"a": 1, "b": "bad"}"""
    let ok =
        case result of
            Err e ->
                e.path == ["b"] && e.message == "expected an Int"
            _ ->
                false
    assert ok


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

test "map decoder" =
    let decoder = map (\s -> s ++ "!") string
    assert (decodeString decoder "\"hi\"" == Ok "hi!")


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

test "map2 decoder" =
    let json = """{"x": 1, "y": 2}"""
    let decoder = map2 (\x y -> x + y) (field "x" int) (field "y" int)
    assert (decodeString decoder json == Ok 3)


-- | Apply a decoder of a function to a decoder of a value. Enables
-- decoding any number of fields by chaining with `decode` and `|>`:
--
--     decode Player
--         |> with (field "name" string)
--         |> with (field "score" int)
with : Decoder a -> Decoder (a -> b) -> Decoder b
export let with da df =
    map2 (\f a -> f a) df da

test "with decoder" =
    let json = """{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5}"""
    let decoder =
        decode (\a b c d e -> a + b + c + d + e)
            |> with (field "a" int)
            |> with (field "b" int)
            |> with (field "c" int)
            |> with (field "d" int)
            |> with (field "e" int)
    assert (decodeString decoder json == Ok 15)


-- | Chain decoders — use the result of one decoder to pick the next.
andThen : (a -> Decoder b) -> Decoder a -> Decoder b
export let andThen f decoder =
    Decoder (\json ->
        case run decoder json of
            Err e ->
                Err e
            Ok val ->
                run (f val) json)

test "andThen decoder" =
    let json = """{"type": "greeting", "message": "hello"}"""
    let decoder =
        field "type" string |> andThen (\t ->
            if t == "greeting" then
                field "message" string
            else
                fail "unknown type")
    assert (decodeString decoder json == Ok "hello")


-- | Try a list of decoders, succeeding with the first one that works.
oneOf : [Decoder a] -> Decoder a
export let oneOf decoders =
    let rec tryAll ds json =
        case ds of
            [] ->
                Err (DecodeError { path = [], message = "oneOf: all decoders failed", value = json })
            [d|rest] ->
                case run d json of
                    Ok val ->
                        Ok val
                    Err _ ->
                        tryAll rest json
    in
    Decoder (\json -> tryAll decoders json)

test "oneOf decoder" =
    let decoder = oneOf [int |> map toFloat, float]
    assert (decodeString decoder "42" == Ok 42.0)
    assert (decodeString decoder "3.14" == Ok 3.14)


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

test "maybe decoder" =
    assert (decodeString (maybe int) "42" == Ok (Just 42))
    assert (decodeString (maybe int) "\"hi\"" == Ok Nothing)


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

test "nullable decoder" =
    assert (decodeString (nullable int) "42" == Ok (Just 42))
    assert (decodeString (nullable int) "null" == Ok Nothing)
    let nullErr =
        case decodeString (nullable int) "\"hi\"" of
            Err e ->
                e.message == "expected an Int"
            _ ->
                false
    assert nullErr


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
                                Err ({ e | path = key :: e.path })
            _ ->
                Err (DecodeError { path = [], message = "expected an Object", value = json }))

test "optionalField decoder" =
    let json1 = """{"name": "Alice", "age": 30}"""
    let json2 = """{"name": "Bob"}"""
    let json3 = """{"name": "Eve", "age": "not a number"}"""
    assert (decodeString (optionalField "age" int) json1 == Ok (Just 30))
    assert (decodeString (optionalField "age" int) json2 == Ok Nothing)
    assert (decodeString (optionalField "missing" int) json1 == Ok Nothing)
    let optErr =
        case decodeString (optionalField "age" int) json3 of
            Err e ->
                e.path == ["age"] && e.message == "expected an Int"
            _ ->
                false
    assert optErr


-- | A decoder that always succeeds with the given value.
succeed : a -> Decoder a
export let succeed val =
    Decoder (\_ -> Ok val)


-- | Start a decoding pipeline. Alias for `succeed`, reads naturally
-- when chained with `with`:
--
--     decode Player
--         |> with (field "name" string)
--         |> with (field "score" int)
decode : a -> Decoder a
export let decode = succeed


-- | A decoder that always fails with the given message.
fail : String -> Decoder a
export let fail msg =
    Decoder (\json -> Err (DecodeError { path = [], message = msg, value = json }))

test "succeed and fail" =
    assert (decodeString (succeed 42) "null" == Ok 42)
    let failErr =
        case decodeString (fail "nope") "null" of
            Err e ->
                e.message == "nope"
            _ ->
                false
    assert failErr

test "nested object decoding with map2" =
    let json = """{"name": "Alice", "scores": [95, 87, 92]}"""
    let decoder = map2 (\name scores -> (name, scores)) (field "name" string) (field "scores" (list int))
    assert (decodeString decoder json == Ok ("Alice", [95, 87, 92]))

test "errorToString formatting" =
    let e1 = DecodeError { path = [], message = "expected a String", value = JNull }
    assert (errorToString e1 == "expected a String")
    let e2 = DecodeError { path = ["user", "name"], message = "expected a String", value = JNull }
    assert (errorToString e2 == "user.name: expected a String")
    let e3 = DecodeError { path = ["items", "[0]", "name"], message = "expected a String", value = JNull }
    assert (errorToString e3 == "items.[0].name: expected a String")
