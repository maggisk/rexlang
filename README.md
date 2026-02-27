# RexLang

A functional programming language with algebraic data types, pattern matching, and Hindley-Milner type inference. Currently implemented as a tree-walking interpreter in Python; the long-term target is **WebAssembly (WasmGC)** — producing `.wasm` binaries that run natively in browsers and on servers via WASI with no runtime installation required.

## Quick start

```bash
cd python
python -m venv .venv && source .venv/bin/activate
pip install pytest

.venv/bin/python bin/main.py ../examples/factorial.rex   # run a file
.venv/bin/python bin/main.py                             # start the REPL
```

## Language

### Primitives

```
42        -- Int
3.14      -- Float
"hello"   -- String
true      -- Bool
```

### Arithmetic and operators

```
1 + 2       -- 3
10 / 3      -- 3  (integer division)
3.14 * 2.0  -- 6.28
"foo" ++ "bar"  -- "foobar"
```

### Let bindings

```
let x = 42
let add x y = x + y
let rec fact n = if n == 0 then 1 else n * fact (n - 1)
```

### Functions

```
let double = fun n -> n * 2
let add x y = x + y    -- curried: add 1 is a function
```

### Pipe operator

```
3 |> inc |> double |> square    -- square(double(inc(3)))
```

### If-then-else

```
if x > 0 then
    "positive"
else
    "non-positive"
```

### Pattern matching

```
case n of
    0 ->
        "zero"
    1 ->
        "one"
    _ ->
        "many"
```

### Algebraic data types

```
type Shape = Circle of float | Rect of float float

let area s =
    case s of
        Circle r ->
            3.14159 * r * r
        Rect w h ->
            w * h
```

### Comments

```
-- single line
(* block comment (* nested *) *)
```

## Style

Branch bodies always go on the next indented line — never on the same line as `->`, `then`, or `else`. This applies to `case` arms, `if`/`then`/`else`, and lambda bodies in multi-line expressions. The style is inspired by Elm.

## Examples

| File | Description |
|---|---|
| `examples/factorial.rex` | Recursive factorial via pattern matching |
| `examples/fibonacci.rex` | Recursive fibonacci |
| `examples/adt.rex` | Algebraic data types and recursion |
| `examples/pattern_match.rex` | Pattern matching on multiple types |
| `examples/higher_order.rex` | Higher-order functions |
| `examples/pipe.rex` | Pipe operator `\|>` |
| `examples/floats.rex` | Float arithmetic and math builtins |

## Running tests

```bash
cd python
.venv/bin/pytest tests/ -q
```

## Roadmap

### Language features
- [ ] Modulo operator (`mod`)
- [ ] Built-in list type with `[1, 2, 3]` literals and `[h | t]` patterns
- [ ] Tuple type `(a, b, c)` with destructuring
- [ ] Let destructuring (`let (x, y) = pair`)
- [ ] Mutual recursion (`let rec f ... and g ...`)
- [ ] Hindley-Milner type inference
- [ ] Pattern match exhaustiveness checking

### Standard library
- [ ] Math: `abs`, `min`, `max`, `pow`, `sin`, `cos`, `log`, `exp`
- [ ] String: `length`, `toUpper`, `toLower`, `trim`, `split`, `join`, `toString`, `parseInt`
- [ ] List: `map`, `filter`, `foldl`, `foldr`, `zip`, `take`, `drop`, `reverse`
- [ ] I/O: explicit `print`, `readLine`

### Error quality
- [ ] Line/column numbers in all error messages
- [ ] Runtime call stack traces
- [ ] Pattern match exhaustiveness warnings

### Tooling
- [ ] `pyproject.toml` and installable CLI
- [ ] Ruff linting
- [ ] REPL history persistence
- [ ] Module system (`module Foo`, `import Bar`)

### Compilation
- [ ] Intermediate representation (IR)
- [ ] WasmGC backend — emit WAT, assemble with `wasm-tools` → `.wasm`
- [ ] WASI output mode (server/CLI)
- [ ] Browser ES module output mode

### Concurrency (stdlib, not language features)
- [ ] Single-threaded actor scheduler (spawn/send/receive via cooperative multitasking)
- [ ] Upgrade to real WASI threads when spec matures
