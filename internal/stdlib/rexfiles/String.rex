export length, toUpper, toLower, trim, split, join, toString, contains, startsWith, endsWith, charAt, substring, indexOf, replace, take, drop, repeat, padLeft, padRight, words, lines, charCode, fromCharCode, parseInt, parseFloat, reverse, toList, fromList, trimLeft, trimRight

import Std:List (map, filter, foldl)


-- # Query


-- | Determine if a string is empty.
--
--     isEmpty "" == true
--     isEmpty "hi" == false
--
isEmpty : String -> Bool
export let isEmpty s =
    s == ""

test "isEmpty" =
    assert (isEmpty "")
    assert ("x" |> isEmpty |> not)
    assert (" " |> isEmpty |> not)


-- length is a builtin

test "length" =
    assert (length "hello" == 5)
    assert (length "" == 0)


-- contains is a builtin

test "contains" =
    assert (contains "ell" "hello")
    assert ("hello" |> contains "xyz" |> not)


-- startsWith is a builtin

test "startsWith" =
    assert (startsWith "hel" "hello")
    assert ("hello" |> startsWith "bye" |> not)


-- endsWith is a builtin

test "endsWith" =
    assert (endsWith "llo" "hello")
    assert ("hello" |> endsWith "bye" |> not)


-- charAt is a builtin

test "charAt" =
    assert (charAt 0 "hello" == Just "h")
    assert (charAt 4 "hello" == Just "o")
    assert (charAt 10 "hello" == Nothing)


-- indexOf is a builtin

test "indexOf" =
    assert (indexOf "ll" "hello" == Just 2)
    assert (indexOf "xyz" "hello" == Nothing)


-- # Transform


-- toUpper is a builtin

test "toUpper" =
    assert ("hello" |> toUpper == "HELLO")


-- toLower is a builtin

test "toLower" =
    assert ("HELLO" |> toLower == "hello")


-- trim is a builtin

test "trim" =
    assert ("  hello  " |> trim == "hello")


-- trimLeft is a builtin

test "trimLeft" =
    assert (trimLeft "  hello  " == "hello  ")
    assert (trimLeft "" == "")


-- trimRight is a builtin

test "trimRight" =
    assert (trimRight "  hello  " == "  hello")
    assert (trimRight "" == "")


-- reverse is a builtin

test "reverse" =
    assert ("hello" |> reverse == "olleh")
    assert (reverse "" == "")
    assert (reverse "a" == "a")


-- replace is a builtin

test "replace" =
    assert ("hello" |> replace "l" "r" == "herro")


-- take is a builtin

test "take" =
    assert (take 3 "hello" == "hel")
    assert (take 0 "hello" == "")
    assert (take 10 "hi" == "hi")
    assert (take 3 "" == "")


-- drop is a builtin

test "drop" =
    assert (drop 3 "hello" == "lo")
    assert (drop 0 "hello" == "hello")
    assert (drop 10 "hi" == "")
    assert (drop 3 "" == "")


-- substring is a builtin

test "substring" =
    assert (substring 1 4 "hello" == "ell")


-- repeat is a builtin

test "repeat" =
    assert (repeat 3 "ab" == "ababab")
    assert (repeat 0 "ab" == "")


-- padLeft is a builtin

test "padLeft" =
    assert (padLeft 5 "0" "42" == "00042")


-- padRight is a builtin

test "padRight" =
    assert (padRight 5 "." "hi" == "hi...")


-- # Split & Join


-- split is a builtin

test "split" =
    assert (split "," "a,b,c" == ["a", "b", "c"])


-- join is a builtin

test "join" =
    assert (join "-" ["a", "b", "c"] == "a-b-c")


-- words is a builtin

test "words" =
    assert (words "hello world" == ["hello", "world"])


-- lines is a builtin

test "lines" =
    assert (lines "a\nb\nc" == ["a", "b", "c"])


-- # Convert


-- toString is a builtin

test "toString" =
    assert (toString 42 == "42")
    assert (toString true == "true")


-- toList is a builtin

test "toList" =
    assert (toList "abc" == ["a", "b", "c"])
    assert (toList "" == [])


-- fromList is a builtin

test "fromList" =
    assert (fromList ["a", "b", "c"] == "abc")
    assert (fromList [] == "")


-- charCode is a builtin

test "charCode" =
    assert (charCode "A" == 65)


-- fromCharCode is a builtin

test "fromCharCode" =
    assert (fromCharCode 65 == "A")


-- parseInt is a builtin

test "parseInt" =
    assert (parseInt "42" == Just 42)
    assert (parseInt "abc" == Nothing)


-- parseFloat is a builtin

test "parseFloat" =
    assert (parseFloat "3.14" == Just 3.14)
    assert (parseFloat "abc" == Nothing)


-- # Dedent


-- | Remove common leading whitespace from all lines.
-- Blank lines are ignored when computing the indentation level.
-- A trailing newline is preserved if present.
--
--     dedent "  hello\n  world\n" == "hello\nworld\n"
--     dedent "  hi\n    there\n" == "hi\n  there\n"
--
dedent : String -> String
export let dedent s =
    let countSpaces line =
            let rec go i =
                if i >= length line then
                    i
                else if charAt i line == Just " " then
                    go (i + 1)
                else
                    i
            in go 0
    and ls = lines s
    and nonEmpty = filter (\l -> l != "") ls
    and indent =
            foldl (\acc l -> let n = countSpaces l in if n < acc then n else acc) 999999999 nonEmpty
    and stripped = map (\l -> if l == "" then "" else drop indent l) ls
    and result = join "\n" stripped
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
