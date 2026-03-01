-- Records: nominal record types with named fields

type Person = { name : String, age : Int }

let alice = Person { name = "Alice", age = 30 }

let getName p = p.name

test "record creation and access" =
    assert (alice.name == "Alice")
    assert (alice.age == 30)
    assert (getName alice == "Alice")

test "record pattern matching" =
    let msg = case alice of
        Person { name = n, age = a } ->
            n ++ " is " ++ show a
    assert (msg == "Alice is 30")

test "record equality" =
    let bob = Person { name = "Alice", age = 30 }
    assert (alice == bob)
    let carol = Person { name = "Carol", age = 25 }
    assert (alice /= carol)

test "partial pattern matching" =
    let justName = case alice of
        Person { name = n } ->
            n
    assert (justName == "Alice")

-- Parametric records
type Pair a b = { fst : a, snd : b }

let p = Pair { fst = 1, snd = "hello" }

test "parametric records" =
    assert (p.fst == 1)
    assert (p.snd == "hello")

test "parametric record pattern" =
    let result = case p of
        Pair { fst = x, snd = y } ->
            show x ++ " " ++ y
    assert (result == "1 hello")

p.fst
