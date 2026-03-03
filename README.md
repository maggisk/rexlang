# RexLang

> Twenty years of language design opinions, vibe coded into existence in days. Elm's elegance. Erlang's actors. Wasm's reach. One binary. No runtime, no dependencies, and no human who fully understands this codebase — only our new AI overlords.

A functional programming language with algebraic data types, pattern matching, and Hindley-Milner type inference. The current implementation is a Go tree-walking interpreter that ships as a single static binary — no runtime dependency. The long-term plan is a **WasmGC compilation backend** — producing `.wasm` binaries that run in browsers (native) and on servers via a Wasm runtime (Wasmtime, Wasmer, WasmEdge).

## Quick start

```bash
go build -o rex ./cmd/rex/

./rex examples/io.rex             # run a program (requires main)
./rex --test examples/factorial.rex  # run tests
./rex                             # start the REPL
```

## Language

### Primitives

```
42          -- Int
3.14        -- Float
"hello"     -- String
true        -- Bool
()          -- Unit

-- Number literal formats
0xFF        -- hex Int (255)
0o77        -- octal Int (63)
0b1010      -- binary Int (10)
1_000_000   -- underscores for readability
0xFF_00_FF  -- underscores work with all formats
3.141_592   -- underscores in floats too
```

### Let bindings and functions

```
let x = 42
let add x y = x + y              -- curried automatically
let rec fact n =
    if n == 0 then
        1
    else
        n * fact (n - 1)

-- Multi-binding let blocks
let a = 3
and b = 4
and square x = x * x
in
square a + square b
```

Definition order doesn't matter — top-level bindings are automatically sorted by dependency. Mutual recursion requires explicit `let rec … and …`; non-recursive multi-binding uses `let … and … in`.

```
let result = double 5             -- forward reference: double is defined below
let double x = x * 2
```

### Pattern matching

```
case shape of
    Circle r ->
        3.14159 * r * r
    Rect w h ->
        w * h
```

### Algebraic data types

```
type Shape = Circle float | Rect float float
type Tree a = Leaf | Node (Tree a) a (Tree a)
```

### Records

```
type Person = { name : String, age : Int }

let alice = Person { name = "Alice", age = 30 }
alice.name    -- "Alice"

-- update (creates a new record)
let bob = { alice | name = "Bob", age = 25 }

-- nested update via dot-path
type Address = { city : String, zip : String }
type PersonFull = { name : String, addr : Address }
let p2 = { person | addr.city = "LA" }

-- pattern matching
case alice of
    Person { name = n } ->
        n

-- parametric records
type Pair a b = { fst : a, snd : b }
let p = Pair { fst = 1, snd = "hello" }
p.fst    -- 1
```

Records are nominal — tied to a `type` declaration. The type name is required for construction and pattern matching. Updates with `{ rec | field = val }` are immutable — they return a new record. Nested dot-paths (`addr.city`) recursively update inner records.

### Type aliases

```
type Name = String
type Predicate a = a -> Bool
type Pair a b = (a, b)
type IntList = [Int]
```

Type aliases are transparent — `Name` and `String` are fully interchangeable. They support type parameters and work with annotations.

### Lists and tuples

```
let xs = [1, 2, 3]
let pair = (42, "hello")

case xs of
    [] ->
        0
    [h | t] ->
        h + sum t
```

### Pipe operator

```
[1, 2, 3, 4, 5]
    |> filter (\x -> x > 2)
    |> map (\x -> x * 10)
    |> sum
```

### String interpolation

```
let name = "Rex"
let version = 1
"Hello, ${name}! Version ${version}"    -- "Hello, Rex! Version 1"
"Escaped: \${not interpolated}"         -- "Escaped: ${not interpolated}"
"Expr: ${1 + 2 + 3}"                   -- "Expr: 6"
```

Expressions inside `${...}` are converted to strings via the `Show` trait. Strings without `${` are unchanged. Use `\$` to produce a literal `$`.

### Multi-line strings

```
let poem = """
Roses are red
Violets are blue
"""

let greeting = """
Hello, ${name}!
Welcome to RexLang.
"""
```

Triple-quoted strings (`"""..."""`) can span multiple lines. The first newline after the opening `"""` is stripped (so content starts on the next line). Escape sequences and `${expr}` interpolation work the same as regular strings. A lone `"` or `""` inside the string is fine — only `"""` closes it.

Use `dedent` from `std:String` to strip common leading whitespace:

```
import std:String (dedent)

let html = dedent """
    <div>
        <p>hello</p>
    </div>
    """
-- "<div>\n    <p>hello</p>\n</div>\n"
```

### Type annotations

Type annotations are optional — RexLang has full type inference. But they serve as documentation and catch mistakes early:

```
double : Int -> Int
let double x = x * 2

identity : a -> a
let identity x = x

fact : Int -> Int
let rec fact n =
    if n == 0 then
        1
    else
        n * fact (n - 1)
```

Annotations go on a separate line before the `let` binding. If the annotation doesn't match the inferred type, you get a clear error. Annotations can also constrain polymorphic types — `identity : Int -> Int` narrows `a -> a` to `Int -> Int`.

### Traits (typeclasses)

```
trait Describable a where
    describe : a -> String

impl Describable Int where
    describe n = "the number " ++ show n
```

The prelude provides `Eq`, `Ord`, `Show`, and `Ordering` with instances for `Int`, `Float`, `String`, and `Bool`.

### Imports and modules

```
import std:List (map, filter, foldl)
import std:Map as M

let m = M.fromList [("a", 1), ("b", 2)]
M.lookup "a" m    -- Just 1
```

### Entry point

Programs run with `./rex file.rex` need an `export let main` that takes command-line args and returns an exit code:

```
import std:IO (println)

export let main args =
    let _ = println "Hello, world!"
    in 0
```

`main` must have type `List String -> Int`. Use `_` to ignore args: `export let main _ = 0`.

Only declarations are allowed at the top level — bare expressions like `1 + 2` are rejected. The REPL is exempt from both rules.

### Built-in test framework

```
let double x = x * 2

test "double works" =
    assert (double 5 == 10)
    assert (double 0 == 0)
```

Run with `--test` (no `main` required):

```bash
./rex --test myfile.rex
```

Tests are parsed and type-checked in normal mode but only executed with `--test`. Test bodies are isolated — bindings don't leak.

### Error handling

IO functions return `Result` instead of raising; `getEnv` returns `Maybe`:

```
import std:Result (withDefault)

let contents = withDefault "" (readFile "data.txt")
```

### Comments

```
-- single line comment
```

## Style

Branch bodies always go on the next indented line — never on the same line as `->`, `then`, or `else`. Inspired by Elm.

```
if n == 0 then
    []
else
    n :: countdown (n - 1)
```

## Type safety

RexLang's type system (Hindley-Milner) catches type errors at compile time —
before your program runs. The goal is to eliminate runtime errors entirely.

What the type system catches today:

- Type mismatches — wrong argument types, applying non-functions, arithmetic on strings
- Unbound variables — referencing names that don't exist
- Module errors — importing non-existent modules or unexported names
- Annotation mismatches — declared type contradicts inferred type

What can still fail at runtime:

- **Division by zero** — `x / 0` (value-dependent, inherently runtime). Use `try` from `std:Result` to recover:

```
import std:Result (try, Ok, Err, DivisionByZero, ModuloByZero)

case try (\_ -> 10 / 0) of
    Ok n ->
        n
    Err (DivisionByZero) ->
        0
    Err (ModuloByZero) ->
        0
```

- **Missing trait instance** — calling a trait method on a type without an `impl` (constraint tracking planned in Traits v2)

Non-exhaustive patterns are caught at compile time — `case` expressions on ADTs, bools, and lists must cover all constructors, and literal/tuple patterns require a catch-all `_ ->` arm. Refutable patterns in `let` bindings are also rejected. IO operations like `readFile` and `getEnv` don't crash — they return `Result` or `Maybe`. Actor mailboxes are unbounded (Erlang-style) so `send` never fails.

## Standard library

| Module | Contents |
| --- | --- |
| `std:List` | `map`, `filter`, `foldl`, `foldr`, `take`, `drop`, `reverse`, `append`, `concat`, `concatMap`, `zip`, `intersperse`, `partition`, `sum`, `product`, `any`, `all`, `isEmpty`, `repeat`, `range`, `head`, `tail`, `last`, `init`, `nth`, `find`, `indexedMap`, `maximum`, `minimum`, `length` |
| `std:Map` | AVL tree sorted map: `insert`, `lookup`, `remove`, `member`, `update`, `size`, `isEmpty`, `filter`, `map`, `foldl`, `foldr`, `fromList`, `toList`, `singleton`, `keys`, `values` |
| `std:Result` | `Ok`/`Err`, `map`, `mapErr`, `andThen`, `withDefault`, `isOk`, `isErr`, `toMaybe`, `fromMaybe`, `try` (catch div/mod by zero), `RuntimeError` ADT |
| `std:Json` | `parse` (String → Result Json String), `stringify` (Json → String), `encodeArr`, `encodeObj`, `getField`, `arrayToList`, `listToArray`, `JNull`/`JBool`/`JNum`/`JStr`/`JArr`/`JObj` ADT |
| `std:String` | `length`, `toUpper`, `toLower`, `trim`, `split`, `join`, `toString`, `contains`, `startsWith`, `endsWith`, `isEmpty`, `charAt`, `substring`, `indexOf`, `replace`, `take`, `drop`, `repeat`, `padLeft`, `padRight`, `words`, `lines`, `charCode`, `fromCharCode`, `parseInt`, `parseFloat`, `dedent` |
| `std:Math` | `abs`, `min`, `max`, `pow`, `sqrt`, trig, `log`, `exp`, `pi`, `e`, `clamp`, `degrees`, `radians`, `logBase` |
| `std:IO` | `readFile`, `writeFile`, `appendFile`, `fileExists`, `listDir` (all return `Result`) |
| `std:Env` | `getEnv` (returns `Maybe`), `getEnvOr`, `args` |
| `std:Process` | `spawn`, `send`, `receive`, `self`, `call` — actor-model concurrency with typed messages |
| `std:Parallel` | `pmap`, `pmapN`, `numCPU` — parallel map over lists using actors; bounded parallelism via chunking |

## Examples

| File | Description |
| --- | --- |
| `examples/factorial.rex` | Recursive factorial |
| `examples/fibonacci.rex` | Recursive Fibonacci |
| `examples/adt.rex` | Algebraic data types |
| `examples/pattern_match.rex` | Pattern matching on multiple types |
| `examples/higher_order.rex` | Higher-order functions |
| `examples/pipe.rex` | Pipe operator `\|>` |
| `examples/list.rex` | List stdlib |
| `examples/tuple.rex` | Tuples and destructuring |
| `examples/mutual_recursion.rex` | Mutual recursion with `let rec … and` |
| `examples/traits.rex` | Trait declarations and implementations |
| `examples/map.rex` | `std:Map` sorted map |
| `examples/interpolation.rex` | String interpolation with `${expr}` |
| `examples/import.rex` | Module imports (selective and qualified) |
| `examples/maybe.rex` | `Maybe` type from Prelude |
| `examples/io.rex` | File I/O with `Result` |
| `examples/string.rex` | String stdlib |
| `examples/math.rex` | Math stdlib |
| `examples/floats.rex` | Float arithmetic |
| `examples/modulo.rex` | Modulo operator |
| `examples/annotations.rex` | Optional type annotations |
| `examples/type_alias.rex` | Type aliases: simple, parametric, function types |
| `examples/records.rex` | Records: creation, access, update, nested dot-paths |
| `examples/actors.rex` | Actor-model concurrency with `std:Process` |
| `examples/parallel.rex` | Parallel map with `std:Parallel` |
| `examples/multiline.rex` | Multi-line strings with `"""` |
| `examples/number_literals.rex` | Hex, octal, binary literals and underscore separators |
| `examples/let_block.rex` | Multi-binding let blocks with `and` |
| `examples/forward_ref.rex` | Forward references between top-level bindings |
| `examples/testing.rex` | Built-in test framework |

## Running tests

```bash
./rex --test internal/stdlib/rexfiles/*.rex
go test ./...
```

## Roadmap

### Language

- [x] Records — nominal records with field access, pattern matching, update syntax with nested dot-paths
- [x] String interpolation — `"hello ${name}"` with `Show` trait dispatch
- [x] Multi-line strings — `"""..."""` triple-quoted strings
- [x] Type aliases — `type Name = String`
- [ ] Traits v2 — parameterized instances, constraint propagation
- [x] Exhaustiveness checking — reject non-exhaustive `case` at compile time; refutable `let` patterns rejected
- [ ] Typed holes — `?name` in expression position; compiler infers the required type and reports it with in-scope bindings, enabling type-directed incremental development
- [x] Type annotations — optional `add : Int -> Int -> Int` before `let` binding
- [x] Multi-binding let — `let a = 1 and b = 2 in expr`
- [ ] User modules — import your own `.rex` files
- [ ] Opaque types — export a type without its constructor; consumers interact only through provided functions (`exposing (Email)` vs `exposing (Email(..))`). Prerequisite: user modules.

### Stdlib

- [x] JSON — `std:Json` with ADT, `parse`/`stringify`, encode/decode helpers
- [ ] JSON decoder combinators — Elm-style `field`, `map2`, `oneOf` for type-safe extraction
- [ ] Date/Time
- [ ] Random numbers

### Tooling

- [ ] Installable `rex` CLI (`go install`)
- [ ] REPL history (`readline`)
- [ ] Better error messages with source locations

### Compilation

- [ ] IR design (A-normal form)
- [ ] WasmGC backend — emit WAT → `wasm-tools` → `.wasm`
- [ ] WASI output (servers/CLI) and browser deployment via standard Wasm loader

## Simmering

Ideas worth keeping in mind but not yet committed to. May never happen.

- **Point-free operator sections** — named operator functions (`add`, `mul`, …) enable `foldl add 0` and `map (mul 2)` without lambdas, but asymmetric operators are a trap: `map (sub 1)` reads "subtract 1" but computes `\x -> 1 - x`. Several languages solve this differently: Haskell has `(- 1)` operator sections; Scala uses `_` as a positional placeholder so `map(_ - 1)` unambiguously means `\x -> x - 1`; Elixir uses `&` capture (`&(&1 - 1)`).

- **Extensible records (row polymorphism)** — functions over "any record with field `x`". Elm had these and [removed them in 0.19](https://elm-lang.org/news/small-assets-without-the-headache) because the complexity cost (error messages, type system machinery) outweighed the flexibility. Traits already cover many of the same use cases. WasmGC's fixed-layout structs also push against it. Worth revisiting only if plain records prove genuinely limiting in practice.
- **Hot module reloading** — WasmGC separates code from the GC-managed heap, which makes this more tractable than classic linear-memory Wasm. Live GC references are typed and runtime-managed, so a host could in theory transfer them from an old module instance to a new one. The open questions are type layout compatibility across versions and the lack of standardized dynamic linking in the Wasm spec today. Needs more research before committing.
- **Implicit mutual recursion** — currently, mutually recursive functions require explicit `let rec … and …`. Elm and Haskell treat all top-level bindings as mutually recursive by default. We could auto-detect cycles in the dependency graph and wrap them in implicit `LetRec` groups instead of erroring. Trade-off: simpler for users, but makes accidental cycles (typos, shadowing bugs) harder to catch.
- **Concurrency / actors** — already implemented via `std:Process` with Go goroutines. May swap internals for real WASI threads when the spec matures.
