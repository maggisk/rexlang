export JNull, JBool, JNum, JStr, JArr, JObj, ArrNil, ArrCons, ObjNil, ObjCons, parse, stringify, encodeNull, encodeBool, encodeNum, encodeStr, encodeArr, encodeObj, getField, arrayToList, listToArray

import std:String (replace, toString)


-- # Types


type Json = JNull | JBool bool | JNum float | JStr string | JArr JsonList | JObj JsonObj

type JsonList = ArrNil | ArrCons Json JsonList

type JsonObj = ObjNil | ObjCons string Json JsonObj


-- # Parse


-- | Parse a JSON string. Returns Ok Json on success, Err String on failure.
--
--     parse "null" == Ok JNull
--     parse "{\"x\": 1}" == Ok (JObj (ObjCons "x" (JNum 1.0) ObjNil))
--
let parse s =
    jsonParse s


-- # Stringify helpers


-- # Stringify


-- | Serialize a Json value to a JSON string.
--
--     stringify JNull == "null"
--     stringify (JBool true) == "true"
--     stringify (JStr "hi") == "\"hi\""
--
let rec stringify j =
    let escapeStr s =
        s
            |> replace "\\" "\\\\"
            |> replace "\"" "\\\""
            |> replace "\n" "\\n"
            |> replace "\r" "\\r"
            |> replace "\t" "\\t"
    in
    let rec strArr arr =
        case arr of
            ArrNil ->
                ""
            ArrCons x (ArrNil) ->
                stringify x
            ArrCons x rest ->
                stringify x ++ ", " ++ strArr rest
    in
    let rec strObj obj =
        case obj of
            ObjNil ->
                ""
            ObjCons k v (ObjNil) ->
                "\"" ++ escapeStr k ++ "\": " ++ stringify v
            ObjCons k v rest ->
                "\"" ++ escapeStr k ++ "\": " ++ stringify v ++ ", " ++ strObj rest
    in
    case j of
        JNull ->
            "null"
        JBool b ->
            if b then
                "true"
            else
                "false"
        JNum n ->
            toString n
        JStr s ->
            "\"" ++ escapeStr s ++ "\""
        JArr arr ->
            "[" ++ strArr arr ++ "]"
        JObj obj ->
            "{" ++ strObj obj ++ "}"


-- # Encode helpers


-- | Create a JSON null.
let encodeNull = JNull

-- | Create a JSON boolean.
let encodeBool b = JBool b

-- | Create a JSON number from a float.
let encodeNum n = JNum n

-- | Create a JSON string.
let encodeStr s = JStr s

-- | Create a JSON array from a Rex list of Json values.
let encodeArr lst =
    let rec fromList xs =
        case xs of
            [] ->
                ArrNil
            [h|t] ->
                ArrCons h (fromList t)
    in
    JArr (fromList lst)

-- | Create a JSON object from a Rex list of (String, Json) pairs.
let encodeObj pairs =
    let rec fromList xs =
        case xs of
            [] ->
                ObjNil
            [pair|t] ->
                let (k, v) = pair in
                ObjCons k v (fromList t)
    in
    JObj (fromList pairs)


-- # Decode helpers


-- | Look up a field in a JSON object, returning Nothing if absent.
let rec getField key obj =
    case obj of
        ObjNil ->
            Nothing
        ObjCons k v rest ->
            if k == key then
                Just v
            else
                getField key rest

-- | Convert a JsonList to a Rex list.
let rec arrayToList arr =
    case arr of
        ArrNil ->
            []
        ArrCons x rest ->
            x :: arrayToList rest

-- | Convert a Rex list to a JsonList.
let listToArray lst =
    let rec fromList xs =
        case xs of
            [] -> ArrNil
            [h|t] -> ArrCons h (fromList t)
    in
    fromList lst


-- # Tests


test "stringify primitives" =
    assert (stringify JNull == "null")
    assert (stringify (JBool true) == "true")
    assert (stringify (JBool false) == "false")
    assert (stringify (JStr "hi") == "\"hi\"")

test "stringify number" =
    assert (stringify (JNum 0.0) == "0.0")
    assert (stringify (JNum 3.14) == "3.14")

test "stringify array" =
    assert (stringify (JArr ArrNil) == "[]")
    assert (stringify (JArr (ArrCons JNull ArrNil)) == "[null]")
    assert (stringify (JArr (ArrCons (JBool true) (ArrCons (JBool false) ArrNil))) == "[true, false]")

test "stringify object" =
    assert (stringify (JObj ObjNil) == "{}")
    assert (stringify (JObj (ObjCons "x" (JNum 1.0) ObjNil)) == "{\"x\": 1.0}")

test "escape in strings" =
    assert (stringify (JStr "say \"hi\"") == "\"say \\\"hi\\\"\"")
    assert (stringify (JStr "line1\nline2") == "\"line1\\nline2\"")

test "encodeArr and encodeObj" =
    assert (stringify (encodeArr [JNull, JBool true]) == "[null, true]")
    assert (stringify (encodeObj [("a", JNum 1.0), ("b", JNull)]) == "{\"a\": 1.0, \"b\": null}")

test "parse valid JSON" =
    import std:Result (isOk, isErr)
    assert (isOk (parse "null"))
    assert (isOk (parse "true"))
    assert (isOk (parse "42"))
    assert (isOk (parse "\"hello\""))
    assert (isOk (parse "[]"))
    assert (isOk (parse "{}"))
    assert (isOk (parse "[1, 2, 3]"))
    assert (isOk (parse "{\"key\": \"value\"}"))

test "parse invalid JSON" =
    import std:Result (isErr)
    assert (isErr (parse "invalid"))
    assert (isErr (parse "{unclosed"))

test "getField" =
    let obj = ObjCons "x" (JNum 1.0) (ObjCons "y" (JStr "hi") ObjNil)
    assert (getField "x" obj == Just (JNum 1.0))
    assert (getField "z" obj == Nothing)
