export length, toUpper, toLower, trim, split, join, toString, contains, startsWith, endsWith, isEmpty


-- # Query


-- | Determine if a string is empty.
--
--     isEmpty "" == true
--     isEmpty "hi" == false
--
let isEmpty s =
    length s == 0
