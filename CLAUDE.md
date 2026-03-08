# CLAUDE.md — RexLang

> **Keep this file current.** Update CLAUDE.md whenever architecture changes, new conventions are established, key decisions are made, or planned work is completed or added. It is the primary source of truth for working in this repo.
>
> **Also update README.md** whenever a new language feature is added: add it to the Language or Standard library section, update the examples table if a new example file was created, and check items off (or remove them from) the Roadmap. The README is the public-facing feature list.

## Project overview

RexLang is a functional language with algebraic data types and pattern matching. The implementation is a Go tree-walking interpreter that ships as a single static binary — no runtime dependency. The long-term plan is a **WasmGC compilation backend** — producing `.wasm` binaries that run in browsers (native) and on servers via a Wasm runtime (Wasmtime, Wasmer, WasmEdge).

## Language goals

- **No runtime errors** — the type system should catch mistakes at compile time; the ideal is an Elm-style guarantee where a well-typed program cannot crash at runtime
- **Readability** — code should be easy to read and understand without knowing the language deeply; a good target for AI code generation
- **Mainstream over ML** — when a design choice comes up, prefer the convention from mainstream languages (TypeScript, Rust, Python, Go) over ML-family tradition (Haskell, OCaml, SML); RexLang is not trying to be another ML

## Repository layout

```
examples/          .rex example programs (one per feature)
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
    embed.go       //go:embed all:rexfiles; Source(name) string (dots → subdirs)
    rexfiles/      .rex stdlib files (Prelude, List, Map, String, Math, IO, Env, Result, Json, Json/Decode, Process, Parallel, Stream, Convert, Net)
```

## Development commands

All commands run from the repo root:

```bash
# build
go build -o rex ./cmd/rex/

# run a program (requires export main)
./rex examples/io.rex
./rex examples/actors.rex arg1 arg2

# run tests in a .rex file (no main required)
./rex --test examples/testing.rex
./rex --test internal/stdlib/rexfiles/List.rex

# --safe flag: promote warnings (todo usage)to errors
./rex --safe examples/io.rex
./rex --safe --test examples/testing.rex

# REPL (blank line to eval, Ctrl-D to exit)
./rex

# run all Go tests
go test ./...

# format
gofmt -w .
```

## Architecture notes

- **Pipeline**: source → `lexer.Tokenize()` → `parser.Parse()` → `ValidateToplevel()` → `typechecker.CheckProgram()` → validate `main : List String -> Int` → `eval.RunProgram()` (which calls `main` with program args)
- **Top-level restriction**: only declarations allowed at top level (bare bindings `name params = body`, `let`, `type`, `trait`, `impl`, `import`, `export`, `test`, type annotations). Bare expressions are rejected. Applies in both file mode and `--test` mode; REPL is exempt.
- **Top-level bindings**: bare `name params = body` at top level (no `let` needed). Parser detects lowercase ident followed by `[ident]* =` and produces `Let{Recursive: true}`. All top-level `let` bindings also auto-set `Recursive: true`. Mutual recursion between top-level bindings is detected automatically by `ReorderToplevel` (cycle detection groups them into `LetRec`). `let`/`let rec` remain for expression-level bindings only.
- **`main` entry point**: programs run with `./rex file.rex` must define `export main args = ...` where `main : List String -> Int`. `args` receives command-line arguments as a list of strings. The return value is the process exit code. `--test` mode does not require `main`.
- **Language**: Go 1.24+. Single binary, no runtime dependency.
- **Type inference**: `internal/typechecker` implements Algorithm W (Hindley-Milner); runs after parse, before eval; type errors are fatal. Types in `internal/types` (`TVar`, `TCon`, `Scheme`). Arithmetic operators (`+` `-` `*` `/`) require `Int` or `Float`; free type variables in arithmetic expressions default to `Int`. Use `toFloat` to convert before Float arithmetic. REPL shows `name : type` after each binding.
- **Values**: `VInt`, `VFloat`, `VString`, `VBool`, `VClosure`, `VCtor`, `VCtorFn`, `VRecordCtorFn`, `VBuiltin`, `VTraitMethod`, `VInstances`, `VModule`, `VPid`, `VRecord` — all implement `Value` interface via `valueKind()`.
- **Actors**: `VPid{Mailbox *Mailbox, ID int64}` is the process handle. `Mailbox` is an unbounded FIFO queue (mutex + cond + slice; Erlang-style — `Send` never blocks or fails). Five builtins: `spawn : (() -> b) -> Pid a`, `send : Pid a -> a -> ()`, `receive : () -> a`, `self : Pid a`, `call : Pid b -> (Pid a -> b) -> a`. Require `import Std:Process (spawn, send, receive, self, call)`. `ProcessBuiltins(selfPid VPid)` returns them keyed to a specific mailbox.
- **Environment**: `Env = map[string]Value`; `Clone()` and `Extend()` for closure snapshots.
- **Tail calls**: the evaluator uses a trampoline `for {}` loop for tail-recursive functions.
- **Type aliases**: `type alias Name = String` — transparent alias, fully interchangeable at the type level. Parametric: `type alias Pair a b = (a, b)`. Stored in `tc.typeAliases` (`TypeAliasInfo{Params, Body}`); non-parametric aliases also stored in `typeDefs` for direct lookup. The `alias` keyword after `type` unambiguously distinguishes aliases from ADTs (no heuristic needed). No runtime effect.
- **ADTs**: `type Foo = A | B int` registers constructors; `type Foo a = …` for parametric ADTs.
- **Records**: `type Person = { name : String, age : Int }` — nominal record types tied to `type` declarations. Construction: `Person { name = "Alice", age = 30 }` or positional: `Person "Alice" 30`. The type name is a positional constructor function (`VRecordCtorFn`) that supports currying and can be passed as a higher-order function (e.g., `map2 Person ...`). Field access: `p.name` (chained: `p.addr.city`; lowercase `.` produces `FieldAccess`; uppercase `.` produces `DotAccess` for modules). Update: `{ alice | name = "Bob" }` — creates a new record with changed fields. Nested dot-path updates: `{ model | user.name = "Alice" }` — recursively clones and updates nested records. Pattern matching: `Person { name = n, age = a }` (partial patterns OK). Parametric records: `type Pair a b = { fst : a, snd : b }`. Typechecker infers record type from field name when the expression type is a TVar. Field metadata stored in `__record_fields__` registry (keyed by type name → `RecordInfo`). Module imports propagate `__record_fields__` and `TypeDefs` via `ModuleResult`, so record types defined in imported modules can be constructed, accessed, and updated by the importer.
- **Let-block**: `let` on its own line followed by indented bindings, terminated by `in`. Parser-only — desugars to nested `Let` AST nodes. Detected when the token after `let` is on a different line and indented. `and` is only for `let rec ... and ...` mutual recursion.
- **Pipe** `|>`: left-associative, desugars to function application at eval time.
- **Trailing lambda**: when `\` appears after a function application (not inside `isAtomStart`), `parseApp()` treats it as the last argument. Sets `caseArmCol` to the function head's column so the lambda body terminates when indentation drops. Enables `Decoder \json -> ...` and `spawn \_ -> ...` without wrapping parens. One-line lambdas still use parens.
- **Traits**: `trait`/`impl` (Rust-style naming) for ad-hoc polymorphism. Single-parameter traits, runtime dispatch. `Prelude.rex` auto-loaded with `Eq`, `Ord`, `Show`. Trait instances stored in `VInstances` keyed by `"TraitName:TypeName:MethodName"`.
- **String interpolation**: `"hello ${expr}"` — lexer emits `TokInterp` with `[]InterpPart`; parser produces `ast.StringInterp{Parts}`; eval dispatches `Show` trait for conversion. `\$` escapes literal `$`. Nested interpolation (`"${f "inner ${x}"}"`) supported via mutual recursion in lexer (`skipInterp`/`skipString`).
- **Multi-line strings**: `"""..."""` triple-quoted strings. Lexer-only feature — produces same `TokString`/`TokInterp` tokens. First newline after opening `"""` stripped. Lone `"` or `""` inside allowed; only `"""` closes.
- **Type annotations**: `name : TypeSig` on a separate line before the binding. Parser detects lowercase ident + `:` in `ParseTokens`. Typechecker stores annotation as `__ann:name` in env; `toplevelLet` checks inferred type against annotation via `checkAnnotation()` (instantiate both, unify). If annotation exists and matches, it replaces the generalized type in env (constraining polymorphism). Eval ignores annotations (`VUnit`).
- **Test framework**: `test "name" = body` / `assert expr`. `--test` flag runs them.
- **Stdlib embedding**: `.rex` files embedded via `//go:embed` in `internal/stdlib/embed.go`.

## Conventions

- Every new language feature needs: lexer token (if needed) + AST node + parser rule + eval case + tests + example file
- Example files in `examples/` are test-only (run with `--test`); programs that produce output have `export main`
- No bare expressions at top level in `.rex` files — only declarations
- `gofmt -w .` before committing
- Comments use `--` in `.rex` files
- **Never commit or push unless explicitly asked** — each request is one-off, not a standing instruction
- **Prefer the pipe operator `|>`** when writing Rex code — use it to make data flow read left-to-right instead of deeply nesting function calls. E.g., `list |> map f |> filter g` over `filter g (map f list)`.

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

Trailing lambda: when a multi-line lambda is the last (or sole) argument to a function, write it without parens — the parser bounds the body by indentation (same `caseArmCol` mechanism as `case`/`test`). One-line lambdas keep parens.

```rex
-- good: trailing lambda (multi-line, last argument)
Decoder \json ->
    case json of
        JStr s ->
            Ok s
        _ ->
            Err (DecodeError { path = [], message = "expected a String", value = json })

-- good: one-line lambda keeps parens
map (\x -> x + 1) list

-- good: non-last argument keeps parens
map (\x -> f x) list
```

One blank line between top-level definitions; two blank lines between sections. Stdlib modules use `-- # Section` headers and `-- | doc` comments above each function. Every stdlib function should have its tests immediately after its definition — not grouped at the bottom of the file.

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
- Opaque types — export a type without its constructor; consumers interact only through provided functions. Prerequisite: user modules. Syntax TBD.
- Package system — third-party dependencies

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
- Date/Time (even basic)
- Random numbers

### Language ergonomics

- [x] Traits v1 — `trait`/`impl`, runtime dispatch, `Eq`/`Ord` in Prelude
- [x] Test framework — `test "name" = …` / `assert expr`, `--test` flag
- [x] Type annotations — optional `add : Int -> Int -> Int` before `let` binding
- [x] Let-blocks — `let` + indented bindings + `in` (parser-only desugaring)
- Traits v2 — parameterized instances (e.g., `impl Ord (List a)`), constraint tracking in types (`Ord a => ...`)
- [x] Exhaustiveness checking — static pass post-HM using `__ctor_families__`; rejects non-exhaustive `case` (ADTs, bools, lists require all constructors; literals/tuples require catch-all `_ ->`); refutable `let` patterns rejected via `isIrrefutable` check
- Typed holes — `?name` in expression position; typechecker infers the required type from surrounding context and reports it along with in-scope bindings; enables type-directed, incremental program construction. Never reaches eval. Implementation: `HoleExpr{Name string}` AST node; typechecker unifies hole with inferred type, collects into a holes report instead of a hard error. Use `?name` (not `_`) to avoid ambiguity with pattern wildcards.

### Error experience

- Better error messages — source locations, span info
- Stack traces on runtime errors (maybe)

### Compilation

- IR design (A-normal form; ADTs map to WasmGC `struct` subtypes)
- WasmGC backend: emit WAT (WebAssembly Text) → `wasm-tools` assemble → `.wasm`

### Before going public

- `go install` support for the `rex` CLI
- Polish README (installation instructions, more examples)
- REPL history (`readline` + `~/.rexlang_history`)

## Key decisions already made

- **`()` unit**: zero-element tuple; `TUnit = TCon("Unit", [])` already existed; added `ast.Unit`, `ast.PUnit`, `VUnit`, `parse_atom`/`parse_atom_pattern` handling
- **Error handling**: IO functions return `Result ok String` instead of raising; `getEnv` returns `Maybe String`; use `Std:Result` or `Std:Maybe` to handle failures
- **Type system**: full Hindley-Milner inference; optional Elm-style annotations (`name : TypeSig` on separate line before `let`)
- **Compilation target**: WasmGC — emit WAT, assemble with `wasm-tools`. Runs in browsers natively and on servers via a Wasm runtime (Wasmtime/Wasmer/WasmEdge). ADTs map to WasmGC `struct` subtypes; TCO via `return_call`.
- **Concurrency**: actors are a stdlib library / set of builtins, not a language feature. `Std:Process` ships five primitives (`spawn`, `send`, `receive`, `self`, `call`) as Go builtins. Require `import Std:Process` — not injected globally. `spawn` runs a Rex closure in a new goroutine with its own mailbox; `call` implements synchronous request-reply. API stable; internals could swap for WASI threads later.
- **No hot reloading** for now
- **Exhaustiveness checking**: planned static pass (post-HM); `__ctor_families__` registry in type env tracks constructor siblings
- **No guards in pattern matching** (not planned)
- **Import system**: Three forms: `import Std:List (map, filter)` — selective stdlib import; `import Std:List as L` — qualified stdlib import; `import Utils (foo)` or `import Lib.Helpers as H` — user module import. `Std:` namespace resolves to embedded stdlib files. Bare uppercase ident = user module from `src/`; colon-prefixed = namespaced (Std, future packages). Dots in user module paths map to directories: `Lib.Helpers` → `src/Lib/Helpers.rex`. `SetSrcRoot()` in both typechecker and eval sets the `src/` root; `cmd/rex/main.go` detects it from cwd. Circular imports are detected with an import stack and produce clear error messages.
- **Export system**: Explicit exports via `export name` on its own line before the binding, or `export type`/`export trait` inline on declarations. Standalone `export name, ...` also works for re-exporting builtins (e.g., `export readFile, writeFile` in IO.rex). Convention: place `export name` on a separate line before the definition to keep function names aligned. `Exported bool` field on `Let`, `LetRec`, `LetPat`, `TypeDecl`, `TraitDecl` AST nodes. Both `CheckModule()` and `loadModule()` collect export names from `Exported` flags and standalone `Export` nodes.
- **`length` name collision**: resolved via qualified imports — `import Std:List as L` and `import Std:String as S` then use `L.length` vs `S.length`.
- **Traits v1**: `trait`/`impl` with Rust-style naming. Single-parameter traits only. Runtime dispatch (no type-level constraints). Prelude auto-loaded with `Ordering`, `Eq`, `Ord`, `Show` and instances for `Int`, `Float`, `String`, `Bool`. Comparison operators (`<`, `>`, `<=`, `>=`) extended to String (lexicographic) and Bool (`false < true`). `where` is a keyword.
- **String interpolation**: `"hello ${expr}"` syntax. Lexer scans `${...}` with mutual recursion (`skipInterp`/`skipString`) to handle nested strings; emits `TokInterp` containing `[]InterpPart`. Parser produces `ast.StringInterp{Parts []Expr}`. Typechecker allows any type per part, returns `TString`. Eval dispatches `Show:TypeName:show` from `__instances__` for each part (short-circuits VString). `\$` produces literal `$`. Strings without `${` produce normal `TokString` (backward compatible). `showInt`/`showFloat` builtins in CoreBuiltins + InitialTypeEnv for Prelude's Show instances.
- **Multi-line strings**: `"""..."""` triple-quoted strings. Handled entirely in the lexer — no new token types, parser/typechecker/eval unchanged. Opening `"""` detected by checking the two chars after the initial `"`. First newline after opening `"""` is stripped. Closing `"""` is three consecutive unescaped `"`. Lone `"` and `""` inside the string body are allowed. Escapes and `${expr}` interpolation work identically to regular strings. `skipString` also handles triple-quoted strings inside interpolation expressions. Produces the same `TokString`/`TokInterp` tokens. Line numbers tracked through the string body for correct error reporting.
- **Test framework**: Zig-inspired `test`/`assert` keywords. `\r` is a supported string escape.
- **Structural equality**: `==` and `!=` work on any Rex value including lists, tuples, ADTs, and records (recursive structural comparison). This means `Just 42 == Just 42` works.
- **Mutual recursion in types**: `_preregister_types` pre-pass in `check_program`, `check_module`, `_load_prelude_tc` registers all TypeDecl names before resolving constructors, enabling mutually recursive ADTs.
- **Std:Json**: `parse : String -> Result Json String` is Go-backed (`jsonParse` builtin in `builtins_core.go`). `stringify` is pure Rex. Single ADT: `type Json = JNull | JBool Bool | JStr String | JNum Float | JArr [Json] | JObj [(String, Json)]` — arrays and objects use standard Rex lists and tuples (no separate `JsonList`/`JsonObj` types). Json.rex imports `Std:List (intersperse, map, foldl)` for stringify helpers.
- **Stdlib test runner**: `RunTests` in `eval.go` runs test blocks. `cmd/rex/main.go --test` flag activates test runner. `test "name" = body` declares inline test blocks; `assert expr` checks a Bool at runtime. Normal execution skips tests. Tests are type-checked in all modes but only evaluated in test mode. Test body env is isolated (bindings don't leak).
- **Let-block**: `let` on its own line, followed by indented bindings on subsequent lines, terminated by `in`. Detected when the token after `let` is on a different line and at a greater column. Parser uses `caseArmCol` to bound each binding's body. Desugars to nested `Let` AST nodes (typechecker and eval untouched). `and` keyword is only for `let rec ... and ...` mutual recursion — removed for non-recursive `let`.
- **`main` entry point**: `export main args = ...` with type `List String -> Int`. `RunProgram` evaluates all top-level declarations, then looks up `main` and calls it with program args as `List String`. The return `Int` is used as the process exit code. `--test` mode and REPL do not require `main`. `ValidateToplevel()` in `eval.go` rejects bare expressions; called early in both `runFile` and `runTests` in `cmd/rex/main.go`. Type validation uses `Instantiate` + `Unify` so `main _ = 0` (type `a -> Int`) unifies with `List String -> Int`.
- **Bare top-level bindings**: `name params = body` at top level — no `let` required. Parser detects `lowercase_ident [lowercase_ident]* =` in `ParseTokens` via `isToplevelBinding()` and calls `parseToplevelBinding()`, producing `Let{Recursive: true, InExpr: nil}`. All top-level `let` bindings (parsed via `parseExpr → parseLet`) also auto-set `Recursive: true` when `InExpr == nil`. Mutual recursion between separate top-level bindings is detected automatically by `ReorderToplevel()` — Kahn's algorithm identifies cycles and groups them into a single `LetRec` node. `let`/`let rec` remain for expression-level bindings (with `in`). `export name params = body` works via `parseExport()` checking `isToplevelBinding()` before falling through to the ident-list case.
- **Std:Process**: Five builtins (`spawn`, `send`, `receive`, `self`, `call`) implemented entirely in Go (`ProcessBuiltins(selfPid VPid)`). Require `import Std:Process` — not injected globally. `call` is Go-only because it needs to close over the caller's `selfPid` — a Rex implementation would capture the module-load-time mailbox instead. `spawn` injects per-goroutine `self` and `receive` into the spawned closure's env. `call` is `Pid b -> (Pid a -> b) -> a` — the message construction function receives the caller's pid. **Important**: recursive loops inside `spawn` must use `in` syntax (`let rec loop n = ... in loop 0`) so the loop body doesn't greedily consume the initial call. Capture `self` before `spawn` if the goroutine needs to reply to the spawning process.
- **Maybe in Std:Maybe**: `type Maybe a = Nothing | Just a` moved from Prelude to `Std:Maybe` module. Require `import Std:Maybe (Just, Nothing)` — type name `Maybe` is available in annotations via `TypeDefs` propagation. Prelude retains only `Ordering`, `Eq`, `Ord`, `Show`.
- **Explicit imports**: Runtime env only contains `CoreBuiltins` (`not`, `error`, `todo`, `showInt`, `showFloat`). All other builtins (IO, Math, String, Env, Process) require module imports. `BuiltinsForModule()` gives each stdlib module only CoreBuiltins + its own specific builtins. `Std:Convert` provides cross-conversion between Maybe and Result (`toResult`, `toMaybe`, `fromMaybe`).
- **Std:Net**: TCP networking module with 7 Go builtins: `tcpListen` (returns `(Listener, Int)` for port-0), `tcpAccept`, `tcpConnect`, `tcpRead` (4096-byte buffer, EOF → `Err "EOF"`), `tcpWrite`, `tcpClose`, `tcpCloseListener`. Opaque `Listener` and `Conn` types (no type params). All IO operations return `Result`. `VListener{L net.Listener}` and `VConn{C net.Conn}` value types in `values.go`. Registered in `BuiltinsForModule` and `typeEnvForModule`/`typeDefsForModule` under `"Net"`. Require `import Std:Net`.
- **`todo` builtin**: `todo : String -> a` — development placeholder that throws "TODO: message" at runtime. Typechecker emits a warning on every `todo` usage. `--safe` flag promotes warnings to errors (intended for CI/deploy). Warnings print in yellow, errors in red (TTY-aware). `Var` AST node has `Line int` for warning source locations.
