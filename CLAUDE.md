# CLAUDE.md — RexLang

> **Keep this file current.** Update CLAUDE.md whenever architecture changes, new conventions are established, key decisions are made, or planned work is completed or added. It is the primary source of truth for working in this repo.

## Project overview

RexLang is a functional language with algebraic data types and pattern matching. The current implementation is a Python tree-walking interpreter. The long-term plan is Hindley-Milner type inference and a **WasmGC compilation backend** — producing `.wasm` binaries that run in browsers (native) and on servers via WASI (Wasmtime/Wasmer/WasmEdge, no runtime install required).

## Repository layout

```
examples/          .rex example programs (one per feature)
python/
  bin/main.py      entry point: file runner + REPL (shows inferred types)
  rexlang/
    token.py       Token dataclass
    lexer.py       tokenizer → list[Token]
    ast.py         AST node dataclasses
    parser.py      recursive-descent parser → list[Expr]
    types.py       HM type representation (TVar, TCon, Scheme, unify, generalize, …)
    typecheck.py   Algorithm W inference; runs after parse, before eval; errors fatal
    values.py      value types (VInt, VFloat, …), Error, value_to_string, helpers
    eval.py        tree-walking evaluator (imports values + builtins)
    __init__.py    re-exports run(), run_program(), eval_toplevel()
    builtins/
      __init__.py  all_builtins() — assembles the full builtin dict
      core.py      not, error, print, println, readLine, toFloat, round, floor, ceiling, truncate
      math.py      abs, min, max, pow, sqrt, trig, log, exp, pi, e
      string.py    length, toUpper, toLower, trim, split, join, toString, contains, startsWith, endsWith
      io.py        readFile, writeFile, appendFile, fileExists, listDir
      env.py       getEnv, getEnvOr, args
    stdlib/
      List.rex     list stdlib (map, filter, foldl, foldr, take, drop, ...)
      Math.rex     math stdlib (abs, min, max, pow, trig, log, exp, pi, e, clamp, degrees, radians, logBase)
      String.rex   string stdlib (length, toUpper, toLower, trim, split, join, toString, contains, startsWith, endsWith, isEmpty)
      IO.rex       filesystem stdlib (readFile→Result, writeFile→Result, appendFile→Result, fileExists→Bool, listDir→Result)
      Env.rex      environment stdlib (getEnv→Maybe, getEnvOr, args)
      Result.rex   result stdlib (Ok, Err, map, mapErr, withDefault, isOk, isErr, andThen)
  tests/
    test_lexer.py
    test_parser.py
    test_eval.py   includes TestExampleFiles which runs examples/*.rex
    test_typecheck.py  HM inference tests (primitives, ADTs, polymorphism, errors, examples)
```

## Development commands

All commands run from `python/`:

```bash
# run a file
.venv/bin/python bin/main.py ../examples/factorial.rex

# REPL (blank line to eval, Ctrl-D to exit)
.venv/bin/python bin/main.py

# tests
.venv/bin/pytest tests/ -q

# format
.venv/bin/ruff format .

# lint
.venv/bin/ruff check .
```

## Architecture notes

- **Pipeline**: source → `lexer.tokenize()` → `parser.parse()` → `typecheck.check_program()` → `eval.eval_program()`
- **Type inference**: `typecheck.py` implements Algorithm W (Hindley-Milner); runs after parse, before eval; type errors are fatal. Types live in `types.py` (`TVar`, `TCon`, `Scheme`). Arithmetic operators (`+` `-` `*` `/`) require `Int` or `Float`; free type variables in arithmetic expressions default to `Int`. Use `toFloat` to convert before Float arithmetic. REPL shows `name : type` after each binding.
- **Values**: `VInt`, `VFloat`, `VString`, `VBool`, `VClosure`, `VCtor`, `VCtorFn`, `VBuiltin` — all are plain dataclasses with `__eq__`
- **Environment**: plain `dict` passed through eval; closures capture a snapshot
- **Tail calls**: the evaluator uses a trampoline loop for tail-recursive functions
- **ADTs**: `type Foo = A | B int` registers constructors (no `of`; type name must be uppercase); `type Foo a = …` for parametric ADTs; `TypeDecl.params` holds type parameter names; `TypeDecl.ctors` is `list[(ctor_name: str, arg_type_names: list[str])]`
- **Pipe** `|>`: left-associative, desugars to function application at eval time

## Conventions

- One feature = one commit
- Every new language feature needs: lexer token (if needed) + AST node + parser rule + eval case + tests + example file
- Example files in `examples/` end with a single expression whose value is asserted in `TestExampleFiles`
- `ruff format` before committing; `ruff check` should be clean
- Comments use `--` in `.rex` files; `#` in Python source

### `.rex` formatting style (Elm-inspired)

Branch bodies always go on the next indented line — never on the same line as `->`, `then`, or `else`:

```rex
-- case arms
case lst of
    [] ->
        0
    [h|t] ->
        1 + length t

-- if-then-else
if n == 0 then
    []
else
    case lst of
        ...
```

One blank line between top-level definitions; two blank lines between sections. Stdlib modules use `-- # Section` headers and `-- | doc` comments above each function.

## Planned work (ordered by dependency)

### Requires HM inference
- IR design (A-normal form; ADTs map to WasmGC `struct` subtypes)
- WasmGC backend: emit WAT (WebAssembly Text) → `wasm-tools` assemble → `.wasm`
- Full module system (`module Foo` declarations, third-party namespaces)

### Can land any time

### Before going public
- `pyproject.toml` + installable CLI (`rexlang` command)
- Ruff linting config
- Polish README (installation instructions, more examples)
- REPL history (`readline` + `~/.rexlang_history`)

## Key decisions already made

- **`()` unit**: zero-element tuple; `TUnit = TCon("Unit", [])` already existed; added `ast.Unit`, `ast.PUnit`, `VUnit`, `parse_atom`/`parse_atom_pattern` handling
- **Error handling**: IO functions return `Result ok String` instead of raising; `getEnv` returns `Maybe String`; use `std:Result` or `std:Maybe` to handle failures
- **Type system**: full Hindley-Milner inference, no annotations required
- **Compilation target**: WasmGC — emit WAT, assemble with `wasm-tools`. Runs in browsers natively and on servers via WASI (no runtime install). ADTs map to WasmGC `struct` subtypes; TCO via `return_call`.
- **Concurrency**: actors are a stdlib library, not a language feature. Start with a single-threaded cooperative scheduler (spawn/send/receive backed by message queues). Swap internals for real WASI threads when the spec matures — API stays the same.
- **No hot reloading** for now
- **Exhaustiveness checking**: static pass in `typecheck.py` (post-HM); `__ctor_families__` registry in type env tracks constructor siblings; `eval.py` has no `__types__` registry
- **No guards in pattern matching** (not planned)
- **Import system**: Two forms: `import std:List (map, filter)` — selective unqualified import; `import std:List as L` — qualified import, all exports via `L.map`, `L.length`, etc. `std:` namespace resolves to `python/rexlang/stdlib/`. Full `module Foo` declarations come after HM inference. `export name, ...` in module files declares public API.
- **`length` name collision**: resolved via qualified imports — `import std:List as L` and `import std:String as S` then use `L.length` vs `S.length`.
