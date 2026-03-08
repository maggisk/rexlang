-- Records: nominal record types with named fields

type Person = { name : String, age : Int }

alice = Person { name = "Alice", age = 30 }

getName p = p.name

test "record creation and access" =
    assert (alice.name == "Alice")
    assert (alice.age == 30)
    assert (getName alice == "Alice")

test "record pattern matching" =
    let msg = match alice
        when Person { name = n, age = a } ->
            n ++ " is " ++ show a
    assert (msg == "Alice is 30")

test "record equality" =
    let bob = Person { name = "Alice", age = 30 }
    assert (alice == bob)
    let carol = Person { name = "Carol", age = 25 }
    assert (alice != carol)

test "partial pattern matching" =
    let justName = match alice
        when Person { name = n } ->
            n
    assert (justName == "Alice")

-- Parametric records
type Pair a b = { fst : a, snd : b }

p = Pair { fst = 1, snd = "hello" }

test "parametric records" =
    assert (p.fst == 1)
    assert (p.snd == "hello")

test "parametric record pattern" =
    let result = match p
        when Pair { fst = x, snd = y } ->
            show x ++ " " ++ y
    assert (result == "1 hello")

-- Record update syntax
test "record update" =
    let bob = { alice | name = "Bob" }
    assert (bob.name == "Bob")
    assert (bob.age == 30)

test "record update multiple fields" =
    let bob = { alice | name = "Bob", age = 25 }
    assert (bob.name == "Bob")
    assert (bob.age == 25)

test "record update preserves original" =
    let bob = { alice | name = "Bob" }
    assert (alice.name == "Alice")
    assert (bob.name == "Bob")

test "record update with expression" =
    let older = { alice | age = alice.age + 1 }
    assert (older.age == 31)

test "parametric record update" =
    let p2 = { p | fst = 99 }
    assert (p2.fst == 99)
    assert (p2.snd == "hello")

-- Nested record update
type Address = { city : String, zip : String }
type PersonFull = { name : String, addr : Address }

test "nested record update" =
    let person = PersonFull { name = "Alice", addr = Address { city = "NYC", zip = "10001" } }
    let p2 = { person | addr.city = "LA" }
    assert (p2.addr.city == "LA")
    assert (p2.addr.zip == "10001")
    assert (p2.name == "Alice")

test "nested update multiple fields" =
    let person = PersonFull { name = "Alice", addr = Address { city = "NYC", zip = "10001" } }
    let p2 = { person | name = "Bob", addr.city = "LA" }
    assert (p2.name == "Bob")
    assert (p2.addr.city == "LA")
    assert (p2.addr.zip == "10001")
