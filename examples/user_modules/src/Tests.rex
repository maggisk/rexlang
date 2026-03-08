-- Tests for user module imports

import Utils (double, square, greet)
import Lib.Helpers as H
import Email (make, toString)

test "double" =
    assert double 5 == 10
    assert double 0 == 0

test "square" =
    assert square 3 == 9
    assert square 0 == 0

test "greet" =
    assert greet "Rex" == "Hello, Rex!"

test "qualified import" =
    assert H.sumDoubles [1, 2, 3] == 12
    assert H.squares [1, 2, 3] == [1, 4, 9]

test "opaque type via smart constructor" =
    let email = make "test@example.com"
    in assert toString email == "test@example.com"
