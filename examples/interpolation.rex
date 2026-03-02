-- String interpolation

let name = "Rex"
let version = 1
let pi = 3.14

test "basic interpolation" =
    assert ("Hello, ${name}!" == "Hello, Rex!")
    assert ("Version ${version}" == "Version 1")

test "expression interpolation" =
    assert ("Expr: ${1 + 2 + 3}" == "Expr: 6")

test "escape" =
    assert ("Escaped: \${not interpolated}" == "Escaped: \${not interpolated}")

test "bool and float" =
    assert ("Bool: ${true}" == "Bool: true")
    assert ("Pi: ${pi}" == "Pi: 3.14")

test "nested" =
    assert ("Nested: ${"hello " ++ show 42}" == "Nested: hello 42")

true
