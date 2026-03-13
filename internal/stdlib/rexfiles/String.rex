import Std:List (map, filter, foldl)
import Std:Maybe (Just, Nothing)


-- # Builtins

export external length : String -> Int

test "length" =
    assert (length "hello" == 5)
    assert (length "" == 0)


export external toUpper : String -> String

test "toUpper" =
    assert ("hello" |> toUpper == "HELLO")


export external toLower : String -> String

test "toLower" =
    assert ("HELLO" |> toLower == "hello")


export external trim : String -> String

test "trim" =
    assert ("  hello  " |> trim == "hello")


export external trimLeft : String -> String

test "trimLeft" =
    assert (trimLeft "  hello  " == "hello  ")
    assert (trimLeft "" == "")


export external trimRight : String -> String

test "trimRight" =
    assert (trimRight "  hello  " == "  hello")
    assert (trimRight "" == "")


export external split : String -> String -> [String]

test "split" =
    assert (split "," "a,b,c" == ["a", "b", "c"])


export external join : String -> [String] -> String

test "join" =
    assert (join "-" ["a", "b", "c"] == "a-b-c")


export external toString : a -> String

test "toString" =
    assert (toString 42 == "42")
    assert (toString true == "true")


export external contains : String -> String -> Bool

test "contains" =
    assert (contains "ell" "hello")
    assert ("hello" |> contains "xyz" |> not)


export external startsWith : String -> String -> Bool

test "startsWith" =
    assert (startsWith "hel" "hello")
    assert ("hello" |> startsWith "bye" |> not)


export external endsWith : String -> String -> Bool

test "endsWith" =
    assert (endsWith "llo" "hello")
    assert ("hello" |> endsWith "bye" |> not)


export external charAt : Int -> String -> Maybe String

test "charAt" =
    assert (charAt 0 "hello" == Just "h")
    assert (charAt 4 "hello" == Just "o")
    assert (charAt 10 "hello" == Nothing)


export external substring : Int -> Int -> String -> String

test "substring" =
    assert (substring 1 4 "hello" == "ell")


export external indexOf : String -> String -> Maybe Int

test "indexOf" =
    assert (indexOf "ll" "hello" == Just 2)
    assert (indexOf "xyz" "hello" == Nothing)


export external replace : String -> String -> String -> String

test "replace" =
    assert ("hello" |> replace "l" "r" == "herro")


export external take : Int -> String -> String

test "take" =
    assert (take 3 "hello" == "hel")
    assert (take 0 "hello" == "")
    assert (take 10 "hi" == "hi")
    assert (take 3 "" == "")


export external drop : Int -> String -> String

test "drop" =
    assert (drop 3 "hello" == "lo")
    assert (drop 0 "hello" == "hello")
    assert (drop 10 "hi" == "")
    assert (drop 3 "" == "")


export external repeat : Int -> String -> String

test "repeat" =
    assert (repeat 3 "ab" == "ababab")
    assert (repeat 0 "ab" == "")


export external padLeft : Int -> String -> String -> String

test "padLeft" =
    assert (padLeft 5 "0" "42" == "00042")


export external padRight : Int -> String -> String -> String

test "padRight" =
    assert (padRight 5 "." "hi" == "hi...")


export external words : String -> [String]

test "words" =
    assert (words "hello world" == ["hello", "world"])


export external lines : String -> [String]

test "lines" =
    assert (lines "a\nb\nc" == ["a", "b", "c"])


export external charCode : String -> Int

test "charCode" =
    assert (charCode "A" == 65)


export external fromCharCode : Int -> String

test "fromCharCode" =
    assert (fromCharCode 65 == "A")


export external parseInt : String -> Maybe Int

test "parseInt" =
    assert (parseInt "42" == Just 42)
    assert (parseInt "abc" == Nothing)


export external parseFloat : String -> Maybe Float

test "parseFloat" =
    assert (parseFloat "3.14" == Just 3.14)
    assert (parseFloat "abc" == Nothing)


export external reverse : String -> String

test "reverse" =
    assert ("hello" |> reverse == "olleh")
    assert (reverse "" == "")
    assert (reverse "a" == "a")


export external toList : String -> [String]

test "toList" =
    assert (toList "abc" == ["a", "b", "c"])
    assert (toList "" == [])


export external fromList : [String] -> String

test "fromList" =
    assert (fromList ["a", "b", "c"] == "abc")
    assert (fromList [] == "")


-- # Query


-- | Determine if a string is empty.
--
--     isEmpty "" == true
--     isEmpty "hi" == false
--
export
isEmpty : String -> Bool
isEmpty s =
    s == ""

test "isEmpty" =
    assert (isEmpty "")
    assert ("x" |> isEmpty |> not)
    assert (" " |> isEmpty |> not)


-- # Dedent


-- | Remove common leading whitespace from all lines.
-- Blank lines are ignored when computing the indentation level.
-- A trailing newline is preserved if present.
--
--     dedent "  hello\n  world\n" == "hello\nworld\n"
--     dedent "  hi\n    there\n" == "hi\n  there\n"
--
export
dedent : String -> String
dedent s =
    let
        countSpaces line =
            let rec go i =
                if i >= length line then
                    i
                else if charAt i line == Just " " then
                    go (i + 1)
                else
                    i
            in go 0
        ls = lines s
        nonEmpty = filter (\l -> l != "") ls
        indent =
            foldl (\acc l -> let n = countSpaces l in if n < acc then n else acc) 999999999 nonEmpty
        stripped = map (\l -> if l == "" then "" else drop indent l) ls
        result = join "\n" stripped
    in
    if endsWith "\n" s then
        result ++ "\n"
    else
        result

test "dedent removes common indentation" =
    assert (dedent "  hello\n  world\n" == "hello\nworld\n")

test "dedent preserves relative indentation" =
    assert (dedent "  hi\n    there\n" == "hi\n  there\n")

test "dedent ignores blank lines" =
    assert (dedent "  a\n\n  b\n" == "a\n\nb\n")

test "dedent no-op on unindented" =
    assert (dedent "hello\nworld" == "hello\nworld")
