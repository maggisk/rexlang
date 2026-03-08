# RexLang

> Twenty years of language design opinions, vibe coded into existence in days. Elm's elegance. Erlang's actors. Wasm's reach. One binary. No runtime, no dependencies, and no human who fully understands this codebase — only our new AI overlords.

A functional programming language with algebraic data types, pattern matching, and Hindley-Milner type inference. The current implementation is a Go tree-walking interpreter that ships as a single static binary — no runtime dependency. The long-term plan is a **WasmGC compilation backend** — producing `.wasm` binaries that run in browsers (native) and on servers via a Wasm runtime (Wasmtime, Wasmer, WasmEdge).

## Quick start

```bash
go build -o rex ./cmd/rex/

./rex examples/io.rex             # run a program (requires main)
./rex --test examples/factorial.rex  # run tests
./rex --safe examples/io.rex      # --safe: reject any `todo` usage
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

### Bindings and functions

```
x = 42
add x y = x + y                  -- curried automatically
fact n =
    if n == 0 then
        1
    else
        n * fact (n - 1)          -- self-recursion works automatically

-- Mutual recursion — just define them as separate bindings
isEven n = if n == 0 then true else isOdd (n - 1)
isOdd n = if n == 0 then false else isEven (n - 1)
```

No `let` needed at the top level — just `name params = body`. Self-recursion and mutual recursion are handled automatically. Definition order doesn't matter — top-level bindings are sorted by dependency, and cycles are grouped into mutual recursion.

Inside expressions, use `let`/`let rec` with `in`:

```
-- Let-block: multiple bindings with a single `in`
let
    a = 3
    b = 4
    square x = x * x
in square a + square b

-- let rec for local recursion
let rec loop n = if n == 0 then 0 else loop (n - 1) in loop 10
```

### Pattern matching

```
match shape
    when Circle r ->
        3.14159 * r * r
    when Rect w h ->
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

alice = Person { name = "Alice", age = 30 }
bob = Person "Bob" 25           -- positional constructor
alice.name    -- "Alice"

-- update (creates a new record)
bob2 = { alice | name = "Bob", age = 25 }

-- nested update via dot-path
type Address = { city : String, zip : String }
type PersonFull = { name : String, addr : Address }
p2 = { person | addr.city = "LA" }

-- pattern matching
match alice
    when Person { name = n } ->
        n

-- parametric records
type Pair a b = { fst : a, snd : b }
p = Pair { fst = 1, snd = "hello" }
p.fst    -- 1
```

Records are nominal — tied to a `type` declaration. The type name doubles as a positional constructor function (`Person "Alice" 30`), which supports partial application and can be passed as a higher-order function (e.g., `map2 Person ...`). Updates with `{ rec | field = val }` are immutable — they return a new record. Nested dot-paths (`addr.city`) recursively update inner records.

### Type aliases

```
type alias Name = String
type alias Predicate a = a -> Bool
type alias Pair a b = (a, b)
type alias IntList = [Int]
```

Type aliases are transparent — `Name` and `String` are fully interchangeable. They support type parameters and work with annotations.

### Lists and tuples

```
xs = [1, 2, 3]
pair = (42, "hello")

match xs
    when [] ->
        0
    when [h | t] ->
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
name = "Rex"
version = 1
"Hello, ${name}! Version ${version}"    -- "Hello, Rex! Version 1"
"Escaped: \${not interpolated}"         -- "Escaped: ${not interpolated}"
"Expr: ${1 + 2 + 3}"                   -- "Expr: 6"
```

Expressions inside `${...}` are converted to strings via the `Show` trait. Strings without `${` are unchanged. Use `\$` to produce a literal `$`.

### Multi-line strings

```
poem = """
Roses are red
Violets are blue
"""

greeting = """
Hello, ${name}!
Welcome to RexLang.
"""
```

Triple-quoted strings (`"""..."""`) can span multiple lines. The first newline after the opening `"""` is stripped (so content starts on the next line). Escape sequences and `${expr}` interpolation work the same as regular strings. A lone `"` or `""` inside the string is fine — only `"""` closes it.

Use `dedent` from `Std:String` to strip common leading whitespace:

```
import Std:String (dedent)

html = dedent """
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
double x = x * 2

identity : a -> a
identity x = x

fact : Int -> Int
fact n =
    if n == 0 then
        1
    else
        n * fact (n - 1)
```

Annotations go on a separate line before the binding. If the annotation doesn't match the inferred type, you get a clear error. Annotations can also constrain polymorphic types — `identity : Int -> Int` narrows `a -> a` to `Int -> Int`.

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
-- Standard library (Std: namespace)
import Std:List (map, filter, foldl)
import Std:Map as M

m = M.fromList [("a", 1), ("b", 2)]
M.lookup "a" m    -- Just 1

-- User modules (resolved from src/ directory)
import Utils (double, greet)
import Lib.Helpers as H

H.sumDoubles [1, 2, 3]    -- 12
```

User modules use absolute paths from a `src/` directory in the project root. Dots map to directories: `import Lib.Helpers` resolves to `src/Lib/Helpers.rex`. The entry file must be inside `src/` for user module imports to work. Circular imports produce a clear error.

### Exports

Use `export` to make bindings visible to importers. Place it on its own line before the definition:

```
export add
add x y = x + y

helper x = x * 2       -- not exported (module-internal)
```

For types and traits, use `export type` / `export trait` inline:

```
export type Shape = Circle Float | Rect Float Float
```

### Entry point

Programs run with `./rex file.rex` need a `main` function that takes command-line args and returns an exit code:

```
import Std:IO (println)

main args =
    let _ = println "Hello, world!"
    in 0
```

`main` must have type `List String -> Int`. Use `_` to ignore args: `main _ = 0`.

Only declarations are allowed at the top level — bare expressions like `1 + 2` are rejected. The REPL is exempt from both rules.

### Built-in test framework

```
double x = x * 2

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
import Std:Result (withDefault)

contents = withDefault "" (readFile "data.txt")
```

### `todo` — development placeholders

Use `todo` as a placeholder for unfinished code. It type-checks as any type but throws at runtime:

```
handle x = todo "implement error handling"
```

The compiler warns whenever `todo` appears. Use `--safe` to promote warnings to errors — ideal for CI:

```bash
./rex --safe myprogram.rex          # errors on any todo
./rex --safe --test myfile.rex      # same for tests
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

- **`todo`** — development placeholder; throws at runtime but the compiler warns, and `--safe` rejects it entirely
- **Division by zero** — `x / 0` (value-dependent, inherently runtime). Use `try` from `Std:Result` to recover:

```
import Std:Result (try, Ok, Err, DivisionByZero, ModuloByZero)

match try (\_ -> 10 / 0)
    when Ok n ->
        n
    when Err (DivisionByZero) ->
        0
    when Err (ModuloByZero) ->
        0
```

- **Missing trait instance** — calling a trait method on a type without an `impl` (constraint tracking planned in Traits v2)

Non-exhaustive patterns are caught at compile time — `match` expressions on ADTs, bools, and lists must cover all constructors, and literal/tuple patterns require a catch-all `_ ->` arm. Refutable patterns in `let` bindings are also rejected. IO operations like `readFile` and `getEnv` don't crash — they return `Result` or `Maybe`. Actor mailboxes are unbounded (Erlang-style) so `send` never fails.

## Standard library

| Module | Contents |
| --- | --- |
| `Std:List` | `map`, `filter`, `foldl`, `foldr`, `take`, `drop`, `reverse`, `append`, `concat`, `concatMap`, `zip`, `intersperse`, `partition`, `sum`, `product`, `any`, `all`, `isEmpty`, `repeat`, `range`, `head`, `tail`, `last`, `init`, `nth`, `find`, `indexedMap`, `maximum`, `minimum`, `length` |
| `Std:Map` | AVL tree sorted map: `insert`, `lookup`, `remove`, `member`, `update`, `size`, `isEmpty`, `filter`, `map`, `foldl`, `foldr`, `fromList`, `toList`, `singleton`, `keys`, `values` |
| `Std:Maybe` | `Maybe`, `Just`, `Nothing`, `isNothing`, `isSome`, `fromMaybe`, `map`, `andThen`, `withDefault`, `filter`, `orElse` |
| `Std:Result` | `Ok`/`Err`, `map`, `mapErr`, `andThen`, `withDefault`, `isOk`, `isErr`, `try` (catch div/mod by zero), `RuntimeError` ADT |
| `Std:Convert` | `toResult` (Maybe→Result), `toMaybe` (Result→Maybe), `fromMaybe` (Maybe→Result) |
| `Std:Json` | `parse` (String → Result Json String), `stringify` (Json → String), `encodeNull`, `encodeBool`, `encodeNum`, `encodeStr`, `encodeArr`, `encodeObj`, `getField`; `type Json = JNull \| JBool Bool \| JStr String \| JNum Float \| JArr [Json] \| JObj [(String, Json)]` |
| `Std:Json.Decode` | Elm-style decoder combinators: `decodeString`, `field`, `at`, `index`, `string`, `int`, `float`, `bool`, `null`, `list`, `dict`, `map`, `map2`, `decode`, `with`, `andThen`, `oneOf`, `maybe`, `succeed`, `fail`; structured `DecodeError` record with path tracking; `errorToString` |
| `Std:String` | `length`, `toUpper`, `toLower`, `trim`, `split`, `join`, `toString`, `contains`, `startsWith`, `endsWith`, `isEmpty`, `charAt`, `substring`, `indexOf`, `replace`, `take`, `drop`, `repeat`, `padLeft`, `padRight`, `words`, `lines`, `charCode`, `fromCharCode`, `parseInt`, `parseFloat`, `dedent` |
| `Std:Math` | `abs`, `min`, `max`, `pow`, `sqrt`, trig, `log`, `exp`, `pi`, `e`, `clamp`, `degrees`, `radians`, `logBase` |
| `Std:IO` | `readFile`, `writeFile`, `appendFile`, `fileExists`, `listDir` (all return `Result`) |
| `Std:Env` | `getEnv` (returns `Maybe`), `getEnvOr`, `args` |
| `Std:Process` | `spawn`, `send`, `receive`, `self`, `call` — actor-model concurrency with typed messages |
| `Std:Parallel` | `pmap`, `pmapN`, `numCPU` — parallel map over lists using actors; bounded parallelism via chunking |
| `Std:Stream` | Lazy streams: `fromList`, `repeat`, `iterate`, `from`, `range`, `map`, `filter`, `flatMap`, `take`, `drop`, `takeWhile`, `dropWhile`, `zip`, `zipWith`, `toList`, `foldl`, `head`, `isEmpty`, `indexedMap` — supports infinite sequences |
| `Std:Net` | TCP networking: `tcpListen`, `tcpAccept`, `tcpConnect`, `tcpRead`, `tcpWrite`, `tcpClose`, `tcpCloseListener` — opaque `Listener`/`Conn` types; all operations return `Result` |

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
| `examples/mutual_recursion.rex` | Mutual recursion (auto-detected) |
| `examples/traits.rex` | Trait declarations and implementations |
| `examples/map.rex` | `Std:Map` sorted map |
| `examples/interpolation.rex` | String interpolation with `${expr}` |
| `examples/import.rex` | Module imports (selective and qualified) |
| `examples/maybe.rex` | `Maybe` type from `Std:Maybe` |
| `examples/io.rex` | File I/O with `Result` |
| `examples/string.rex` | String stdlib |
| `examples/math.rex` | Math stdlib |
| `examples/floats.rex` | Float arithmetic |
| `examples/modulo.rex` | Modulo operator |
| `examples/annotations.rex` | Optional type annotations |
| `examples/type_alias.rex` | Type aliases: simple, parametric, function types |
| `examples/records.rex` | Records: creation, access, update, nested dot-paths |
| `examples/actors.rex` | Actor-model concurrency with `Std:Process` |
| `examples/parallel.rex` | Parallel map with `Std:Parallel` |
| `examples/stream.rex` | Lazy streams with `Std:Stream` |
| `examples/multiline.rex` | Multi-line strings with `"""` |
| `examples/number_literals.rex` | Hex, octal, binary literals and underscore separators |
| `examples/let_block.rex` | Let-blocks with multiple bindings |
| `examples/forward_ref.rex` | Forward references between top-level bindings |
| `examples/testing.rex` | Built-in test framework |
| `examples/json_decode.rex` | JSON decoder combinators with `Std:Json.Decode` |
| `examples/tcp_echo.rex` | TCP echo server with `Std:Net` and `Std:Process` |
| `examples/user_modules/` | User module imports with `src/` directory |

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
- [x] Type aliases — `type alias Name = String`
- [ ] Traits v2 — parameterized instances, constraint propagation
- [x] Exhaustiveness checking — reject non-exhaustive `match` at compile time; refutable `let` patterns rejected
- [ ] Typed holes — `?name` in expression position; compiler infers the required type and reports it with in-scope bindings, enabling type-directed incremental development
- [x] Type annotations — optional `add : Int -> Int -> Int` before binding
- [x] Let-blocks — `let` with indented bindings terminated by `in`
- [x] Bare top-level bindings — `name params = body` (no `let` needed); implicit self and mutual recursion
- [x] `todo` builtin — development placeholder; `--safe` flag rejects it for CI/deploy
- [x] User modules — import your own `.rex` files from `src/` directory
- [ ] Opaque types — export a type without its constructor; consumers interact only through provided functions (`exposing (Email)` vs `exposing (Email(..))`). Prerequisite: user modules.

### Stdlib

- [x] JSON — `Std:Json` with ADT, `parse`/`stringify`, encode/decode helpers
- [x] JSON decoder combinators — `Std:Json.Decode` with Elm-style `field`, `map2`, `oneOf`, `andThen`, `list`, `dict`, `maybe`
- [x] TCP networking — `Std:Net` with `tcpListen`, `tcpAccept`, `tcpConnect`, `tcpRead`, `tcpWrite`, `tcpClose`, `tcpCloseListener`
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

- **Extensible records (row polymorphism)** — functions over "any record with field `x`". Elm has a restricted form (read-only narrowing, no field addition/deletion); PureScript has full row polymorphism. Elm's restricted version would compile to WasmGC via monomorphization (concrete record type known at each call site), so the compilation target isn't a blocker. The real cost is type system complexity — error messages get harder and the inference machinery grows. Traits already cover many of the same use cases. Worth revisiting only if plain records prove genuinely limiting in practice.
- **Hot module reloading** — WasmGC separates code from the GC-managed heap, which makes this more tractable than classic linear-memory Wasm. Live GC references are typed and runtime-managed, so a host could in theory transfer them from an old module instance to a new one. The open questions are type layout compatibility across versions and the lack of standardized dynamic linking in the Wasm spec today. Needs more research before committing.
- **Concurrency / actors** — already implemented via `Std:Process` with Go goroutines. May swap internals for real WASI threads when the spec matures.
- **List fusion** — the WasmGC compiler could detect `map |> filter |> take` chains and fuse them into a single pass, eliminating intermediate lists. Until then, an explicit `Std:Stream` module provides opt-in lazy evaluation. If fusion lands, `Stream` becomes unnecessary for finite pipelines (but remains useful for infinite sequences).
- **Tagged templates** — backtick-delimited literals with a tag prefix: `` html`<div onClick={handler}>{content}</div>` ``. The parser desugars the template into typed function calls at compile time (not a runtime string). Each `{expr}` hole is a real Rex expression, fully typechecked. Different tags (html, sql, regex, …) could provide different desugaring strategies. Backticks are only valid with a tag — no untagged template literals. Enables JSX-like HTML authoring without contaminating the core expression grammar.
