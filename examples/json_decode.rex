import Std:Json.Decode (decodeString, field, string, int, float, bool, list, map2, decode, with, oneOf, maybe, nullable, optionalField, at, succeed, errorToString)
import Std:Result (Ok, Err)
import Std:Maybe (Just, Nothing)


-- Decode a simple object into a record

type User = { name : String, age : Int }

test "decode user" =
    let json = """{"name": "Alice", "age": 30}"""
    let userDecoder = map2 User (field "name" string) (field "age" int)
    assert (decodeString userDecoder json == Ok (User { name = "Alice", age = 30 }))


-- Decode nested objects into records

type Score = { player : String, score : Float }

test "decode nested" =
    let json = """{"user": {"name": "Bob", "score": 95.5}}"""
    let decoder = map2 Score (at ["user", "name"] string) (at ["user", "score"] float)
    assert (decodeString decoder json == Ok (Score { player = "Bob", score = 95.5 }))


-- Decode a list of records

test "decode list of records" =
    let json = """[{"name": "Alice", "age": 25}, {"name": "Bob", "age": 30}]"""
    let userDecoder = map2 User (field "name" string) (field "age" int)
    let result = decodeString (list userDecoder) json
    assert (result == Ok [User { name = "Alice", age = 25 }, User { name = "Bob", age = 30 }])


-- Use oneOf for flexible decoding

test "oneOf for union types" =
    import Std:Json.Decode (null)
    let flexDecoder = oneOf [string, null "default"]
    assert (decodeString flexDecoder "\"hello\"" == Ok "hello")
    assert (decodeString flexDecoder "null" == Ok "default")


-- Use nullable for values that may be null

type Profile = { name : String, bio : Maybe String }

test "nullable fields" =
    let decoder = map2 Profile (field "name" string) (field "bio" (nullable string))
    assert (decodeString decoder """{"name": "Alice", "bio": "hello"}""" == Ok (Profile { name = "Alice", bio = Just "hello" }))
    assert (decodeString decoder """{"name": "Bob", "bio": null}""" == Ok (Profile { name = "Bob", bio = Nothing }))


-- Use optionalField for keys that may be absent

type Config = { host : String, port : Maybe Int }

test "optional fields" =
    let decoder = map2 Config (field "host" string) (optionalField "port" int)
    assert (decodeString decoder """{"host": "localhost", "port": 8080}""" == Ok (Config { host = "localhost", port = Just 8080 }))
    assert (decodeString decoder """{"host": "localhost"}""" == Ok (Config { host = "localhost", port = Nothing }))
    -- type mismatch still fails (unlike maybe)
    let typeErr =
        case decodeString decoder """{"host": "localhost", "port": "bad"}""" of
            Err e ->
                e.path == ["port"] && e.message == "expected an Int"
            _ ->
                false
    assert typeErr


-- Use decode/with to decode many fields without needing mapN

type Player = { name : String, score : Int, active : Bool }

test "decode/with for many fields" =
    let json = """{"name": "Alice", "score": 100, "active": true}"""
    let decoder =
        decode Player
            |> with (field "name" string)
            |> with (field "score" int)
            |> with (field "active" bool)
    assert (decodeString decoder json == Ok (Player { name = "Alice", score = 100, active = true }))


-- Use andThen for dependent decoding

type Point = { x : Int, y : Int }

test "andThen for tagged types" =
    import Std:Json.Decode (andThen, fail)
    let json = """{"type": "point", "x": 10, "y": 20}"""
    let decoder =
        field "type" string |> andThen \t ->
            if t == "point" then
                map2 Point (field "x" int) (field "y" int)
            else
                fail "unknown type"
    assert (decodeString decoder json == Ok (Point { x = 10, y = 20 }))


-- Error messages include paths for nested structures

test "error path tracking" =
    let json = """{"users": [{"name": "Alice"}, {"name": 42}]}"""
    let decoder = field "users" (list (field "name" string))
    let result = decodeString decoder json
    let ok =
        case result of
            Err e ->
                errorToString e == "users.[1].name: expected a String"
            _ ->
                false
    assert ok
