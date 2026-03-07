import Std:String (replace, toString)
import Std:Maybe (Just, Nothing)
import Std:List (intersperse, map, foldl)


-- # Types


export type Json = JNull | JBool Bool | JStr String | JNum Float | JArr [Json] | JObj [(String, Json)]


-- # Parse


-- | Parse a JSON string. Returns Ok Json on success, Err String on failure.
--
--     parse "null" == Ok JNull
--     parse "{\"x\": 1}" == Ok (JObj [("x", JNum 1.0)])
--
export
parse : String -> Result Json String
parse s =
    jsonParse s

test "parse valid JSON" =
    import Std:Result (isOk)
    assert ("null" |> parse |> isOk)
    assert ("true" |> parse |> isOk)
    assert ("42" |> parse |> isOk)
    assert ("\"hello\"" |> parse |> isOk)
    assert ("[]" |> parse |> isOk)
    assert ("{}" |> parse |> isOk)
    assert ("[1, 2, 3]" |> parse |> isOk)
    assert ("{\"key\": \"value\"}" |> parse |> isOk)

test "parse invalid JSON" =
    import Std:Result (isErr)
    assert ("invalid" |> parse |> isErr)
    assert ("{unclosed" |> parse |> isErr)


-- # Stringify


-- | Serialize a Json value to a JSON string.
--
--     stringify JNull == "null"
--     stringify (JBool true) == "true"
--     stringify (JStr "hi") == "\"hi\""
--
export
stringify : Json -> String
stringify j =
    let escapeStr s =
        s
            |> replace "\\" "\\\\"
            |> replace "\"" "\\\""
            |> replace "\n" "\\n"
            |> replace "\r" "\\r"
            |> replace "\t" "\\t"
    in
    let rec strArr items =
        items
            |> map (\x -> stringify x)
            |> intersperse ", "
            |> foldl (\acc x -> acc ++ x) ""
    in
    let rec strObj pairs =
        pairs
            |> map (\pair ->
                let (k, v) = pair
                in "\"${escapeStr k}\": ${stringify v}")
            |> intersperse ", "
            |> foldl (\acc x -> acc ++ x) ""
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
            "\"${escapeStr s}\""
        JArr arr ->
            "[${strArr arr}]"
        JObj obj ->
            "{${strObj obj}}"

test "stringify primitives" =
    assert (stringify JNull == "null")
    assert (stringify (JBool true) == "true")
    assert (stringify (JBool false) == "false")
    assert (stringify (JStr "hi") == "\"hi\"")

test "stringify number" =
    assert (stringify (JNum 0.0) == "0.0")
    assert (stringify (JNum 3.14) == "3.14")

test "stringify array" =
    assert (stringify (JArr []) == "[]")
    assert (stringify (JArr [JNull]) == "[null]")
    assert (stringify (JArr [JBool true, JBool false]) == "[true, false]")

test "stringify object" =
    assert (stringify (JObj []) == "{}")
    assert (stringify (JObj [("x", JNum 1.0)]) == "{\"x\": 1.0}")

test "escape in strings" =
    assert (stringify (JStr "say \"hi\"") == "\"say \\\"hi\\\"\"")
    assert (stringify (JStr "line1\nline2") == "\"line1\\nline2\"")


-- # Encode helpers


-- | Create a JSON null.
export
encodeNull = JNull

-- | Create a JSON boolean.
export
encodeBool : Bool -> Json
encodeBool b = JBool b

-- | Create a JSON number from a float.
export
encodeNum : Float -> Json
encodeNum n = JNum n

-- | Create a JSON string.
export
encodeStr : String -> Json
encodeStr s = JStr s

-- | Create a JSON array from a Rex list of Json values.
export
encodeArr : [Json] -> Json
encodeArr lst = JArr lst

-- | Create a JSON object from a Rex list of (String, Json) pairs.
export
encodeObj : [(String, Json)] -> Json
encodeObj pairs = JObj pairs

test "encodeArr and encodeObj" =
    assert ([JNull, JBool true] |> encodeArr |> stringify == "[null, true]")
    assert ([("a", JNum 1.0), ("b", JNull)] |> encodeObj |> stringify == "{\"a\": 1.0, \"b\": null}")


-- # Decode helpers


-- | Look up a field in a JSON object's key-value pairs, returning Nothing if absent.
export
getField : String -> [(String, Json)] -> Maybe Json
getField key pairs =
    case pairs of
        [] ->
            Nothing
        [(k, v)|rest] ->
            if k == key then
                Just v
            else
                getField key rest

test "getField" =
    let obj = [("x", JNum 1.0), ("y", JStr "hi")]
    assert (getField "x" obj == Just (JNum 1.0))
    assert (getField "z" obj == Nothing)
