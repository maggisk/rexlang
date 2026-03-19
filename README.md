# Rex

```rex
import Std:Http.Server (serve, html, json, text, segments, Request, Response)
import Std:IO (println)
import Std:Result (Ok, Err)

type User = { name : String, role : String }

handle : Request -> Response
handle req =
    match (req.method, segments req.path)
        when ("GET", []) ->
            html 200 "<h1>Welcome</h1>"
        when ("GET", ["users", id]) ->
            json 200 "{\"id\": \"${id}\"}"
        when ("POST", ["users"]) ->
            json 201 req.body
        when _ ->
            text 404 "not found"

export main _ =
    match serve 8080 handle
        when Ok _ -> 0
        when Err e -> let _ = println e in 1
```

A type-safe HTTP server in 20 lines. No null pointers, no unhandled exceptions, no runtime surprises. Every branch is checked at compile time.

## What is Rex?

Rex is a functional programming language that compiles to Go. It has algebraic data types, pattern matching, Hindley-Milner type inference, and Erlang-style actors. The compiler catches mistakes at compile time so your programs don't crash at runtime. Rex ships as a single binary and Go is the only dependency.

> Twenty years of language design opinions, vibe coded into existence. Elm's elegance. Erlang's actors. Go's speed. One binary. No runtime errors, and no human who fully understands this codebase -- only our new AI overlords.

## Install

```bash
go install github.com/maggisk/rexlang/cmd/rex@latest
```

That's it. Now you have the `rex` command.

## Quick start

```bash
mkdir myapp && cd myapp
rex init                          # creates rex.toml
mkdir src
```

Create `src/Main.rex`:

```rex
import Std:IO (println)

export main _ =
    let _ = println "Hello from Rex!"
    in 0
```

Run it:

```bash
rex src/Main.rex           # compiles to Go, builds, runs
rex build src/Main.rex     # produces a standalone binary
```

## Features

### Type inference

Rex infers all types. Annotations are optional documentation:

```rex
double x = x * 2                      -- inferred: Int -> Int

identity x = x                        -- inferred: a -> a

names = ["Alice", "Bob", "Charlie"]    -- inferred: [String]
```

### Algebraic data types

```rex
type Shape = Circle Float | Rect Float Float

area shape =
    match shape
        when Circle r ->
            3.14159 * r * r
        when Rect w h ->
            w * h
```

Forget a case? The compiler rejects it. Every `match` must be exhaustive.

### Pattern matching

Match on anything: ADTs, tuples, lists, records, literals. Nest them freely.

```rex
import Std:Maybe (Just, Nothing)

firstJust a b =
    match (a, b)
        when (Just x, _) ->
            Just x
        when (_, Just y) ->
            Just y
        when _ ->
            Nothing
```

### Pipe operator

```rex
import Std:List (map, filter)
import Std:Stream (from, take)

-- first 5 even squares
from 1
    |> map (\x -> x * x)
    |> filter (\x -> x % 2 == 0)
    |> take 5
-- [4, 16, 36, 64, 100]
```

### Actors

Erlang-style concurrency with typed messages. On the Go backend, actors are goroutines.

```rex
import Std:Process (spawn, send, receive, call)

type Msg = Inc | Get (Pid Int) | Stop

counter =
    spawn \me ->
        let rec loop n =
            match receive me
                when Inc ->
                    loop (n + 1)
                when Get replyTo ->
                    let _ = send replyTo n
                    in loop n
                when Stop ->
                    ()
        in loop 0

_ = send counter Inc
_ = send counter Inc
_ = send counter Inc
n = call counter Get        -- 3
```

### Traits

```rex
trait Describable a where
    describe : a -> String

impl Describable Int where
    describe n =
        if n > 0 then "positive"
        else if n == 0 then "zero"
        else "negative"
```

Constraint tracking is automatic. Use `sort` on a polymorphic list and the compiler infers `Ord a =>`:

```rex
import Std:List (sort)

-- inferred: Ord a => [a] -> [a]
mySort lst = sort lst
```

### Records

```rex
type Person = { name : String, age : Int }

alice = Person { name = "Alice", age = 30 }
alice.name                                    -- "Alice"
bob = { alice | name = "Bob", age = 25 }      -- immutable update
```

### JSON decoding

Elm-style composable decoders with path-tracked errors:

```rex
import Std:Json.Decode (decodeString, decode, with, field, string, int, bool)

type Player = { name : String, score : Int, active : Bool }

playerDecoder =
    decode Player
        |> with (field "name" string)
        |> with (field "score" int)
        |> with (field "active" bool)
```

### Built-in tests

```rex
double x = x * 2

test "double works" =
    assert (double 5 == 10)
    assert (double 0 == 0)
```

```bash
rex --test myfile.rex                  # run tests
rex --test --only="double" myfile.rex  # filter by name
rex --safe myfile.rex                  # reject any `todo` placeholders
```

### Browser compilation

```bash
rex --compile --target=browser src/Main.rex    # emits .js + .html
```

Actors compile to synchronous CPS on JavaScript. The `Std:Js` module provides browser FFI via an opaque `JsRef` type.

## Go interop

Rex compiles to Go, and the interop is designed to feel natural from both sides. Stdlib builtins are plain Go functions with an `external` declaration in Rex.

### The type mapping

| Rex type | Go type | Notes |
| --- | --- | --- |
| `Int` | `int64` | |
| `Float` | `float64` | |
| `String` | `string` | |
| `Bool` | `bool` | |
| `List a` | `*RexList` | Cons-cell linked list |
| `Result ok String` | `(T, error)` | Go's idiomatic error pattern |
| `Result Unit String` | `error` | When the success value is `()` |
| `Maybe a` | `*T` | `nil` = `Nothing`, non-nil = `Just` |
| ADTs | interface + structs | Each constructor is a struct implementing the interface |
| Records | Go struct | Field names are exported |

### Result maps to (T, error)

When a Go function returns `(T, error)`, Rex sees it as `Result T String`. The compiler generates the wrapper automatically:

**Go side** (`io.go`):

```go
func Std_IO_readFile(path string) (string, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return "", err
    }
    return string(data), nil
}
```

**Rex side** (`IO.rex`):

```rex
external readFile : String -> Result String String
```

**Usage:**

```rex
import Std:IO (readFile)
import Std:Result (Ok, Err)

match readFile "config.txt"
    when Ok contents ->
        parseConfig contents
    when Err message ->
        useDefaults ()
```

No exceptions, no panics. The Go `error` becomes a `Result` that the type system forces you to handle.

### Maybe maps to pointer/nil

A Go function returning a pointer maps to `Maybe`. `nil` becomes `Nothing`, non-nil becomes `Just`:

**Go side** (`env.go`):

```go
func Std_Env_getEnv(name string) *string {
    val, ok := os.LookupEnv(name)
    if !ok {
        return nil
    }
    return &val
}
```

**Rex side** (`Env.rex`):

```rex
external getEnv : String -> Maybe String
```

**Usage:**

```rex
import Std:Env (getEnv)
import Std:Maybe (Just, Nothing, withDefault)

port = getEnv "PORT" |> withDefault "8080"
```

### ADTs map to interfaces

Each Rex ADT becomes a Go interface with a tag method, and each constructor becomes a struct:

```rex
type Shape = Circle Float | Rect Float Float
```

Compiles to:

```go
type Rex_Shape interface{ tagRex_Shape() int }

type Rex_Shape_Circle struct{ F0 float64 }
func (Rex_Shape_Circle) tagRex_Shape() int { return 0 }

type Rex_Shape_Rect struct{ F0, F1 float64 }
func (Rex_Shape_Rect) tagRex_Shape() int { return 1 }
```

### Writing your own builtins

1. Declare the type in your `.rex` file:

```rex
external myBuiltin : String -> Int -> Result String String
```

2. Write the Go companion function with the naming convention `Pkg_Module_name`:

```go
func Pkg_Module_myBuiltin(s string, n int64) (string, error) {
    // Your Go code here
    return result, nil
}
```

The compiler generates a wrapper that handles type assertions and Result/Maybe conversion.

## Standard library

| Module | Highlights |
| --- | --- |
| `Std:List` | `map`, `filter`, `foldl`, `sort`, `zip`, `concat`, `find`, `partition`, 30+ functions |
| `Std:Map` | AVL tree sorted map: `insert`, `lookup`, `remove`, `fromList`, `toList` |
| `Std:Maybe` | `Just`/`Nothing`, `map`, `andThen`, `withDefault`, `orElse` |
| `Std:Result` | `Ok`/`Err`, `map`, `andThen`, `withDefault`, `try` (catch div-by-zero) |
| `Std:String` | `split`, `join`, `trim`, `replace`, `contains`, `parseInt`, 25+ functions |
| `Std:Json` | `parse`/`stringify`, `Json` ADT with encode helpers |
| `Std:Json.Decode` | Elm-style decoders: `field`, `map2`, `oneOf`, `andThen`, path-tracked errors |
| `Std:IO` | `readFile`, `writeFile`, `listDir` -- all return `Result` |
| `Std:Env` | `getEnv` (returns `Maybe`), `getEnvOr`, `args` |
| `Std:Process` | Actors: `spawn`, `send`, `receive`, `self`, `call` |
| `Std:Parallel` | `pmap`, `pmapN` -- parallel map using actors |
| `Std:Stream` | Lazy infinite sequences: `from`, `iterate`, `map`, `filter`, `take` |
| `Std:Net` | TCP: `tcpListen`, `tcpAccept`, `tcpConnect`, `tcpRead`, `tcpWrite` |
| `Std:Http.Server` | HTTP server with pattern-matching routing |
| `Std:Math` | `abs`, `pow`, `sqrt`, trig, `log`, `pi`, `e`, `clamp` |
| `Std:DateTime` | `Instant`/`Duration` opaque types, formatting, arithmetic |
| `Std:Random` | Pure seed-based RNG + actor facade for stateful usage |
| `Std:Bitwise` | `bitAnd`, `bitOr`, `bitXor`, `bitNot`, `shiftLeft`, `shiftRight` |
| `Std:Convert` | `toResult`, `toMaybe`, `fromMaybe` -- cross-convert Maybe/Result |
| `Std:Js` | Browser FFI: `JsRef`, `jsGlobal`, `jsGet`, `jsCall` (browser target only) |

## CLI reference

```bash
rex src/Main.rex                               # compile and run
rex build src/Main.rex                         # produce standalone binary
rex --test src/Foo.rex                         # run tests
rex --test --only="pattern" src/Foo.rex        # run matching tests
rex --safe src/Main.rex                        # reject todo placeholders
rex --types src/Foo.rex                        # show inferred types
rex --types Std:List                           # show stdlib module types
rex --compile --target=browser src/Main.rex    # compile to JS
rex init                                       # create rex.toml
rex install                                    # fetch dependencies
rex install https://github.com/user/pkg main   # add git dependency
rex install ../mylib                           # add local path dependency
```

## Language reference

Full syntax and semantics are in the sections below. Click to expand.

<details>
<summary><strong>Primitives</strong></summary>

```rex
42          -- Int
3.14        -- Float
"hello"     -- String
true        -- Bool
()          -- Unit

0xFF        -- hex Int (255)
0o77        -- octal Int (63)
0b1010      -- binary Int (10)
1_000_000   -- underscores for readability
```

</details>

<details>
<summary><strong>Functions and bindings</strong></summary>

```rex
x = 42
add x y = x + y                  -- curried automatically

-- mutual recursion works automatically
isEven n = if n == 0 then true else isOdd (n - 1)
isOdd n = if n == 0 then false else isEven (n - 1)
```

No `let` needed at the top level. Definition order doesn't matter. Inside expressions, use `let`/`let rec` with `in`:

```rex
let
    a = 3
    b = 4
    square x = x * x
in square a + square b
```

</details>

<details>
<summary><strong>Type annotations</strong></summary>

```rex
double : Int -> Int
double x = x * 2

identity : a -> a
identity x = x

sort : Ord a => [a] -> [a]
sort lst = ...
```

Annotations go on a separate line before the binding. If the annotation contradicts the inferred type, you get an error.

</details>

<details>
<summary><strong>Lists and tuples</strong></summary>

```rex
xs = [1, 2, 3]
pair = (42, "hello")

match xs
    when [] ->
        0
    when [h | t] ->
        h + sum t
```

</details>

<details>
<summary><strong>String interpolation</strong></summary>

```rex
name = "Rex"
version = 1
"Hello, ${name}! Version ${version}"    -- "Hello, Rex! Version 1"
"Expr: ${1 + 2 + 3}"                   -- "Expr: 6"
```

Expressions inside `${...}` are converted to strings via the `Show` trait.

</details>

<details>
<summary><strong>Multi-line strings</strong></summary>

```rex
import Std:String (dedent)

html = dedent """
    <div>
        <p>hello</p>
    </div>
    """
```

</details>

<details>
<summary><strong>Imports and modules</strong></summary>

```rex
-- Standard library
import Std:List (map, filter, foldl)
import Std:Map as M

-- User modules (from src/ directory)
import Utils (double, greet)
import Lib.Helpers as H

-- Package modules
import mylib:Parser (parse)
```

</details>

<details>
<summary><strong>Exports and opaque types</strong></summary>

```rex
export add
add x y = x + y

export type Shape = Circle Float | Rect Float Float

-- Opaque: type name exported, constructors hidden
export opaque type Email = Email String

export
make : String -> Email
make s = Email s
```

</details>

<details>
<summary><strong>Error handling</strong></summary>

IO functions return `Result` instead of raising. There are no exceptions.

```rex
import Std:IO (readFile)
import Std:Result (Ok, Err, withDefault)

contents = withDefault "" (readFile "data.txt")
```

`todo` serves as a development placeholder. It type-checks as any type but throws at runtime. The compiler warns, and `--safe` makes it an error:

```rex
handle x = todo "implement later"
```

</details>

<details>
<summary><strong>Packages</strong></summary>

```toml
# rex.toml
[dependencies]
mylib = { git = "https://github.com/user/mylib.git", ref = "main" }
utils = { path = "../utils" }
```

```bash
rex init                                       # create rex.toml
rex install                                    # fetch all dependencies
rex install https://github.com/user/pkg main   # add git dependency
rex install ../utils                           # add local path dependency
```

</details>

## Examples

The `examples/` directory has runnable code for every feature:

| File | What it shows |
| --- | --- |
| `http.rex` | HTTP server with pattern-matching routing |
| `actors.rex` | Stateful counter actor with typed messages |
| `tcp_echo.rex` | TCP echo server with `Std:Net` and actors |
| `json_decode.rex` | Elm-style JSON decoder combinators |
| `pattern_match.rex` | Exhaustive matching on ADTs, tuples, lists, nested types |
| `traits.rex` | Custom traits, parameterized instances, constraint tracking |
| `parallel.rex` | Parallel map with `Std:Parallel` |
| `stream.rex` | Lazy infinite sequences |
| `records.rex` | Records: creation, access, update, nested dot-paths |
| `pipe.rex` | Pipe operator `\|>` |
| `adt.rex` | Algebraic data types |
| `io.rex` | File I/O with `Result` |
| `testing.rex` | Built-in test framework |

Run any example:

```bash
rex examples/http.rex              # run (needs main)
rex --test examples/traits.rex     # run tests
```

## Running tests

```bash
rex --test internal/stdlib/rexfiles/*.rex   # stdlib tests
go test ./...                               # Go-level tests
make test                                   # everything
```

## Status

Rex is a personal project. The language is usable and has a real standard library, but it is not yet stable. Expect breaking changes.

What works well today: the type system, pattern matching, actors, the Go compilation pipeline, the standard library, and the package system. The JavaScript backend covers the full language. The WasmGC backend is in progress.

## License

MIT
