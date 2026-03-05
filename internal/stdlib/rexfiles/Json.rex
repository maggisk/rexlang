import Std:String (replace, toString)
import Std:Maybe (Just, Nothing)


-- # Types


export type Json = JNull | JBool bool | JStr string | JNum float | JArr JsonList | JObj JsonObj

export type JsonList = ArrNil | ArrCons Json JsonList

export type JsonObj = ObjNil | ObjCons string Json JsonObj


-- # Parse


-- | Parse a JSON string. Returns Ok Json on success, Err String on failure.
--
--     parse "null" == Ok JNull
--     parse "{\"x\": 1}" == Ok (JObj (ObjCons "x" (JNum 1.0) ObjNil))
--
parse : String -> Result Json String
export let parse s =
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
stringify : Json -> String
export let rec stringify j =
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
                "${stringify x}, ${strArr rest}"
    in
    let rec strObj obj =
        case obj of
            ObjNil ->
                ""
            ObjCons k v (ObjNil) ->
                "\"${escapeStr k}\": ${stringify v}"
            ObjCons k v rest ->
                "\"${escapeStr k}\": ${stringify v}, ${strObj rest}"
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
    assert (stringify (JArr ArrNil) == "[]")
    assert (stringify (JArr (ArrCons JNull ArrNil)) == "[null]")
    assert (stringify (JArr (ArrCons (JBool true) (ArrCons (JBool false) ArrNil))) == "[true, false]")

test "stringify object" =
    assert (stringify (JObj ObjNil) == "{}")
    assert (stringify (JObj (ObjCons "x" (JNum 1.0) ObjNil)) == "{\"x\": 1.0}")

test "escape in strings" =
    assert (stringify (JStr "say \"hi\"") == "\"say \\\"hi\\\"\"")
    assert (stringify (JStr "line1\nline2") == "\"line1\\nline2\"")


-- # Encode helpers


-- | Create a JSON null.
export let encodeNull = JNull

-- | Create a JSON boolean.
encodeBool : Bool -> Json
export let encodeBool b = JBool b

-- | Create a JSON number from a float.
encodeNum : Float -> Json
export let encodeNum n = JNum n

-- | Create a JSON string.
encodeStr : String -> Json
export let encodeStr s = JStr s

-- | Create a JSON array from a Rex list of Json values.
encodeArr : [Json] -> Json
export let encodeArr lst =
    let rec fromList xs =
        case xs of
            [] ->
                ArrNil
            [h|t] ->
                ArrCons h (fromList t)
    in
    JArr (fromList lst)

-- | Create a JSON object from a Rex list of (String, Json) pairs.
encodeObj : [(String, Json)] -> Json
export let encodeObj pairs =
    let rec fromList xs =
        case xs of
            [] ->
                ObjNil
            [pair|t] ->
                let (k, v) = pair in
                ObjCons k v (fromList t)
    in
    JObj (fromList pairs)

test "encodeArr and encodeObj" =
    assert ([JNull, JBool true] |> encodeArr |> stringify == "[null, true]")
    assert ([("a", JNum 1.0), ("b", JNull)] |> encodeObj |> stringify == "{\"a\": 1.0, \"b\": null}")


-- # Decode helpers


-- | Look up a field in a JSON object, returning Nothing if absent.
getField : String -> JsonObj -> Maybe Json
export let rec getField key obj =
    case obj of
        ObjNil ->
            Nothing
        ObjCons k v rest ->
            if k == key then
                Just v
            else
                getField key rest

test "getField" =
    let obj = ObjCons "x" (JNum 1.0) (ObjCons "y" (JStr "hi") ObjNil)
    assert (getField "x" obj == Just (JNum 1.0))
    assert (getField "z" obj == Nothing)


-- | Convert a JsonList to a Rex list.
arrayToList : JsonList -> [Json]
export let rec arrayToList arr =
    case arr of
        ArrNil ->
            []
        ArrCons x rest ->
            x :: arrayToList rest


-- | Convert a Rex list to a JsonList.
listToArray : [Json] -> JsonList
export let listToArray lst =
    let rec fromList xs =
        case xs of
            [] -> ArrNil
            [h|t] -> ArrCons h (fromList t)
    in
    fromList lst
