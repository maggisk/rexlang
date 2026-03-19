-- json_api.rex — JSON parsing and encoding
--
-- Demonstrates Rex's JSON handling:
--   1. Parsing raw JSON into typed records via decoder combinators
--   2. Building JSON from Rex data using encode helpers
--   3. Round-tripping data through JSON
--
-- Uses Std:Json for the Json ADT, parse/stringify, and encode helpers.
-- Uses Std:Json.Decode for Elm-style decoder combinators.

import Std:IO (println)
import Std:Json (parse, stringify, encodeObj, encodeStr, encodeArr, encodeNum, encodeBool)
import Std:Json.Decode (decodeString, field, string, int, float, bool, list, map2, decode, with, oneOf, maybe, nullable, errorToString)
import Std:Result (Ok, Err)
import Std:Maybe (Just, Nothing)
import Std:Math (toFloat)
import Std:List (map, indexedMap)
import Std:String (join)


-- # Data types


type User = { name : String, email : String }

type Post = { title : String, body : String, published : Bool }

type ApiResponse = { users : [User], total : Int }


-- # Decoders — define how to extract typed data from JSON


userDecoder =
    map2 User
        (field "name" string)
        (field "email" string)

postDecoder =
    decode Post
        |> with (field "title" string)
        |> with (field "body" string)
        |> with (field "published" bool)

responseDecoder =
    map2 ApiResponse
        (field "users" (list userDecoder))
        (field "total" int)


-- # Encoders — convert Rex data to JSON


encodeUser : User -> Json
encodeUser user =
    encodeObj
        [ ("name", encodeStr user.name)
        , ("email", encodeStr user.email)
        ]

encodePost : Post -> Json
encodePost post =
    encodeObj
        [ ("title", encodeStr post.title)
        , ("body", encodeStr post.body)
        , ("published", encodeBool post.published)
        ]

encodeResponse : ApiResponse -> Json
encodeResponse resp =
    encodeObj
        [ ("users", encodeArr (resp.users |> map encodeUser))
        , ("total", encodeNum (toFloat resp.total))
        ]


-- # Tests


test "decode a single user" =
    let json = """{"name": "Alice", "email": "alice@example.com"}"""
    in assert (decodeString userDecoder json == Ok (User { name = "Alice", email = "alice@example.com" }))

test "decode a post with many fields" =
    let json = """{"title": "Hello Rex", "body": "First post!", "published": true}"""
    in assert (decodeString postDecoder json == Ok (Post { title = "Hello Rex", body = "First post!", published = true }))

test "decode a nested API response" =
    let json = """
        {
            "users": [
                {"name": "Alice", "email": "alice@example.com"},
                {"name": "Bob", "email": "bob@example.com"}
            ],
            "total": 2
        }
        """
    in
    let result = decodeString responseDecoder json
    in
    match result
        when Ok resp ->
            assert (resp.total == 2)
        when Err e ->
            assert false

test "encode and re-decode round-trips" =
    let
        original = User { name = "Charlie", email = "charlie@example.com" }
        jsonStr = original |> encodeUser |> stringify
        decoded = decodeString userDecoder jsonStr
    in assert (decoded == Ok original)

test "encode an API response" =
    let
        resp = ApiResponse
            { users = [User { name = "Alice", email = "a@b.com" }]
            , total = 1
            }
        jsonStr = resp |> encodeResponse |> stringify
    in
    -- Re-decode and verify
    match decodeString responseDecoder jsonStr
        when Ok decoded ->
            assert (decoded.total == 1)
        when Err _ ->
            assert false

test "decode error has useful message" =
    let json = """{"name": 42, "email": "test@test.com"}"""
    in
    match decodeString userDecoder json
        when Err e ->
            assert (errorToString e == "name: expected a String")
        when Ok _ ->
            assert false
