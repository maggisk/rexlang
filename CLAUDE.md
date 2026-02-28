# CLAUDE.md — RexLang

> **Keep this file current.** Update CLAUDE.md whenever architecture changes, new conventions are established, key decisions are made, or planned work is completed or added. It is the primary source of truth for working in this repo.
>
> **Also update README.md** whenever a new language feature is added: add it to the Language or Standard library section, update the examples table if a new example file was created, and check items off (or remove them from) the Roadmap. The README is the public-facing feature list.

## Project overview

RexLang is a functional language with algebraic data types and pattern matching. The implementation is a Go tree-walking interpreter that ships as a single static binary — no runtime, no pip, no venv. The long-term plan is a **WasmGC compilation backend** — producing `.wasm` binaries that run in browsers (native) and on servers via WASI (Wasmtime/Wasmer/WasmEdge, no runtime install required).

## Repository layout

```
examples/          .rex example programs (one per feature)
stdlib/            .rex stdlib files (embedded in the binary via //go:embed)
  Prelude.rex      auto-loaded prelude (Maybe type, Ordering type, Eq/Ord traits + instances for Int/Float/String/Bool)
  List.rex         list stdlib
  Map.rex          sorted map stdlib — AVL tree using Ord trait
  Math.rex         math stdlib
  String.rex       string stdlib
  IO.rex           filesystem stdlib
  Env.rex          environment stdlib
  Result.rex       result stdlib
  Json.rex         JSON stdlib
go.mod             module github.com/maggisk/rexlang
cmd/
  rex/main.go      CLI: run file, --test, REPL
internal/
  lexer/           Token + Tokenize()
  ast/             All AST node types (Expr, Pattern, TySyntax interfaces)
  parser/          Recursive-descent parser; offside rule via caseArmCol
  types/           TVar, TCon, Scheme; Unify, Generalize, ApplySubst, etc.
  typechecker/     Algorithm W (check_program, check_module, prelude cache)
  eval/
    values.go      Value interface + all value types; StructuralEq, ValueToString
    eval.go        Eval(), EvalToplevel(), RunProgram(), RunTests(), REPL helpers
    builtins_core.go  All builtins: core, math, string, IO, env, JSON
  stdlib/
    embed.go       //go:embed rexfiles/*.rex; Source(name) string
    rexfiles/      .rex files for embedding
python/            Python reference implementation (kept for reference)
```

## Development commands

All commands run from the repo root:

```bash
# build
go build -o rex ./cmd/rex/

# run a file
./rex examples/factorial.rex

# run tests in a .rex file
./rex --test examples/testing.rex
./rex --test stdlib/List.rex

# REPL (blank line to eval, Ctrl-D to exit)
./rex

# run all Go tests
go test ./...

# format
gofmt -w .
```

## Architecture notes

- **Pipeline**: source → `lexer.Tokenize()` → `parser.Parse()` → `typechecker.CheckProgram()` → `eval.RunProgram()`
- **Language**: Go 1.24+. Single binary, no runtime dependency.
- **Type inference**: `internal/typechecker` implements Algorithm W (Hindley-Milner); runs after parse, before eval; type errors are fatal. Types in `internal/types` (`TVar`, `TCon`, `Scheme`). Arithmetic operators (`+` `-` `*` `/`) require `Int` or `Float`; free type variables in arithmetic expressions default to `Int`. Use `toFloat` to convert before Float arithmetic. REPL shows `name : type` after each binding.
- **Values**: `VInt`, `VFloat`, `VString`, `VBool`, `VClosure`, `VCtor`, `VCtorFn`, `VBuiltin`, `VTraitMethod`, `VInstances`, `VModule` — all implement `Value` interface via `valueKind()`.
- **Environment**: `Env = map[string]Value`; `Clone()` and `Extend()` for closure snapshots.
- **Tail calls**: the evaluator uses a trampoline `for {}` loop for tail-recursive functions.
- **ADTs**: `type Foo = A | B int` registers constructors; `type Foo a = …` for parametric ADTs.
- **Pipe** `|>`: left-associative, desugars to function application at eval time.
- **Traits**: `trait`/`impl` (Rust-style naming) for ad-hoc polymorphism. Single-parameter traits, runtime dispatch. `Prelude.rex` auto-loaded. Trait instances stored in `VInstances` keyed by `"TraitName:TypeName:MethodName"`.
- **Test framework**: `test "name" = body` / `assert expr`. `--test` flag runs them.
- **Stdlib embedding**: `.rex` files embedded via `//go:embed` in `internal/stdlib/embed.go`.

## Conventions

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

### Data structures & types
- [x] Map/Dict — `std:Map` AVL tree, sorted by `Ord` trait
- Records — `{ name : String, age : Int }`, field access, update syntax
- String interpolation — `"hello ${name}"` or similar
- Type aliases — `type Name = String` (lightweight, distinct from ADTs)
- Multi-line strings
- Number literals — hex, underscores (`1_000_000`)
- Char type vs expanded String — decide later

### Module system
- [x] Stdlib modules — `import std:List`, `import std:Map as M`, etc.
- User modules — import your own `.rex` files
- Opaque types — export a type without its constructor; consumers interact only through provided functions. Prerequisite: user modules. Syntax TBD.
- Package system — third-party dependencies

### Stdlib
- [x] List — map, filter, foldl, foldr, zip, concat, concatMap, range, repeat, find, partition, intersperse, indexedMap, maximum, minimum, …
- [x] Map — AVL tree sorted map (insert, lookup, remove, fold, …)
- [x] Result — Ok/Err, map, mapErr, andThen, withDefault
- [x] String — length, toUpper, toLower, trim, split, join, contains, charAt, substring, indexOf, replace, repeat, padLeft, padRight, words, lines, charCode, fromCharCode, parseInt, parseFloat, …
- [x] Math — abs, min, max, pow, trig, log, exp, pi, e, clamp, …
- [x] IO — readFile, writeFile, appendFile, fileExists, listDir (return Result)
- [x] Env — getEnv (Maybe), getEnvOr, args
- [x] Json — parse (Python-backed), stringify (pure Rex), Json ADT, encode/decode helpers
- JSON decoder combinators — Elm-style `field`, `map2`, `oneOf` for type-safe extraction
- Date/Time (even basic)
- Random numbers

### Language ergonomics
- [x] Traits v1 — `trait`/`impl`, runtime dispatch, `Eq`/`Ord` in Prelude
- [x] Test framework — `test "name" = …` / `assert expr`, `--test` flag
- Type annotations — optional `let f : Int -> Int`, documentation aid
- Traits v2 — parameterized instances (e.g., `impl Ord (List a)`), constraint tracking in types (`Ord a => ...`), `Show` trait

### Error experience
- Better error messages — source locations, span info
- Stack traces on runtime errors (maybe)

### Compilation
- IR design (A-normal form; ADTs map to WasmGC `struct` subtypes)
- WasmGC backend: emit WAT (WebAssembly Text) → `wasm-tools` assemble → `.wasm`

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
- **Traits v1**: `trait`/`impl` with Rust-style naming. Single-parameter traits only. Runtime dispatch (no type-level constraints). Prelude auto-loaded with `Ordering`, `Eq`, `Ord` and instances for `Int`, `Float`, `String`, `Bool`. Comparison operators (`<`, `>`, `<=`, `>=`) extended to String (lexicographic) and Bool (`false < true`). `where` is a keyword.
- **Test framework**: Zig-inspired `test`/`assert` keywords. `\r` is a supported string escape.
- **Structural equality**: `==` and `/=` work on any Rex value including lists, tuples, and ADTs (recursive structural comparison). This means `Just 42 == Just 42` works.
- **Mutual recursion in types**: `_preregister_types` pre-pass in `check_program`, `check_module`, `_load_prelude_tc` registers all TypeDecl names before resolving constructors, enabling mutually recursive ADTs.
- **std:Json**: `parse : String -> Result Json String` is Python-backed (`jsonParse` builtin in `builtins/json.py`). `stringify` is pure Rex. The Json ADT uses three mutually recursive types (`Json`, `JsonList`, `JsonObj`). `stringify` nests its helpers inside itself to avoid forward-reference issues. Json.rex imports `std:String (replace, toString)` for `escapeStr`.
- **Stdlib test runner**: `run_tests` in `eval.py` accepts `_extra_type_env`/`_extra_builtins` for stdlib module context. `main.py --test` detects stdlib paths and injects module builtins automatically. `test "name" = body` declares inline test blocks; `assert expr` checks a Bool at runtime. `--test` flag activates test runner; normal execution skips tests. Tests are type-checked in all modes but only evaluated in test mode. Test body env is isolated (bindings don't leak).
