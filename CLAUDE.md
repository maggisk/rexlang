# CLAUDE.md — RexLang

> **Keep this file current.** Update CLAUDE.md whenever architecture changes, new conventions are established, key decisions are made, or planned work is completed or added. It is the primary source of truth for working in this repo.
>
> **Also update README.md** whenever a new language feature is added: add it to the Language or Standard library section, update the examples table if a new example file was created, and check items off (or remove them from) the Roadmap. The README is the public-facing feature list.

## Project overview

RexLang is a functional language with algebraic data types and pattern matching. `rex file.rex` compiles to Go behind the scenes, builds a native binary, and runs it. Ships as a single static binary — Go is the only dependency. Additional compilation targets: **JavaScript** (browser) and **WasmGC** (in progress).

## Language goals

- **No runtime errors** — the type system should catch mistakes at compile time; the ideal is an Elm-style guarantee where a well-typed program cannot crash at runtime
- **Readability** — code should be easy to read and understand without knowing the language deeply; a good target for AI code generation
- **Mainstream over ML** — when a design choice comes up, prefer the convention from mainstream languages (TypeScript, Rust, Python, Go) over ML-family tradition (Haskell, OCaml, SML); RexLang is not trying to be another ML

## Repository layout

```
examples/          .rex example programs (one per feature)
go.mod             module github.com/maggisk/rexlang
cmd/
  rex/main.go      CLI: run, build, test, compile, install, init
internal/
  lexer/           Token + Tokenize()
  ast/             All AST node types (Expr, Pattern, TySyntax interfaces)
  parser/          Recursive-descent parser; offside rule via caseArmCol
  types/           TVar, TCon, Scheme; Unify, Generalize, ApplySubst, etc.
  typechecker/     Algorithm W (check_program, check_module, prelude cache)
  ir/              A-normal form intermediate representation; Lowerer (AST → ANF)
  codegen/
    golang.go      Go backend: IR → Go source → go build → native binary
    javascript.go  JS backend: IR → .js + .html for browser
    wat.go         WasmGC backend: IR → WAT → wasm-tools → .wasm
    runtime.go     Shared Go runtime (embedded into generated code)
  stdlib/
    embed.go       //go:embed all:rexfiles; Source(name), GoCompanion(name)
    rexfiles/      .rex stdlib files + companion .go files for builtins
```

## Development commands

All commands run from the repo root:

```bash
# build the rex CLI
go build -o rex ./cmd/rex/

# run a program (compiles to Go behind the scenes)
./rex examples/io.rex
./rex examples/actors.rex arg1 arg2

# build a standalone binary
./rex build src/App.rex              # produces ./app
./rex build --out=server src/App.rex # produces ./server

# run tests in a .rex file (no main required)
./rex --test examples/testing.rex
./rex --test --only="double" examples/testing.rex
./rex --test internal/stdlib/rexfiles/List.rex

# compile to JavaScript (browser target)
./rex --compile --target=browser src/Main.rex  # produces Main.js + Main.html

# compile to WebAssembly (requires wasm-tools)
./rex --compile examples/hello.rex    # produces hello.wasm + hello.wat

# --safe flag: promote warnings (todo usage) to errors
./rex --safe examples/io.rex
./rex --safe --test examples/testing.rex

# show inferred types
./rex --types src/App.rex
./rex --types Std:List

# package management
./rex init                              # create rex.toml
./rex install                           # fetch all dependencies
./rex install https://github.com/u/pkg main  # add git dep
./rex install ../mylib                  # add local path dep

# run all tests (build + rex tests + go tests)
make test

# format
gofmt -w .
```

## Architecture notes

- **Pipeline**: source → `lexer.Tokenize()` → `parser.Parse()` → `ValidateToplevel()` → `ValidateIndentation()` → `ReorderToplevel()` → `typechecker.CheckProgram()` → validate `main : List String -> Int` → `ir.Lower()` → `codegen.EmitGo()` → `go build` → execute binary
- **Build output**: generated Go source goes to `.cache/rex-build/` (next to `rex.toml` or in cwd). Directory is wiped each run; Go's own content-based build cache makes repeated builds fast.
- **Top-level restriction**: only declarations allowed at top level (bare bindings `name params = body`, `let`, `type`, `trait`, `impl`, `import`, `export`, `test`, type annotations). Bare expressions are rejected.
- **Top-level bindings**: bare `name params = body` at top level (no `let` needed). Parser detects lowercase ident followed by `[ident]* =` and produces `Let{Recursive: true}`. All top-level `let` bindings also auto-set `Recursive: true`. Mutual recursion between top-level bindings is detected automatically by `ReorderToplevel` (cycle detection groups them into `LetRec`). `let`/`let rec` remain for expression-level bindings only.
- **`main` entry point**: programs run with `./rex file.rex` must define `export main args = ...` where `main : List String -> Int`. `args` receives command-line arguments as a list of strings. The return value is the process exit code. `--test` mode does not require `main`.
- **Language**: Go 1.24+. Single binary, Go is the only dependency.
- **Type inference**: `internal/typechecker` implements Algorithm W (Hindley-Milner); runs after parse, before codegen; type errors are fatal. Types in `internal/types` (`TVar`, `TCon`, `Scheme`). Arithmetic operators (`+` `-` `*` `/`) require `Int` or `Float`; free type variables in arithmetic expressions default to `Int`. Use `toFloat` to convert before Float arithmetic.
- **Go codegen**: all values are `any` at the Go level. ADTs compile to Go interfaces + structs. Records compile to Go structs. Lists are cons cells (`*RexList`). Actors use goroutines + channels. Currying via nested closures.
- **External builtins**: `external name : Type` in .rex files declares Go-implemented functions. Companion `.go` files in `internal/stdlib/rexfiles/` provide the implementations (e.g., `Std_String_length`). The codegen emits thin `rex_*` wrappers that convert between `any` and typed Go parameters.
- **Tail calls**: Go backend uses a trampoline `for {}` loop for self-recursive tail calls.
- **Type aliases**: `type alias Name = String` — transparent alias, fully interchangeable at the type level. Parametric: `type alias Pair a b = (a, b)`. Stored in `tc.typeAliases` (`TypeAliasInfo{Params, Body}`); non-parametric aliases also stored in `typeDefs` for direct lookup. The `alias` keyword after `type` unambiguously distinguishes aliases from ADTs (no heuristic needed). No runtime effect.
- **ADTs**: `type Foo = A | B int` registers constructors; `type Foo a = …` for parametric ADTs.
- **Records**: `type Person = { name : String, age : Int }` — nominal record types tied to `type` declarations. Construction: `Person { name = "Alice", age = 30 }` or positional: `Person "Alice" 30`. The type name is a positional constructor function that supports currying and can be passed as a higher-order function (e.g., `map2 Person ...`). Field access: `p.name` (chained: `p.addr.city`; lowercase `.` produces `FieldAccess`; uppercase `.` produces `DotAccess` for modules). Update: `{ alice | name = "Bob" }` — creates a new record with changed fields. Nested dot-path updates: `{ model | user.name = "Alice" }` — recursively clones and updates nested records. Pattern matching: `Person { name = n, age = a }` (partial patterns OK). Parametric records: `type Pair a b = { fst : a, snd : b }`. Typechecker infers record type from field name when the expression type is a TVar. Field metadata stored in `__record_fields__` registry (keyed by type name → `RecordInfo`). Module imports propagate `__record_fields__` and `TypeDefs` via `ModuleResult`, so record types defined in imported modules can be constructed, accessed, and updated by the importer.
- **Let-block**: `let` on its own line followed by indented bindings, terminated by `in`. Parser-only — desugars to nested `Let` AST nodes. Detected when the token after `let` is on a different line and indented. `and` is only for `let rec ... and ...` mutual recursion.
- **`let` requires `in`**: In expression contexts (match arms, function bodies, lambdas), `let x = expr in body` requires explicit `in`. `parseLetBody` bounds the RHS with `caseArmCol = letCol` so the body can't eat tokens at the `let` column or before — this prevents greedy parsing and gives clear errors when `in` is missing. Top-level and test-body `let` bindings (`Let{InExpr: nil}`) are unaffected — those contexts loop over independent expressions.
- **Pipe** `|>`: left-associative, desugars to function application.
- **Trailing lambda**: when `\` appears after a function application (not inside `isAtomStart`), `parseApp()` treats it as the last argument. Sets `caseArmCol` to the function head's column so the lambda body terminates when indentation drops. Enables `Decoder \json -> ...` and `spawn \_ -> ...` without wrapping parens. One-line lambdas still use parens.
- **Traits**: `trait`/`impl` (Rust-style naming) for ad-hoc polymorphism. Single-parameter traits, runtime dispatch. `Prelude.rex` auto-loaded with `Eq`, `Ord`, `Show` and instances for primitives, lists, tuples, and unit. Parameterized instances: `impl Show (List a)`, `impl Eq (Maybe a)`, etc. — `ImplDecl.TargetType` is `TySyntax`; dispatch matches outer type name (`"List"`, `"Maybe"`, `"Tuple2"`). `__ctor_types__` map resolves constructors to type names for dispatch. Trait instances keyed by `"TraitName:TypeName:MethodName"`. **Compile-time constraint tracking**: `Scheme.Constraints` tracks trait requirements on type variables. Constraints are inferred automatically when trait methods are called (e.g., `compare` → `Ord`), propagated through function calls, and checked when type variables resolve to concrete types. Optional `Ord a => ...` annotation syntax supported. `TokFatArrow` (`=>`), `TyConstrained` AST node, `TyConstraint`. Note: `==`/`!=`/`<`/`>` operators use built-in structural equality/comparison, NOT trait dispatch, so they don't generate constraints.
- **String interpolation**: `"hello ${expr}"` — lexer emits `TokInterp` with `[]InterpPart`; parser produces `ast.StringInterp{Parts}`; `Show` trait dispatches conversion for non-string parts. `\$` escapes literal `$`. Nested interpolation (`"${f "inner ${x}"}"`) supported via mutual recursion in lexer (`skipInterp`/`skipString`).
- **Multi-line strings**: `"""..."""` triple-quoted strings. Lexer-only feature — produces same `TokString`/`TokInterp` tokens. First newline after opening `"""` stripped. Lone `"` or `""` inside allowed; only `"""` closes.
- **Type annotations**: `name : TypeSig` on a separate line before the binding. Parser detects lowercase ident + `:` in `ParseTokens`. Typechecker stores annotation as `__ann:name` in env; `toplevelLet` checks inferred type against annotation via `checkAnnotation()` (instantiate both, unify). If annotation exists and matches, it replaces the generalized type in env (constraining polymorphism).
- **Test framework**: `test "name" = body` / `assert expr`. `--test` flag runs them.
- **Stdlib embedding**: `.rex` files embedded via `//go:embed` in `internal/stdlib/embed.go`.

## Conventions

- Every new language feature needs: lexer token (if needed) + AST node + parser rule + IR lowering + codegen + tests + example file
- Example files in `examples/` are test-only (run with `--test`); programs that produce output have `export main`
- No bare expressions at top level in `.rex` files — only declarations
- `gofmt -w .` before committing
- Comments use `--` in `.rex` files
- **Never commit or push unless explicitly asked** — each request is one-off, not a standing instruction
- **Prefer the pipe operator `|>`** when writing Rex code — use it to make data flow read left-to-right instead of deeply nesting function calls. E.g., `list |> map f |> filter g` over `filter g (map f list)`.

### `.rex` formatting style (Elm-inspired)

Branch bodies always go on the next indented line — never on the same line as `->`, `then`, or `else`:

```rex
-- match arms
match lst
    when [] ->
        0
    when [h|t] ->
        1 + length t

-- if-then-else
if n == 0 then
    []
else
    match lst
        when ...
```

Trailing lambda: when a multi-line lambda is the last (or sole) argument to a function, write it without parens — the parser bounds the body by indentation (same `caseArmCol` mechanism as `match`/`test`). One-line lambdas keep parens.

```rex
-- good: trailing lambda (multi-line, last argument)
Decoder \json ->
    match json
        when JStr s ->
            Ok s
        when _ ->
            Err (DecodeError { path = [], message = "expected a String", value = json })

-- good: one-line lambda keeps parens
map (\x -> x + 1) list

-- good: non-last argument keeps parens
map (\x -> f x) list
```

`in` always goes on its own line — never at the end of the binding line. Chained `let ... in let ... in` should use let-block syntax instead.

```rex
-- bad: in at end of line
let msg = receive () in
send me msg

-- good: in on its own line
let msg = receive ()
in send me msg

-- bad: chained let-in
let _ = println "hello" in
let x = computeStuff () in
x

-- good: let-block
let
    _ = println "hello"
    x = computeStuff ()
in x

-- good: let-block, multi-line continuation
let
    _ = println "hello"
    x = computeStuff ()
in
match x
    when Ok v ->
        v
    when Err _ ->
        0
```

One blank line between top-level definitions; two blank lines between sections. Stdlib modules use `-- # Section` headers and `-- | doc` comments above each function. Every stdlib function should have its tests immediately after its definition — not grouped at the bottom of the file.

Exports: `export` on its own line above each exported function. For types, `export` is inline: `export type Foo = ...` or `export opaque type Foo = ...`. Standalone `export name, ...` lists only for re-exporting builtins (e.g., IO.rex, Env.rex).

```rex
-- good: export above function
export
rngMake : Int -> Rng
rngMake seed = ...

-- good: export inline with type
export type Rng = | Rng Int
export opaque type Rng = | Rng Int
```

## Planned work (ordered by dependency)

### Data structures & types

- [x] Map/Dict — `Std:Map` AVL tree, sorted by `Ord` trait
- [x] Records — `type Person = { name : String, age : Int }`, field access, pattern matching, update syntax `{ rec | field = val }` with nested dot-paths
- [x] String interpolation — `"hello ${name}"` with `Show` trait dispatch
- [x] Type aliases — `type alias Name = String`
- [x] Multi-line strings — `"""..."""` triple-quoted, first newline stripped, escapes and interpolation work as normal
- [x] Number literals — hex (`0xFF`), octal (`0o77`), binary (`0b1010`), underscores (`1_000_000`)
- Char type vs expanded String — decide later

### Module system

- [x] Stdlib modules — `import Std:List`, `import Std:Map as M`, etc.
- [x] User modules — `import Utils`, `import Lib.Helpers as H`; resolved from `src/` directory in cwd; dots map to directories; circular imports detected
- [x] Opaque types — `export opaque type Email = Email String`; type name available for annotations, constructors hidden from importers
- [x] Package system — `rex.toml` with git and path dependencies; `rex init`, `rex install`; package namespace in imports (`import pkg:Module`)

### Stdlib

- [x] List — map, filter, foldl, foldr, zip, concat, concatMap, range, repeat, find, partition, intersperse, indexedMap, maximum, minimum, …
- [x] Map — AVL tree sorted map (insert, lookup, remove, fold, …)
- [x] Result — Ok/Err, map, mapErr, andThen, withDefault, try (catch div/mod by zero), RuntimeError ADT
- [x] String — length, toUpper, toLower, trim, split, join, contains, charAt, substring, indexOf, replace, repeat, padLeft, padRight, words, lines, charCode, fromCharCode, parseInt, parseFloat, …
- [x] Math — abs, min, max, pow, trig, log, exp, pi, e, clamp, …
- [x] IO — readFile, writeFile, appendFile, fileExists, listDir (return Result)
- [x] Env — getEnv (Maybe), getEnvOr, args
- [x] Json — parse (Go-backed), stringify (pure Rex), Json ADT, encode/decode helpers
- [x] Process — actor model: `spawn`, `send`, `receive`, `self`, `call`; unbounded FIFO mailboxes (Erlang-style); `Pid a` opaque type; require `import Std:Process`
- [x] Parallel — `pmap`, `pmapN`, `numCPU`; parallel map over lists using actors; bounded parallelism via chunking
- [x] Json.Decode — Elm-style decoder combinators: `decodeString`, `field`, `at`, `index`, `string`, `int`, `float`, `bool`, `null`, `list`, `dict`, `map`, `map2`, `decode`, `with`, `andThen`, `oneOf`, `maybe`, `succeed`, `fail`; structured `DecodeError` record (`path`, `message`, `value`) with path tracking through `field`/`index`/`list`/`dict`/`optionalField`; `errorToString` for human-readable messages
- [x] Convert — `toResult` (Maybe→Result), `toMaybe` (Result→Maybe), `fromMaybe` (Maybe→Result); cross-conversion between Maybe and Result
- [x] Stream — lazy streams via thunks (`type Stream a = Empty | Cons a (() -> Stream a)`); pure Rex, no Go builtins; `fromList`, `repeat`, `iterate`, `from`, `range`, `map`, `filter`, `flatMap`, `take`, `drop`, `takeWhile`, `dropWhile`, `zip`, `zipWith`, `toList`, `foldl`, `head`, `isEmpty`, `indexedMap`; supports infinite sequences
- [x] Net — TCP networking: `tcpListen`, `tcpAccept`, `tcpConnect`, `tcpRead`, `tcpWrite`, `tcpClose`, `tcpCloseListener`; opaque `Listener` and `Conn` types; all operations return `Result`; `tcpListen` returns `(Listener, Int)` tuple for port-0 usage
- [x] Random — `Std:Random` with pure seed-based API (`rngMake`, `rngInt`, `rngFloat`, `rngBool`, `rngList`) and actor facade (`randomInt`, `randomFloat`, `randomBool`, `shuffle`); xorshift32 algorithm; opaque `Rng` type; one Go builtin (`systemSeed`)
- [x] Bitwise — `Std:Bitwise` with `bitAnd`, `bitOr`, `bitXor`, `bitNot`, `shiftLeft`, `shiftRight`; all operate on `Int`; Go builtins
- [x] DateTime — `Std:DateTime` with `Instant`/`Duration` opaque types, `DateTimeParts` record, `Weekday` ADT; pure Rex calendar math (Hinnant algorithm), formatting/parsing; 2 Go builtins (`dateTimeNow`, `dateTimeUtcOffset`)
- [x] Js — `Std:Js` generic JS FFI primitives: `JsRef` opaque type, `jsGlobal`, `jsGet`, `jsSet`, `jsCall`, `jsNew`, `jsCallback`, `jsFrom*`/`jsTo*` conversions, `jsNull`; browser-only (overlay-only via `Js.browser.rex`); JS codegen replaces calls with inline JS
- Html and Browser moved to `tea-rex` web framework package (separate repo)

### Language ergonomics

- [x] Traits v1 — `trait`/`impl`, runtime dispatch, `Eq`/`Ord` in Prelude
- [x] Test framework — `test "name" = …` / `assert expr`, `--test` flag
- [x] Type annotations — optional `add : Int -> Int -> Int` before `let` binding
- [x] Let-blocks — `let` + indented bindings + `in` (parser-only desugaring)
- [x] Traits v2 Phase 1 — parameterized instances (`impl Show (List a)`, `impl Eq (Maybe a)`, etc.); runtime dispatch on outer type name; no constraint syntax yet
- [x] Traits v2 Phase 2 — constraint tracking in types (`Ord a => ...`), compile-time enforcement
- [x] Exhaustiveness checking — static pass post-HM using `__ctor_families__`; rejects non-exhaustive `match` (ADTs, bools, lists require all constructors; literals/tuples require catch-all `_ ->`); refutable `let` patterns rejected via `isIrrefutable` check
- ~~Typed holes~~ — skipped; better suited as a language server / LSP feature than a CLI compiler feature. Revisit if/when LSP is built.

### Error experience

- [x] Type error line numbers — `TypeError` carries `Line`; AST nodes (`App`, `Binop`, `Fun`, `ListLit`, `TupleLit`, `RecordCreate`, `FieldAccess`, `RecordUpdate`, `StringInterp`, `UnaryMinus`) now track source line; `infer()` defer-wraps errors with line info from expression context
- Better error messages — span info, column info, source snippets (follow-up)
- Stack traces on runtime errors (maybe)

### Compilation (WasmGC backend)

Ordered by dependency — each step builds on the previous:

1. [x] **IR (A-normal form)** — lower typechecked AST to ANF where every subexpression is named; carry type annotations for codegen; pattern match compilation to decision trees
2. [x] **Toolchain bootstrap** — `--compile` flag emits WAT, assembles with `wasm-tools`; end-to-end tests with Wasmtime; `main _ = <int-expr>` works with arithmetic, if/else, let bindings
3. [x] **Primitives + arithmetic** — Int (`i64`), Float (`f64`), Bool (`i32`); arithmetic, comparison, logical operators; type-driven instruction selection in codegen
4. [x] **Functions + closures** — calling convention, closure structs (funcref + captured env), currying via partial application
5. [x] **ADTs + pattern matching** — `struct` subtypes with tag field, branch on tag + downcast; exhaustiveness already checked
6. [x] **Strings** — WasmGC `(array (mut i8))` for UTF-8 bytes; data segments for literals; `$string_eq` byte-by-byte comparison; string patterns in match
7. [x] **Lists, tuples** — cons-cell lists (`$list`/`$list_cons` subtypes with tag+head+tail); tuple structs by arity; `PNil`/`PCons`/`PTuple` pattern matching; polymorphic elements via anyref boxing
8. [x] **Tail calls** — `return_call` for TCO
9. [x] **Polymorphic boxing** — box primitives to `anyref` (`$box_i64`, `$box_f64`, `ref.i31` for Bool); unbox at use sites; enables polymorphic data structures and trait dispatch
10. [x] **Traits** — static dispatch when type is known at compile time; runtime dispatch via `br_on_cast` + `call_ref` when polymorphic; per-ADT supertypes for type testing; resolve functions return reusable funcrefs
11. [ ] **Stdlib** — recompile pure Rex stdlib; WASI host imports for IO/Net/Env; JS host imports for browser (Temporal API for DateTime)
12. [ ] **Actors** — depends on Wasm stack switching proposal

Key design decisions:
- **Polymorphism**: box type variables to a common `anyref` representation (simpler) vs monomorphization (faster); start with boxing
- **Closures**: every function potentially partially applied; uncurry optimization where arity is known at call site
- **Two deployment targets**: WASI (server/CLI via Wasmtime/Wasmer) and browser (JS host provides IO + DOM)

### Compilation (Go backend)

Compile Rex to Go source code, then `go build` to produce a native binary. Reuses the existing IR (ANF) — only the codegen layer is new. Pipeline: `source → parse → typecheck → IR → Go codegen → go build → binary`. Flag: `--compile-go`.

Ordered by dependency — each step builds on the previous:

1. [x] **Scaffold + hello world** — `internal/codegen/golang.go` with `EmitGo(prog, typeEnv)`; `--compile-go` flag in `cmd/rex/main.go`; `main _ = 0` emits Go `main()` + `os.Exit()`; write `.go` file, run `go build`
2. [x] **Primitives + arithmetic** — Int (`int64`), Float (`float64`), Bool, String, Unit; arithmetic, comparison, logical operators; `println`/`print` builtins; `if/then/else`; let bindings → Go local variables
3. [x] **Functions + closures** — top-level functions → Go functions; closures → Go closures; currying via partial application helpers; functions as values (`any` interface for polymorphism)
4. [x] **ADTs + pattern matching** — ADTs → Go interfaces + structs (tag + fields); pattern matching → type switches / if-else chains; constructor functions
5. [x] **Strings, lists, tuples** — strings → `string`; lists → cons cells (Go structs) or slices; tuples → generated struct types by arity; pattern matching on all three
6. [x] **Records** — record types → Go structs; field access, record update (clone + modify); record patterns
7. [x] **Tail call optimization** — trampoline loop for self-recursive tail calls
8. [x] **Traits** — static dispatch when type is known; runtime dispatch via type switch on `any`; Show/Eq/Ord from Prelude
9. [x] **Stdlib** — pure Rex stdlib compiles through same pipeline; IO/Net/Env → Go stdlib calls; module resolution reuses `ir.ResolveImports`
10. [x] **Actors** — `spawn` → `go func()`; mailboxes → Go channels; `send`/`receive`/`self`/`call` → channel operations

Key design decisions:
- **Polymorphism**: `any` (Go interface) for type variables; type assertions at use sites
- **Closures**: Go closures capture variables naturally; currying needs wrapper functions
- **Actors**: goroutines + channels are a direct match for Rex's actor model
- **Advantage over WasmGC**: no manual memory layout, no boxing gymnastics, actors work natively via goroutines

### Go backend as primary runtime (DONE)

The tree-walking interpreter has been removed. Go compilation is the only execution path:
- `rex file.rex` — compiles to Go, builds, and runs
- `rex build file.rex` — produces a standalone binary
- `rex --test file.rex` — compiles with test runner, builds, and runs
- `internal/eval/` deleted entirely
- Stdlib companion `.go` files use generated Rex types directly (`Rex_Request`, `RexList`, etc.)

### Compilation (JS backend — browser)

Compile Rex to JavaScript for browser deployment. Pipeline: `source → parse → typecheck → IR → JS codegen → .js + .html`. Flag: `--compile --target=browser`.

Ordered by dependency — each step builds on the previous:

1. [x] **Scaffold + hello world** — `internal/codegen/javascript.go` with `EmitJS(prog, typeEnv)`; `--compile --target=browser` in `cmd/rex/main.go`; `main _ = 0` emits `rex_main(null)`. Write `.js` + `.html` files.
2. [x] **Primitives + arithmetic** — numbers (JS `number` for both Int and Float), Bool, String, Unit (`null`); arithmetic, comparison, logical operators; `println`/`print`; `if/then/else`; let bindings → `const` declarations
3. [x] **Functions + closures** — top-level functions → JS functions; closures work naturally; currying via nested arrow functions; all values are JS dynamic types (no boxing needed)
4. [x] **ADTs + pattern matching** — constructors → objects `{$tag: "Red", $type: "Color"}` with field access `._0`, `._1`; pattern matching → if/else chains checking `.$tag`
5. [x] **Strings, lists, tuples** — strings → JS strings; lists → `{$tag: "Cons", head, tail}` / `null` for nil; tuples → arrays `[a, b]`; pattern matching on all three
6. [x] **Records** — plain JS objects `{x: 10, y: 32}`; field access → `.field`; record update → spread `{...rec, field: val}`
7. [x] **Tail call optimization** — not needed for basic recursion (stack is deep enough); trampoline can be added later
8. [x] **Traits** — dispatch functions with `typeof` + `.$tag`/`.$type` checks
9. [x] **Stdlib** — pure Rex stdlib through same pipeline; IO builtins → `console.log`; module resolution reuses `ir.ResolveImports`
10. [x] **Actors** — synchronous CPS-transformed `receive()`: `let msg = receive() in body` → `rex_receive_cps((msg) => { body })`. `spawn(f)` runs `f` which sets a `_resume` callback and returns. `send(pid, msg)` calls `pid._resume(msg)` synchronously. `call(pid, msgFn)` creates a reply pid and reads the reply from its buffer. No async, no `worker_threads` — pure synchronous direct function calls.

Key design decisions:
- **No boxing needed**: JS is dynamically typed — everything is already `any`
- **Closures**: arrow functions capture variables naturally; currying is trivial
- **No compilation step**: emit `.js` and run directly in browser
- **Actors**: synchronous CPS — `receive()` is CPS-transformed so `send` directly invokes the handler; no event loop or threads needed
- **Target overlays**: `--target=browser` enables `.browser.rex` module overlays (e.g., `Js.browser.rex` loaded for `import Std:Js`)
- **Js FFI**: `Std:Js` provides generic JS interop — `JsRef` opaque type; `jsGlobal`, `jsGet`, `jsSet`, `jsCall`, `jsNew`, `jsCallback`; `jsFrom*`/`jsTo*` conversions; `jsNull`. JS codegen intercepts calls by name (including `Std_Js__` prefix) and emits inline JS. Rex stubs use `error "browser-only builtin"` as placeholders.

### Browser Framework (TEA + Virtual DOM) — moved to tea-rex

Html (VDOM types, elements, attributes, events, renderToString, diffing/patching) and the TEA runtime (`browserMount`) now live in the `tea-rex` web framework package. The language provides `Std:Js`, `Std:Net`, and `Std:Process` as building blocks.

### Before going public

- `go install` support for the `rex` CLI
- Polish README (installation instructions, more examples)

## Key decisions already made

- **`()` unit**: zero-element tuple; `TUnit = TCon("Unit", [])` already existed; added `ast.Unit`, `ast.PUnit`, `VUnit`, `parse_atom`/`parse_atom_pattern` handling
- **Error handling**: IO functions return `Result ok String` instead of raising; `getEnv` returns `Maybe String`; use `Std:Result` or `Std:Maybe` to handle failures
- **Type system**: full Hindley-Milner inference; optional Elm-style annotations (`name : TypeSig` on separate line before `let`)
- **Compilation target**: WasmGC — emit WAT, assemble with `wasm-tools`. Runs in browsers natively and on servers via a Wasm runtime (Wasmtime/Wasmer/WasmEdge). ADTs map to WasmGC `struct` subtypes; TCO via `return_call`. Pipeline: `--compile` flag → parse → typecheck → IR lowering (ANF) → WAT emission (`internal/codegen`) → `wasm-tools parse` → `.wasm`. Currently supports `main _ = <expr>` with Int (i64), Float (f64), Bool (i32), arithmetic, comparisons, logical ops, if/else, let bindings.
- **Concurrency**: actors are a stdlib library / set of builtins, not a language feature. `Std:Process` ships five primitives (`spawn`, `send`, `receive`, `self`, `call`). Require `import Std:Process` — not injected globally. Go backend: goroutines + channels. JS backend: synchronous CPS. API stable across backends.
- **No hot reloading** for now
- **Exhaustiveness checking**: planned static pass (post-HM); `__ctor_families__` registry in type env tracks constructor siblings
- **No guards in pattern matching** (not planned)
- **Import system**: Three forms: `import Std:List (map, filter)` — selective stdlib import; `import Std:List as L` — qualified stdlib import; `import Utils (foo)` or `import Lib.Helpers as H` — user module import. `Std:` namespace resolves to embedded stdlib files. Bare uppercase ident = user module from `src/`; colon-prefixed = namespaced (Std, packages). Dots in user module paths map to directories: `Lib.Helpers` → `src/Lib/Helpers.rex`. Package imports use the package name as namespace: `import pkg:Module`. `SetSrcRoot()` in typechecker sets the `src/` root; `cmd/rex/main.go` detects it from cwd. Circular imports are detected with an import stack and produce clear error messages.
- **Export system**: Explicit exports via `export name` on its own line before the binding, or `export type`/`export trait` inline on declarations. Standalone `export name, ...` also works for re-exporting builtins (e.g., `export readFile, writeFile` in IO.rex). Convention: place `export name` on a separate line before the definition to keep function names aligned. `Exported bool` field on `Let`, `LetRec`, `LetPat`, `TypeDecl`, `TraitDecl` AST nodes. Both `CheckModule()` and `loadModule()` collect export names from `Exported` flags and standalone `Export` nodes.
- **`length` name collision**: resolved via qualified imports — `import Std:List as L` and `import Std:String as S` then use `L.length` vs `S.length`.
- **Traits v1**: `trait`/`impl` with Rust-style naming. Single-parameter traits only. Runtime dispatch (no type-level constraints). Prelude auto-loaded with `Ordering`, `Eq`, `Ord`, `Show` and instances for `Int`, `Float`, `String`, `Bool`. Comparison operators (`<`, `>`, `<=`, `>=`) extended to String (lexicographic) and Bool (`false < true`). `where` is a keyword.
- **String interpolation**: `"hello ${expr}"` syntax. Lexer scans `${...}` with mutual recursion (`skipInterp`/`skipString`) to handle nested strings; emits `TokInterp` containing `[]InterpPart`. Parser produces `ast.StringInterp{Parts []Expr}`. Typechecker allows any type per part, returns `TString`. `Show` trait dispatch converts non-string parts. `\$` produces literal `$`. Strings without `${` produce normal `TokString` (backward compatible).
- **Multi-line strings**: `"""..."""` triple-quoted strings. Handled entirely in the lexer — no new token types, parser/typechecker/eval unchanged. Opening `"""` detected by checking the two chars after the initial `"`. First newline after opening `"""` is stripped. Closing `"""` is three consecutive unescaped `"`. Lone `"` and `""` inside the string body are allowed. Escapes and `${expr}` interpolation work identically to regular strings. `skipString` also handles triple-quoted strings inside interpolation expressions. Produces the same `TokString`/`TokInterp` tokens. Line numbers tracked through the string body for correct error reporting.
- **Test framework**: Zig-inspired `test`/`assert` keywords. `\r` is a supported string escape.
- **Structural equality**: `==` and `!=` work on any Rex value including lists, tuples, ADTs, and records (recursive structural comparison). This means `Just 42 == Just 42` works.
- **Mutual recursion in types**: `_preregister_types` pre-pass in `check_program`, `check_module`, `_load_prelude_tc` registers all TypeDecl names before resolving constructors, enabling mutually recursive ADTs.
- **Std:Json**: `parse : String -> Result Json String` is Go-backed (`Std_Json_jsonParse` in companion file). `stringify` is pure Rex. Single ADT: `type Json = JNull | JBool Bool | JStr String | JNum Float | JArr [Json] | JObj [(String, Json)]` — arrays and objects use standard Rex lists and tuples (no separate `JsonList`/`JsonObj` types).
- **Stdlib test runner**: `--test` flag activates test runner. `test "name" = body` declares inline test blocks; `assert expr` checks a Bool at runtime. Normal execution skips tests. Tests are type-checked in all modes but only evaluated in test mode. `--only=<pattern>` filters tests by name. Duplicate test names are rejected at compile time.
- **Let-block**: `let` on its own line, followed by indented bindings on subsequent lines, terminated by `in`. Detected when the token after `let` is on a different line and at a greater column. Parser uses `caseArmCol` to bound each binding's body. Desugars to nested `Let` AST nodes (parser-only transform). `and` keyword is only for `let rec ... and ...` mutual recursion — removed for non-recursive `let`.
- **`main` entry point**: `export main args = ...` with type `List String -> Int`. The generated Go code calls `main` with program args as a Rex list. The return `Int` is used as the process exit code. `--test` mode does not require `main`. Type validation uses `Instantiate` + `Unify` so `main _ = 0` (type `a -> Int`) unifies with `List String -> Int`.
- **Bare top-level bindings**: `name params = body` at top level — no `let` required. Parser detects `lowercase_ident [lowercase_ident]* =` in `ParseTokens` via `isToplevelBinding()` and calls `parseToplevelBinding()`, producing `Let{Recursive: true, InExpr: nil}`. All top-level `let` bindings (parsed via `parseExpr → parseLet`) also auto-set `Recursive: true` when `InExpr == nil`. Mutual recursion between separate top-level bindings is detected automatically by `ReorderToplevel()` — Kahn's algorithm identifies cycles and groups them into a single `LetRec` node. `let`/`let rec` remain for expression-level bindings (with `in`). `export name params = body` works via `parseExport()` checking `isToplevelBinding()` before falling through to the ident-list case.
- **Std:Process**: Five builtins (`spawn`, `send`, `receive`, `self`, `call`). Require `import Std:Process` — not injected globally. Go backend: `spawn` → goroutine, mailboxes → channels, `call` → synchronous request-reply. `call` is `Pid b -> (Pid a -> b) -> a` — the message construction function receives the caller's pid. **Important**: recursive loops inside `spawn` must use `in` syntax (`let rec loop n = ... in loop 0`) so the loop body doesn't greedily consume the initial call.
- **Maybe in Std:Maybe**: `type Maybe a = Nothing | Just a` moved from Prelude to `Std:Maybe` module. Require `import Std:Maybe (Just, Nothing)` — type name `Maybe` is available in annotations via `TypeDefs` propagation. Prelude retains only `Ordering`, `Eq`, `Ord`, `Show`.
- **Explicit imports**: Only core builtins (`not`, `error`, `todo`, `showInt`, `showFloat`) are available globally. All other builtins (IO, Math, String, Env, Process) require module imports. `Std:Convert` provides cross-conversion between Maybe and Result (`toResult`, `toMaybe`, `fromMaybe`).
- **Std:Net**: TCP networking module with 7 Go builtins: `tcpListen` (returns `(Listener, Int)` for port-0), `tcpAccept`, `tcpConnect`, `tcpRead` (4096-byte buffer, EOF → `Err "EOF"`), `tcpWrite`, `tcpClose`, `tcpCloseListener`. Opaque `Listener` and `Conn` types (no type params). All IO operations return `Result`. Require `import Std:Net`.
- **`match`/`when` syntax**: Pattern matching uses `match expr` + `when pat -> body` arms (not `case`/`of`). `match` and `when` are keywords; `case` and `of` are not reserved and can be used as identifiers. Parser: `parseMatch()` consumes `match`, parses scrutinee, then loops over `when` arms at the same column (`firstWhenCol`). `caseArmCol = firstWhenCol` bounds arm bodies. Nested matches work because inner `when` arms are indented further right than outer `firstWhenCol`. `MatchArm` has `Line`/`Col` fields for error reporting.
- **Traits v2 Phase 1 — parameterized instances**: `impl Show (List a)`, `impl Eq (Maybe a)`, etc. `ImplDecl.TargetType` is now `TySyntax` (was `string`). Parser's `parseImplTarget()` handles parenthesized type expressions (`(List a)`, `(a, b)`). `RuntimeTypeName(v, env)` resolves compound types — Lists → `"List"`, tuples → `"Tuple2"`, VCtor → lookup in `__ctor_types__` map (constructor→type-name registry built from TypeDecl eval). `__ctor_types__` merged on import alongside `__instances__`. Typechecker freshens type variables in the resolved target type before substitution (prevents `{a → List a}` infinite recursion in `ApplySubst`). Trait dispatch propagates caller's `__instances__` and `__ctor_types__` into impl closures so cross-module types work (e.g., `show [Just 1]` where List impl is in Prelude and Maybe is imported). Impl closures are back-patched with new instances for self-referential dispatch (e.g., `eq [1,2] [1,2]` recursively calls `eq` on elements and sublists). Prelude adds: `Show/Eq/Ord (List a)`, `Show/Eq/Ord (a, b)`, `Show ()`. Maybe.rex adds: `Show/Eq/Ord (Maybe a)`. Result.rex adds: `Show/Eq (Result a b)`.
- **Traits v2 Phase 2 — compile-time constraints**: `types.Constraint{Trait, Var}` tracks trait requirements on type variables. `Scheme.Constraints` carries constraints on quantified vars. Constraints are seeded when `TraitDecl` methods get schemes with constraints on the trait param. `instantiate` remaps constraints to fresh vars and appends to `tc.constraints`. `resolveConstraints(s, env, startIdx)` applies the final substitution — TVars stay as constraints; TCons are checked against `__trait_instances__` (error if no instance). Called at all 6 `Generalize` sites. `checkAnnotation` maps inferred constraint vars to annotation vars via fresh instantiation + unification. Parser: `tryParseConstraintPrefix()` handles `Ord a => ...` and `(Eq a, Show a) => ...` syntax via backtracking. AST: `TyConstrained{Constraints, Inner}`. Lexer: `TokFatArrow` (`=>`). `SchemeToString` renders `Ord a => [a] -> [a]`. Key design: `==`/`!=`/`<`/`>` use built-in comparison (not trait dispatch), so they do NOT generate constraints. Only explicit trait method calls (e.g., `compare`, `show`) generate constraints. Inner constraints (e.g., `Eq (List a)` doesn't check that `a` has `Eq`) are NOT tracked — follow-up work.
- **`todo` builtin**: `todo : String -> a` — development placeholder that throws "TODO: message" at runtime. Typechecker emits a warning on every `todo` usage. `--safe` flag promotes warnings to errors (intended for CI/deploy). Warnings print in yellow, errors in red (TTY-aware). `Var` AST node has `Line int` for warning source locations.
- **Std:Bitwise**: Six Go builtins: `bitAnd`, `bitOr`, `bitXor` (`Int -> Int -> Int`), `bitNot` (`Int -> Int`), `shiftLeft`, `shiftRight` (`Int -> Int -> Int`). Named functions instead of operators — avoids `|` conflict with ADT syntax and keeps rarely-used operations out of operator space.
- **Std:Random**: Pure seed-based RNG with actor facade. One Go builtin (`systemSeed` — uses `math/rand/v2` for crypto-seeded entropy). Algorithm: xorshift32 (three XOR-shifts, masked to 32 bits), period ~2^32. Opaque `Rng` type hides internal state. Pure API: `rngMake` (seed → Rng), `rngInt` (range), `rngFloat` ([0,1)), `rngBool`, `rngList` (generate n values) — each returns `(value, newRng)` for deterministic threading. Actor facade: module-level actor holds Rng state; `randomInt`/`randomFloat`/`randomBool`/`shuffle` use `call` for convenient stateful API. `shuffle` uses Fisher-Yates selection. Imports `Std:Math (toFloat)` for float conversion and `Std:Process` for actor primitives. For concurrent programs, use the pure seed-based API (give each goroutine its own `Rng`).
- **Opaque types**: `export opaque type Email = Email String` — exports the type name (for annotations via `TypeDefs`) but hides constructors from importers. Consumers can't construct, pattern match, or access fields of opaque types directly — they must use exported smart constructors and accessor functions. `opaque` is a keyword; only valid as `export opaque type`. Works with both ADTs and records. In `CheckModule`/`loadModule`, opaque constructors are excluded from the exports set, and opaque record fields are excluded from propagated `__record_fields__`. `__ctor_families__` entries for opaque constructors are also filtered out to prevent pattern matching. `TypeDefs` still propagates the type name so annotations like `Email -> String` work. Trait instances on opaque types propagate normally (e.g., `impl Show Email` works for callers). Tests in `internal/typechecker/opaque_test.go` and `examples/user_modules/src/Email.rex`.
- **Std:DateTime**: Inspired by JS Temporal API. Two Go builtins: `dateTimeNow` (returns Unix millis as Int) and `dateTimeUtcOffset` (returns local UTC offset in minutes). Everything else is pure Rex — calendar math uses Howard Hinnant's civil time algorithm (public domain, with yoe clamp fix for era boundaries). Opaque types: `Instant` (millis since epoch) and `Duration` (millis). `DateTimeParts` record: `{ year, month, day, hour, minute, second, millisecond : Int }`. `Weekday` ADT: `Monday | Tuesday | ... | Sunday`. Public API: `now`, `fromMillis`, `fromParts`, `fromLocalParts`, `parse` (format string), `toMillis`, `toParts`, `toLocalParts`, `format`, `formatLocal`, `weekday`, duration constructors (`milliseconds`/`seconds`/`minutes`/`hours`/`days`), `toMilliseconds`/`toSeconds`, `add`/`sub`/`diff`. Format tokens: `YYYY`, `MM`, `DD`, `HH`, `mm`, `ss`, `SSS`. Trait instances: `Show`/`Eq`/`Ord` for `Instant`, `Duration`, `Weekday`. Pipe-friendly: last argument is always "self". Designed to map naturally to JS Temporal for future Wasm browser backend (host imports instead of pure Rex).
