-- JSON API example: decode typed records from JSON strings

import Std:Json (parse, stringify, JStr, JNum, JObj, JArr)
import Std:Json.Decode (decodeString, field, string, int, map2, succeed, fail, list, errorToString)
import Std:Result (Ok, Err)

-- Define a User type and its decoder
type User = { name : String, age : Int }

userDecoder = map2 User
    (field "name" string)
    (field "age" int)

test "decode single user" =
    let json = "{\"name\": \"Alice\", \"age\": 30}"
    in match decodeString userDecoder json
        when Ok user ->
            assert (user.name == "Alice")
        when Err e ->
            assert false

test "decode user list" =
    let json = "[{\"name\": \"Alice\", \"age\": 30}, {\"name\": \"Bob\", \"age\": 25}]"
    in match decodeString (list userDecoder) json
        when Ok users ->
            assert (users == [User { name = "Alice", age = 30 }, User { name = "Bob", age = 25 }])
        when Err e ->
            assert false

test "decode error gives helpful message" =
    let json = "{\"name\": \"Alice\"}"
    in match decodeString userDecoder json
        when Ok _ ->
            assert false
        when Err e ->
            let msg = errorToString e
            in assert (msg == "age: field 'age' not found")
