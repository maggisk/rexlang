# RexLang

> Twenty years of language design opinions, vibe coded into existence in days. Elm's elegance. Erlang's actors. Wasm's reach. One binary. No runtime, no dependencies, no human who fully understands this codebase — only our AI overlords.

A functional programming language with algebraic data types, pattern matching, and Hindley-Milner type inference. The implementation is a Go tree-walking interpreter that ships as a single static binary — no runtime dependency. The long-term plan is a **WasmGC compilation backend** — producing `.wasm` binaries that run in browsers (native) and on servers via a Wasm runtime (Wasmtime, Wasmer, WasmEdge).

## Quick start

```bash
go build -o rex ./cmd/rex/

./rex examples/factorial.rex      # run a file
./rex --test examples/*.rex # run tests
./rex                             # start the REPL
```

## Language

### Primitives

```
42        -- Int
3.14      -- Float
"hello"   -- String
true      -- Bool
()        -- Unit
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

-- pattern matching
case alice of
    Person { name = n } ->
        n

-- parametric records
type Pair a b = { fst : a, snd : b }
let p = Pair { fst = 1, snd = "hello" }
p.fst    -- 1
```

Records are nominal — tied to a `type` declaration. The type name is required for construction and pattern matching.

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
    |> filter (fun x -> x > 2)
    |> map (fun x -> x * 10)
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

### Built-in test framework

```
let double x = x * 2

test "double works" =
    assert (double 5 == 10)
    assert (double 0 == 0)
```

Run with `--test`:

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
-- single line
(* block comment (* nested *) *)
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
- **Non-exhaustive patterns** — `case` without a matching arm (exhaustiveness checking planned)
- **Division by zero** — `x / 0` (value-dependent, inherently runtime)
- **Mailbox overflow** — actor mailbox exceeds 1024 messages

IO operations like `readFile` and `getEnv` don't crash — they return `Result` or `Maybe`.

## Standard library

| Module       | Contents                                                                                                                                                                                                                                                                                    |
| ------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `std:List`   | `map`, `filter`, `foldl`, `foldr`, `take`, `drop`, `reverse`, `append`, `concat`, `concatMap`, `zip`, `intersperse`, `partition`, `sum`, `product`, `any`, `all`, `isEmpty`, `repeat`, `range`, `head`, `tail`, `last`, `init`, `nth`, `find`, `indexedMap`, `maximum`, `minimum`, `length` |
| `std:Map`    | AVL tree sorted map: `insert`, `lookup`, `remove`, `member`, `update`, `size`, `isEmpty`, `filter`, `map`, `foldl`, `foldr`, `fromList`, `toList`, `singleton`, `keys`, `values`                                                                                                            |
| `std:Result` | `Ok`/`Err`, `map`, `mapErr`, `andThen`, `withDefault`, `isOk`, `isErr`                                                                                                                                                                                                                      |
| `std:Json`   | `parse` (String → Result Json String), `stringify` (Json → String), `encodeArr`, `encodeObj`, `getField`, `arrayToList`, `listToArray`, `JNull`/`JBool`/`JNum`/`JStr`/`JArr`/`JObj` ADT                                                                                                     |
| `std:String` | `length`, `toUpper`, `toLower`, `trim`, `split`, `join`, `toString`, `contains`, `startsWith`, `endsWith`, `isEmpty`, `charAt`, `substring`, `indexOf`, `replace`, `repeat`, `padLeft`, `padRight`, `words`, `lines`, `charCode`, `fromCharCode`, `parseInt`, `parseFloat`                  |
| `std:Math`   | `abs`, `min`, `max`, `pow`, `sqrt`, trig, `log`, `exp`, `pi`, `e`, `clamp`, `degrees`, `radians`, `logBase`                                                                                                                                                                                 |
| `std:IO`     | `readFile`, `writeFile`, `appendFile`, `fileExists`, `listDir` (all return `Result`)                                                                                                                                                                                                        |
| `std:Env`    | `getEnv` (returns `Maybe`), `getEnvOr`, `args`                                                                                                                                                                                                                                              |
| `std:Process`| `spawn`, `send`, `receive`, `self`, `call` — actor-model concurrency with typed messages                                                                                                                                                                                                    |

## Examples

| File                            | Description                              |
| ------------------------------- | ---------------------------------------- |
| `examples/factorial.rex`        | Recursive factorial                      |
| `examples/fibonacci.rex`        | Recursive Fibonacci                      |
| `examples/adt.rex`              | Algebraic data types                     |
| `examples/pattern_match.rex`    | Pattern matching on multiple types       |
| `examples/higher_order.rex`     | Higher-order functions                   |
| `examples/pipe.rex`             | Pipe operator `\|>`                      |
| `examples/list.rex`             | List stdlib                              |
| `examples/tuple.rex`            | Tuples and destructuring                 |
| `examples/mutual_recursion.rex` | Mutual recursion with `let rec … and`    |
| `examples/traits.rex`           | Trait declarations and implementations   |
| `examples/map.rex`              | `std:Map` sorted map                     |
| `examples/interpolation.rex`    | String interpolation with `${expr}`      |
| `examples/import.rex`           | Module imports (selective and qualified) |
| `examples/maybe.rex`            | `Maybe` type from Prelude                |
| `examples/io.rex`               | File I/O with `Result`                   |
| `examples/string.rex`           | String stdlib                            |
| `examples/math.rex`             | Math stdlib                              |
| `examples/floats.rex`           | Float arithmetic                         |
| `examples/modulo.rex`           | Modulo operator                          |
| `examples/annotations.rex`      | Optional type annotations                |
| `examples/records.rex`          | Nominal records with field access        |
| `examples/actors.rex`           | Actor-model concurrency with `std:Process` |
| `examples/testing.rex`          | Built-in test framework                  |

## Running tests

```bash
./rex --test internal/stdlib/rexfiles/*.rex
go test ./...
```

## Roadmap

### Language

- [x] Records — nominal records with field access, pattern matching
- [x] String interpolation — `"hello ${name}"` with `Show` trait dispatch
- [ ] Type aliases — `type Name = String`
- [ ] Traits v2 — parameterized instances, constraint propagation
- [x] Type annotations — optional `add : Int -> Int -> Int` before `let` binding
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

- **Extensible records (row polymorphism)** — functions over "any record with field `x`". Elm had these and [removed them in 0.19](https://elm-lang.org/news/small-assets-without-the-headache) because the complexity cost (error messages, type system machinery) outweighed the flexibility. Traits already cover many of the same use cases. WasmGC's fixed-layout structs also push against it. Worth revisiting only if plain records prove genuinely limiting in practice.
- **Hot module reloading** — WasmGC separates code from the GC-managed heap, which makes this more tractable than classic linear-memory Wasm. Live GC references are typed and runtime-managed, so a host could in theory transfer them from an old module instance to a new one. The open questions are type layout compatibility across versions and the lack of standardized dynamic linking in the Wasm spec today. Needs more research before committing.
- **Concurrency / actors** — already implemented via `std:Process` with Go goroutines. May swap internals for real WASI threads when the spec matures.
