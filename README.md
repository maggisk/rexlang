# RexLang

A functional programming language with algebraic data types, pattern matching, and Hindley-Milner type inference. Currently implemented as a tree-walking interpreter in Python; the long-term target is **WebAssembly (WasmGC)** — producing `.wasm` binaries that run natively in browsers and on servers via WASI with no runtime installation required.

## Quick start

```bash
cd python
python -m venv .venv && source .venv/bin/activate
pip install pytest

.venv/bin/python bin/main.py ../examples/factorial.rex   # run a file
.venv/bin/python bin/main.py --test ../examples/testing.rex  # run tests
.venv/bin/python bin/main.py                             # start the REPL
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

### Traits (typeclasses)

```
trait Describable a where
    describe : a -> String

impl Describable Int where
    describe n = "the number " ++ toString n
```

The prelude provides `Eq`, `Ord`, and `Ordering` with instances for `Int`, `Float`, `String`, and `Bool`.

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
.venv/bin/python bin/main.py --test myfile.rex
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

## Standard library

| Module | Contents |
|---|---|
| `std:List` | `map`, `filter`, `foldl`, `foldr`, `take`, `drop`, `reverse`, `append`, `concat`, `concatMap`, `zip`, `intersperse`, `partition`, `sum`, `product`, `any`, `all`, `isEmpty`, `repeat`, `range`, `head`, `tail`, `last`, `init`, `nth`, `find`, `indexedMap`, `maximum`, `minimum`, `length` |
| `std:Map` | AVL tree sorted map: `insert`, `lookup`, `remove`, `member`, `update`, `size`, `isEmpty`, `filter`, `map`, `foldl`, `foldr`, `fromList`, `toList`, `singleton`, `keys`, `values` |
| `std:Result` | `Ok`/`Err`, `map`, `mapErr`, `andThen`, `withDefault`, `isOk`, `isErr` |
| `std:Json` | `parse` (String → Result Json String), `stringify` (Json → String), `encodeArr`, `encodeObj`, `getField`, `arrayToList`, `listToArray`, `JNull`/`JBool`/`JNum`/`JStr`/`JArr`/`JObj` ADT |
| `std:String` | `length`, `toUpper`, `toLower`, `trim`, `split`, `join`, `toString`, `contains`, `startsWith`, `endsWith`, `isEmpty`, `charAt`, `substring`, `indexOf`, `replace`, `repeat`, `padLeft`, `padRight`, `words`, `lines`, `charCode`, `fromCharCode`, `parseInt`, `parseFloat` |
| `std:Math` | `abs`, `min`, `max`, `pow`, `sqrt`, trig, `log`, `exp`, `pi`, `e`, `clamp`, `degrees`, `radians`, `logBase` |
| `std:IO` | `readFile`, `writeFile`, `appendFile`, `fileExists`, `listDir` (all return `Result`) |
| `std:Env` | `getEnv` (returns `Maybe`), `getEnvOr`, `args` |

## Examples

| File | Description |
|---|---|
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
| `examples/import.rex` | Module imports (selective and qualified) |
| `examples/maybe.rex` | `Maybe` type from Prelude |
| `examples/io.rex` | File I/O with `Result` |
| `examples/string.rex` | String stdlib |
| `examples/math.rex` | Math stdlib |
| `examples/floats.rex` | Float arithmetic |
| `examples/modulo.rex` | Modulo operator |
| `examples/testing.rex` | Built-in test framework |

## Running tests

```bash
cd python
.venv/bin/pytest tests/ -q        # Python test suite (614 tests)
.venv/bin/python bin/main.py --test ../python/rexlang/stdlib/List.rex   # stdlib self-tests
.venv/bin/python bin/main.py --test ../python/rexlang/stdlib/Map.rex
.venv/bin/python bin/main.py --test ../python/rexlang/stdlib/Result.rex
.venv/bin/python bin/main.py --test ../python/rexlang/stdlib/Json.rex
```

## Roadmap

### Language
- [ ] Records — `{ name : String, age : Int }`, field access, update syntax
- [ ] String interpolation — `"hello ${name}"`
- [ ] Type aliases — `type Name = String`
- [ ] Traits v2 — parameterized instances, constraint propagation, `Show` trait
- [ ] Where clauses — `expr where x = …`
- [ ] Type annotations — optional `let f : Int -> Int`
- [ ] User modules — import your own `.rex` files

### Stdlib
- [x] JSON — `std:Json` with ADT, `parse`/`stringify`, encode/decode helpers
- [ ] JSON decoder combinators — Elm-style `field`, `map2`, `oneOf` for type-safe extraction
- [ ] Date/Time
- [ ] Random numbers

### Tooling
- [ ] `pyproject.toml` + installable `rexlang` CLI
- [ ] REPL history (`readline`)
- [ ] Better error messages with source locations

### Compilation
- [ ] IR design (A-normal form)
- [ ] WasmGC backend — emit WAT → `wasm-tools` → `.wasm`
- [ ] WASI output (servers/CLI)
- [ ] Browser ES module output
