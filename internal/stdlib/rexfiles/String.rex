export length, toUpper, toLower, trim, split, join, toString, contains, startsWith, endsWith, isEmpty, charAt, substring, indexOf, replace, take, drop, repeat, padLeft, padRight, words, lines, charCode, fromCharCode, parseInt, parseFloat, dedent

import std:List (map, filter, foldl)


-- # Query


-- | Determine if a string is empty.
--
--     isEmpty "" == true
--     isEmpty "hi" == false
--
let isEmpty s =
    s == ""


-- # Transform


-- | Remove common leading whitespace from all lines.
-- Blank lines are ignored when computing the indentation level.
-- A trailing newline is preserved if present.
--
--     dedent "  hello\n  world\n" == "hello\nworld\n"
--     dedent "  hi\n    there\n" == "hi\n  there\n"
--
let dedent s =
    let countSpaces line =
            let rec go i =
                if i >= length line then
                    i
                else if charAt i line == Just " " then
                    go (i + 1)
                else
                    i
            in go 0
        ls = lines s
        nonEmpty = filter (fn l -> l != "") ls
        indent =
            foldl (fn acc l -> let n = countSpaces l in if n < acc then n else acc) 999999999 nonEmpty
        stripped = map (fn l -> if l == "" then "" else drop indent l) ls
        result = join "\n" stripped
    in
    if endsWith "\n" s then
        result ++ "\n"
    else
        result


-- # Tests
-- Note: these tests run in a standalone context where only core builtins
-- (not, error) and prelude operators are available. Builtin string functions
-- (length, toUpper, etc.) are tested via imports in test_eval.py.


test "isEmpty" =
    assert (isEmpty "")
    assert (not (isEmpty "x"))
    assert (not (isEmpty " "))

test "dedent removes common indentation" =
    assert (dedent "  hello\n  world\n" == "hello\nworld\n")

test "dedent preserves relative indentation" =
    assert (dedent "  hi\n    there\n" == "hi\n  there\n")

test "dedent ignores blank lines" =
    assert (dedent "  a\n\n  b\n" == "a\n\nb\n")

test "dedent no-op on unindented" =
    assert (dedent "hello\nworld" == "hello\nworld")

test "take returns first N characters" =
    assert (take 3 "hello" == "hel")
    assert (take 0 "hello" == "")
    assert (take 10 "hi" == "hi")
    assert (take 3 "" == "")

test "drop removes first N characters" =
    assert (drop 3 "hello" == "lo")
    assert (drop 0 "hello" == "hello")
    assert (drop 10 "hi" == "")
    assert (drop 3 "" == "")
