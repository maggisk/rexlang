-- Short-circuit evaluation of && and ||
--
-- The right-hand side is only evaluated when the result is not
-- already determined by the left-hand side.

let rec loop = (\_ -> loop ())


test "false && <crash> does not evaluate right side" =
    assert ((false && loop ()) == false)

test "true || <crash> does not evaluate right side" =
    assert ((true || loop ()) == true)

test "true && false evaluates both sides" =
    assert ((true && false) == false)

test "false || true evaluates both sides" =
    assert ((false || true) == true)

test "chained && short-circuits at first false" =
    assert ((true && false && loop ()) == false)

test "chained || short-circuits at first true" =
    assert ((false || true || loop ()) == true)

test "practical: validate before compute" =
    import std:List (length)
    let isValid xs = length xs > 0 && length xs < 10
    assert (isValid [1, 2, 3])
    assert (not (isValid []))

true
