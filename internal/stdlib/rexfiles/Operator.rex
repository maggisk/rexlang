export add, sub, mul, div, mod, eq, neq, lt, lte, gt, gte, cat, cons, flip, identity, const


-- # Arithmetic


-- | add a b == a + b
-- | Useful as a first-class function: foldl add 0, map (add 1)
let add a b = a + b

test "add" =
    assert (add 3 4 == 7)
    assert (add 0 5 == 5)


-- | sub a b == a - b
-- | For "x - n" use (flip sub n): map (flip sub 1) [5, 4, 3] == [4, 3, 2]
let sub a b = a - b

test "sub" =
    assert (sub 10 3 == 7)
    assert (flip sub 1 5 == 4)


-- | mul a b == a * b
-- | Useful for scaling: map (mul 2) [1, 2, 3] == [2, 4, 6]
let mul a b = a * b

test "mul" =
    assert (mul 3 4 == 12)
    assert (mul 2 0 == 0)


-- | div a b == a / b
let div a b = a / b

test "div" =
    assert (div 10 2 == 5)


-- | mod a b == a % b
let mod a b = a % b

test "mod" =
    assert (mod 10 3 == 1)


-- # Comparison


-- | eq a b == (a == b)
let eq a b = a == b

test "eq" =
    assert (eq 3 3)
    assert (eq "hi" "hi")


-- | neq a b == (a != b)
let neq a b = a != b

test "neq" =
    assert (neq 3 4)
    assert (neq "a" "b")


-- | lt a b == (a < b)
let lt a b = a < b

test "lt" =
    assert (lt 1 2)
    assert (not (lt 2 2))


-- | lte a b == (a <= b)
let lte a b = a <= b

test "lte" =
    assert (lte 2 2)
    assert (lte 1 2)


-- | gt a b == (a > b)
let gt a b = a > b

test "gt" =
    assert (gt 3 2)
    assert (not (gt 2 2))


-- | gte a b == (a >= b)
let gte a b = a >= b

test "gte" =
    assert (gte 3 3)
    assert (gte 4 3)


-- # String / List


-- | cat a b == a ++ b  (string concatenation)
let cat a b = a ++ b

test "cat" =
    assert (cat "hello" " world" == "hello world")
    assert (cat "" "x" == "x")


-- | cons x xs == x :: xs  (list prepend)
let cons x xs = x :: xs

test "cons" =
    assert (cons 1 [2, 3] == [1, 2, 3])
    assert (cons 0 [] == [0])


-- # Higher-order


-- | flip f x y == f y x
-- | Reverses argument order. foldl (flip cons) [] reverses a list.
let flip f x y = f y x

test "flip" =
    assert (flip sub 1 5 == 4)
    assert (flip cons [] 1 == [1])


-- | identity x == x
let identity x = x

test "identity" =
    assert (identity 42 == 42)
    assert (identity "hi" == "hi")


-- | const x _ == x  (ignores second argument)
let const x _ = x

test "const" =
    assert (const 0 999 == 0)
    assert (const "hi" false == "hi")
