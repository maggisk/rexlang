export length, toUpper, toLower, trim, split, join, toString, contains, startsWith, endsWith, isEmpty, charAt, substring, indexOf, replace, repeat, padLeft, padRight, words, lines, charCode, fromCharCode, parseInt, parseFloat


-- # Query


-- | Determine if a string is empty.
--
--     isEmpty "" == true
--     isEmpty "hi" == false
--
let isEmpty s =
    s == ""


-- # Tests
-- Note: these tests run in a standalone context where only core builtins
-- (not, error) and prelude operators are available. Builtin string functions
-- (length, toUpper, etc.) are tested via imports in test_eval.py.


test "isEmpty" =
    assert (isEmpty "")
    assert (not (isEmpty "x"))
    assert (not (isEmpty " "))
