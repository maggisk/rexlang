# JS FFI: Companion Files for the Browser Target

## Problem

Rex compiles to JavaScript for the browser (`--compile --target=browser`), but
there's no way for user code or library authors to provide JavaScript
implementations for `external` declarations. Today:

- **Stdlib builtins** (String, Math, etc.) are hardcoded in
  `internal/codegen/javascript.go` — each one is hand-written inline JS.
- **Std:Js** uses `error "browser-only builtin"` stubs — the codegen intercepts
  calls by name and emits inline JS. Not extensible by users.
- **Per-function `.js` files** exist as a mechanism (`loadCompanionJS` in
  `resolve.go`) but are unused — no stdlib or user code ships `.js` companions.

The Go FFI solves this well: companion `.go` files sit alongside `.rex` files,
the compiler discovers them by convention, and `rex check` validates they
compile. We need the JS equivalent.

## Goals

1. **Same `external` keyword** — `external name : Type` works for both Go and JS
   targets. No new syntax.
2. **ESM output** — browser target emits ES modules. Modern browsers load them
   natively via `<script type="module">`. No bundler required for development.
3. **Optional bundler for prod** — users can point esbuild/Vite at the output
   for optimized production builds.
4. **Library support** — packages (tea-rex, rex-db) can ship JS companions
   alongside Go companions.
5. **No auto-conversion** — companions work with Rex's JS runtime
   representations directly. Generated helpers and constructors make this
   ergonomic.

---

## Output model

### ESM-only — no IIFE

The current `--target=browser` emits a single IIFE-wrapped `.js` file. This
changes to an **output directory** of ES modules:

```bash
rex --compile --target=browser src/Main.rex              # default: ./dist/
rex --compile --target=browser --out=public src/Main.rex  # custom output dir
```

Output directory:
```
dist/
  Main.mjs              # compiled Rex program (all Rex code)
  rex_prelude.mjs       # runtime helpers (list conversion, constructors)
  String.mjs            # stdlib companion (copied from embedded)
  Math.mjs              # stdlib companion
  Canvas.mjs            # user companion (copied from src/)
  index.html            # <script type="module" src="Main.mjs">
```

`Main.mjs` has real `import` statements pointing at companions:
```javascript
import { length, toUpper } from "./String.mjs";
import { drawCircle } from "./Canvas.mjs";

// ... compiled program ...

export function main() { return $main(null); }
```

### Development workflow

```bash
rex --compile --target=browser --out=public src/Main.rex
cd public && python3 -m http.server
# open http://localhost:8000 — browser resolves ESM imports natively
```

No bundler, no build step. Reload to see changes.

### Production workflow

```bash
rex --compile --target=browser --out=dist src/Main.rex
npx esbuild dist/Main.mjs --bundle --outfile=public/app.js
# or: npx vite build dist/
```

User brings their own bundler. Tree-shaking, minification, npm dependency
resolution all handled by the bundler.

### Why drop IIFE

The IIFE mode created a dual-mode problem: companion files needed to work
both as standalone scripts (IIFE inlining) and as ES modules (ESM imports).
This required fragile import stripping and special handling.

ESM-only eliminates this complexity. The cost — a directory instead of a single
file — is minimal. `<script type="module">` is supported in all modern browsers.
For users who need a single file (legacy browsers, embedding), a bundler
produces one.

---

## Companion file format

### Location & naming

Companion `.mjs` files live alongside `.rex` files, **one per module**:

```
# Stdlib (embedded in binary)
internal/stdlib/rexfiles/
  String.rex          # external length, toUpper, split, ...
  String.go           # Go companion (existing)
  String.mjs          # JS companion (new)
  Math.rex
  Math.go
  Math.mjs
  Http/
    Server.rex
    http.server.go    # Go companion (existing convention: lowercase dotted)
    Server.mjs        # JS companion: same name as .rex file

# User library
tea-rex/src/
  Html.rex
  Html.mjs            # JS companion

# User application
my-app/src/
  Canvas.rex          # external drawCircle, clearCanvas, ...
  Canvas.mjs          # JS companion
```

**Convention**: the companion file has the same basename as the `.rex` file,
with `.mjs` extension.

**Why `.mjs`?** Avoids collision with compilation output (which uses `.js` or
`.mjs` for the compiled entry point). Signals "ES module" — companions are
standard ES modules that can be tested with Node.js, imported by other JS code,
and resolved by any bundler.

### Contents

Companions are **standard ES modules** with named exports matching the
`external` declarations in the corresponding `.rex` file:

```javascript
// String.mjs — JS companion for Std:String

import { arrayToList, listToArray } from "./rex_prelude.mjs";

export function length(s) {
    return s.length;
}

export function toUpper(s) {
    return s.toUpperCase();
}

export function split(sep, s) {
    return arrayToList(s.split(sep));
}

export function join(sep, lst) {
    return listToArray(lst).join(sep);
}
```

**Named exports** — the compiler imports specific names (`import { length }
from "./String.mjs"`). Default exports are not used.

**Parameter order** matches the Rex type signature. If
`external split : String -> String -> List String`, the companion function
is `export function split(sep, s)` — first param is first arrow, second is
second.

**Currying is handled by the compiler**, not the companion. The companion
always receives all arguments uncurried. The codegen wraps multi-argument
companions in curried arrow functions:

```javascript
// Generated in Main.mjs
import { split } from "./String.mjs";
const Std$String$split = (a0) => (a1) => split(a0, a1);
```

**Helper functions** in the companion that aren't `external` are private.
They're not exported and invisible to Rex.

### External imports

Companions can import anything — browser APIs, npm packages, other JS:

```javascript
// Markdown.mjs
import markdownIt from "markdown-it";
const md = markdownIt();

export function render(s) {
    return md.render(s);
}
```

In dev mode (native ESM), bare specifiers like `"markdown-it"` won't resolve
in the browser — the user needs an import map or a dev server like Vite that
rewrites imports. Companions that only use browser globals and the Rex prelude
work without any tooling.

---

## `rex_prelude.mjs` — Runtime helpers

The prelude provides constructors and conversion utilities for Rex's JS
runtime types. Generated by `rex check --target=browser` (see validation
section). Committed to the repo.

### Contents

```javascript
// rex_prelude.mjs — generated, do not edit

// -- Lists --

export function listToArray(lst) {
    const arr = [];
    while (lst !== null) {
        arr.push(lst.head);
        lst = lst.tail;
    }
    return arr;
}

export function arrayToList(arr) {
    let lst = null;
    for (let i = arr.length - 1; i >= 0; i--) {
        lst = { $tag: "Cons", head: arr[i], tail: lst };
    }
    return lst;
}

export function cons(head, tail) {
    return { $tag: "Cons", head, tail };
}

export const nil = null;

// -- Maybe --

export function just(val) {
    return { $tag: "Just", $type: "Maybe", _0: val };
}

export const nothing = { $tag: "Nothing", $type: "Maybe" };

// -- Result --

export function ok(val) {
    return { $tag: "Ok", $type: "Result", _0: val };
}

export function err(msg) {
    return { $tag: "Err", $type: "Result", _0: msg };
}

// -- Tuples --

export function tuple2(a, b) { return [a, b]; }
export function tuple3(a, b, c) { return [a, b, c]; }

// -- Unit --

export const unit = null;

// -- Actor bridge --

export function send(pid, msg) {
    if (pid._resume) {
        const fn = pid._resume;
        pid._resume = null;
        fn(msg);
    } else {
        pid.ch.push(msg);
    }
}
```

### `send` in the prelude

The prelude exports `send` so that companions can push async results into the
actor system from Promise callbacks:

```javascript
// In a companion:
import { send, ok, err } from "./rex_prelude.mjs";

export function httpFetch(url, pid) {
    fetch(url)
        .then(r => r.text())
        .then(text => send(pid, ok(text)))
        .catch(e => send(pid, err(e.message)));
    return null; // unit
}
```

This enables async JS interop without async/await in Rex (see Async section).

---

## Generated `<Module>.types.mjs`

Like `*.types.go` for the Go FFI, the compiler generates JS constructor
functions for ADTs and records:

```javascript
// Html.types.mjs — generated, do not edit

// type Html = Text String | Element String (List Attribute) (List Html)
export function Text(a0) {
    return { $tag: "Text", $type: "Html", _0: a0 };
}
export function Element(a0, a1, a2) {
    return { $tag: "Element", $type: "Html", _0: a0, _1: a1, _2: a2 };
}

// type Attribute = Attr String String | Event String (JsRef -> Msg)
export function Attr(a0, a1) {
    return { $tag: "Attr", $type: "Attribute", _0: a0, _1: a1 };
}
export function Event(a0, a1) {
    return { $tag: "Event", $type: "Attribute", _0: a0, _1: a1 };
}
```

Companion usage:

```javascript
import { Text, Element, Attr } from "./Html.types.mjs";
import { arrayToList } from "./rex_prelude.mjs";

Element("div",
    arrayToList([Attr("class", "wrapper")]),
    arrayToList([Text("hello")]))
```

Generated by `rex check --target=browser`. Committed to the repo. Mirrors Go's
`*.types.go` exactly — same trigger, same lifecycle.

Only modules with `type` declarations produce output. Pure external modules
(no type declarations) produce no `.types.mjs`.

---

## Async without async/await

Rex has no async/await keywords. Async JS interop works through the existing
actor system and CPS-transformed `receive`.

### How it works

The JS backend CPS-transforms `receive` — code after `receive` becomes a
callback:

```rex
-- Rex code (inside an actor)
httpFetch "https://example.com" (self ())
let result = receive ()
in handleResult result
```

```javascript
// Compiled JS
httpFetch("https://example.com", $self());
$receive_cps($self(), (result) => {
    handleResult(result);
});
// execution returns here — actor is suspended
```

The companion starts an async operation and calls `send` when done:

```javascript
import { send, ok, err } from "./rex_prelude.mjs";

export function httpFetch(url, pid) {
    fetch(url)
        .then(r => r.text())
        .then(text => send(pid, ok(text)))
        .catch(e => send(pid, err(e.message)));
    return null;
}
```

Flow:
1. Actor calls `httpFetch(url, self())` — starts fetch, returns immediately.
2. Actor hits `receive()` — no message yet. Registers callback, returns.
3. Synchronous execution ends. Browser event loop runs.
4. `fetch` resolves. `.then` fires. Calls `send(pid, ok(text))`.
5. `send` sees `_resume` is set, invokes it. Actor code resumes.

No async/await. No Promises in Rex. The CPS transform + actor messaging is
sufficient.

### `main` has no pid

`main` cannot call `receive` — it has no actor identity. This is a feature:
top-level code is always synchronous and deterministic. Async requires
explicitly entering the actor world via `spawn`.

### TEA integration (tea-rex)

In the TEA architecture, `update` returns a command as a function:

```rex
update : Msg -> Model -> (Model, Pid Msg -> ())
```

The second element is a function that the framework spawns in an actor. It
receives the app's pid (for sending results back) and runs in an actor context
where `receive` works:

```rex
update msg model =
    match msg
        when FetchClicked ->
            (model, \appPid ->
                httpFetch "/api/data" (self ())
                let result = receive ()
                in send appPid (GotResponse result))
        when GotResponse (Ok text) ->
            ({ model | data = text }, noCmd)

noCmd : Pid msg -> ()
noCmd _ = ()
```

The framework handles the spawn:
```rex
-- tea-rex internals
let (newModel, cmd) = update msg model
in spawn \_ -> cmd appPid
```

This means:
- `update` itself is synchronous — it constructs a closure and returns.
- The closure can't accidentally block — it only runs inside a spawned actor.
- No special `Cmd` type needed — commands are just `Pid Msg -> ()` functions.

The framework provides convenience helpers:

```rex
httpGet : String -> (Result String String -> msg) -> Pid msg -> ()
httpGet url toMsg appPid =
    httpFetch url (self ())
    let result = receive ()
    in send appPid (toMsg result)
```

User writes:
```rex
(model, httpGet "/api/data" GotResponse)
```

---

## Compiler integration

### Discovery

When compiling with `--target=browser`, the import resolver looks for
companion `.mjs` files:

```
Module "Std:String"    → internal/stdlib/rexfiles/String.mjs (embedded)
Module "tearex:Html"   → <package-src>/Html.mjs
User module "Canvas"   → <src-root>/Canvas.mjs
```

**Target overlays**: `Foo.browser.rex` is merged with `Foo.rex` before
companion lookup. The companion implements the merged set of externals.
Target-only modules (e.g., `Js.browser.rex` with no base `Js.rex`) work the
same way — the companion is `Js.mjs`.

### Unresolved externals

If no companion exists, behavior depends on the builtin:

1. **Codegen-handled builtins** (Std:Js, actor runtime, core runtime): no
   companion needed, codegen emits inline JS as today.
2. **Stdlib builtins with companions**: companion provides the implementation.
3. **Unresolved externals**: compile error listing the missing functions and
   the expected companion path.

### Output emission

1. Compile all Rex code into `Main.mjs` (all modules inlined, as today).
2. For each module with externals resolved via companion:
   - Copy the companion `.mjs` to the output directory.
   - Emit `import { name1, name2 } from "./Companion.mjs"` at top of `Main.mjs`.
   - Emit curried wrappers: `const Mod$name = (a0) => (a1) => name(a0, a1);`
3. Copy `rex_prelude.mjs` to the output directory.
4. Generate `index.html` with `<script type="module" src="Main.mjs">`.

---

## Stdlib migration

### Currently hardcoded builtins

The JS codegen currently inlines ~40 stdlib builtins as hand-written JS in
`emitRuntimeHelpers()` and `emitApp()`. These move to companion `.mjs` files:

| Module | Current | After |
|--------|---------|-------|
| `Std:String` | Hardcoded in codegen | `String.mjs` companion |
| `Std:Math` | Hardcoded in codegen | `Math.mjs` companion |
| `Std:IO` | Hardcoded (print only) | `IO.mjs` companion |
| `Std:Json` | Hardcoded in codegen | `Json.mjs` companion |

### What stays in codegen

**Std:Js** — the JS FFI primitives emit fundamentally different JS patterns
(property access, method calls, constructor invocation). These are codegen
transforms, not function calls.

**Actor runtime** — `spawn`, `send`, `receive`, `self`, `call` require CPS
transformation of surrounding code. The `receive` call restructures the
continuation. Must stay in codegen.

**Core runtime** — `$eq`, `$compare`, `$display`, `$$apply` are internal
runtime functions, not external builtins.

### Migration path

1. Implement companion loading and ESM output.
2. Move stdlib builtins to `.mjs` companions one module at a time.
3. Remove hardcoded builtin emission from codegen.

Each phase is independently shippable. Phase 1 unblocks user/library FFI.

---

## Validation: `rex check --target=browser`

Extend `rex check` to validate JS companions:

```bash
rex check                   # validate Go companions (existing)
rex check --target=browser  # validate JS companions (new)
```

### What it does

1. Parse all `.rex` files, collect `external` declarations per module.
2. For each module with externals, look for `<Module>.mjs`.
3. Check that each expected function name exists as a top-level
   `export function` declaration in the companion.
4. Generate `rex_prelude.mjs` in `src/` (committed, for dev-time imports).
5. Generate `<Module>.types.mjs` for each module with type declarations.
6. Report missing companions and missing function names.

### What it does NOT do

Full validation (running the JS, type checking) is left to the user's test
suite. This matches Go FFI where `rex check` runs `go build` but doesn't
execute. JS has no static compilation step to leverage.

---

## Package/library support

### Library structure

A Rex library targeting both Go and browser ships both companion types:

```
tea-rex/
  rex.toml              # [package] name = "tearex"
  src/
    Html.rex             # type Html, external renderToDOM, ...
    Html.go              # Go companion
    Html.mjs             # JS companion
    Html.types.go        # Generated by rex check
    Html.types.mjs       # Generated by rex check --target=browser
    rex_runtime.go       # Generated by rex check
    rex_prelude.mjs      # Generated by rex check --target=browser
  go.mod                 # Generated by rex check
```

### Consuming libraries

When an app imports a package with JS companions:

```toml
# my-app/rex.toml
[dependencies]
tearex = { path = "../tea-rex" }
```

The compiler follows the same path as Go companions:
1. Resolve package source root from `rex.toml`.
2. For each module with externals, look for `<Module>.mjs` in the package.
3. Copy companion to output directory, emit import in `Main.mjs`.

---

## Type representation reference

Companions receive and return Rex values in their JS representation:

| Rex type | JS representation | Example |
|----------|-------------------|---------|
| `Int` | `number` (integer) | `42` |
| `Float` | `number` (float) | `3.14` |
| `String` | `string` | `"hello"` |
| `Bool` | `boolean` | `true` |
| `()` | `null` | `null` |
| `List a` | cons cells / `null` | `{$tag:"Cons", head:1, tail:null}` |
| `(a, b)` | array | `[1, "two"]` |
| `(a, b, c)` | array | `[1, 2, 3]` |
| `Maybe a` | tagged object | `{$tag:"Just", $type:"Maybe", _0:42}` |
| `Result a b` | tagged object | `{$tag:"Ok", $type:"Result", _0:val}` |
| Record | plain object | `{x:10, y:20}` |
| ADT | tagged object | `{$tag:"Circle", $type:"Shape", _0:5}` |
| `a -> b` | function | `(a) => b` |
| `JsRef` | raw JS value | anything |

**ADT field numbering**: constructor arguments are `_0`, `_1`, `_2`, etc.
Zero-argument constructors have no `_N` fields.

**The `$type` field**: present on ADT values, holds the type name. Used by
trait dispatch. Generated constructors in `*.types.mjs` include it.

---

## Error messages

### Missing companion file

```
Error: Module "Canvas" has 3 external declarations but no JS companion.
       Expected: src/Canvas.mjs

       Externals without implementations:
         drawCircle : Int -> Int -> Int -> ()
         clearCanvas : () -> ()
         getContext : String -> Result JsRef String
```

### Missing function in companion

```
Error: src/Canvas.mjs is missing implementation for external "getContext".

       The companion exports: drawCircle, clearCanvas
       The .rex file declares: drawCircle, clearCanvas, getContext
```

### Go-only externals compiled for browser

```
Error: Module "Db" has externals with no JS companion.
       This module has a Go companion (Db.go) but no JS companion (Db.mjs).
       Add Db.mjs or remove the browser-target import.
```

---

## Examples

### Simple companion (browser APIs only)

```rex
-- Canvas.rex
import Std:Result (Ok, Err)
import Std:Js (JsRef)

external getContext2d : JsRef -> Result JsRef String
external fillRect : Int -> Int -> Int -> Int -> JsRef -> ()
external setFillStyle : String -> JsRef -> ()
```

```javascript
// Canvas.mjs
import { ok, err } from "./rex_prelude.mjs";

export function getContext2d(canvas) {
    try {
        const ctx = canvas.getContext("2d");
        if (!ctx) return err("getContext returned null");
        return ok(ctx);
    } catch (e) {
        return err(e.message);
    }
}

export function fillRect(x, y, w, h, ctx) {
    ctx.fillRect(x, y, w, h);
    return null;
}

export function setFillStyle(color, ctx) {
    ctx.fillStyle = color;
    return null;
}
```

### Async companion (fetch via actor bridge)

```rex
-- Http.rex
import Std:Result (Ok, Err)
import Std:Process (Pid)

external httpFetch : String -> Pid (Result String String) -> ()
```

```javascript
// Http.mjs
import { send, ok, err } from "./rex_prelude.mjs";

export function httpFetch(url, pid) {
    fetch(url)
        .then(r => r.text())
        .then(text => send(pid, ok(text)))
        .catch(e => send(pid, err(e.message)));
    return null;
}
```

Rex usage:
```rex
import Http (httpFetch)
import Std:Process (spawn, self, receive, send)

fetchAndPrint url appPid =
    httpFetch url (self ())
    let result = receive ()
    in send appPid result
```

### Companion with npm dependency

```rex
-- Markdown.rex
external render : String -> String
```

```javascript
// Markdown.mjs
import markdownIt from "markdown-it";
const md = markdownIt();

export function render(s) {
    return md.render(s);
}
```

Requires a bundler or import map to resolve `"markdown-it"`.

### Library companion (tea-rex)

```rex
-- Html.rex (in tea-rex)
external mountApp : String -> (JsRef -> ()) -> ()
```

```javascript
// Html.mjs (in tea-rex)
import { listToArray } from "./rex_prelude.mjs";
import { Text, Element, Attr } from "./Html.types.mjs";

export function mountApp(selector, renderCallback) {
    const root = document.querySelector(selector);
    renderCallback(root);
    return null;
}

// Internal helper — not exported to Rex
function vdomToNode(vdom) {
    if (vdom.$tag === "Text") {
        return document.createTextNode(vdom._0);
    }
    const el = document.createElement(vdom._0);
    for (const attr of listToArray(vdom._1)) {
        if (attr.$tag === "Attr") {
            el.setAttribute(attr._0, attr._1);
        }
    }
    for (const child of listToArray(vdom._2)) {
        el.appendChild(vdomToNode(child));
    }
    return el;
}
```

---

## Decisions

- **ESM-only output.** Drop IIFE. Browsers support `<script type="module">`
  natively. Eliminates dual-mode complexity. Bundler optional for prod.

- **Output is a directory.** `--out` flag specifies location (default `./dist/`).
  Contains `Main.mjs`, companions, prelude, and `index.html`.

- **Companions use `export`.** Standard ES modules. Testable with Node.js.
  Importable by other JS code. Resolvable by any bundler.

- **One `.mjs` per module** (not per function). Mirrors Go FFI. Replaces the
  current unused per-function `.js` mechanism.

- **No auto-conversion.** Companions receive and return Rex's JS runtime
  representations directly. `rex_prelude.mjs` and `*.types.mjs` provide
  ergonomic helpers.

- **Generated `.types.mjs`.** Constructor functions for ADTs and records,
  like Go's `*.types.go`. Generated by `rex check --target=browser`.
  Committed to repo.

- **`rex_prelude.mjs` generated and committed.** Contains list conversion,
  Maybe/Result constructors, and `send` for actor bridge. Generated by
  `rex check --target=browser`, committed for dev-time imports.

- **Std:Js stays intercepted in codegen.** The JS FFI primitives are codegen
  transforms, not function implementations. Same for actor CPS runtime.

- **Currying handled by compiler.** Companions are always uncurried — they
  receive all arguments. The codegen wraps them in curried form based on
  the `external` type signature arity.

- **Async via actors, not language keywords.** No async/await in Rex.
  Companions use Promises and call `send` to push results into the actor
  system. TEA commands are `Pid Msg -> ()` functions — the framework
  spawns actors to run them.

- **`main` has no pid.** Top-level code is synchronous. Async requires
  `spawn`. This is a deliberate boundary, not a limitation.

---

## Open questions

1. **Stdlib companion embedding.** Stdlib `.go` companions are embedded via
   `//go:embed` in `internal/stdlib/embed.go`. Stdlib `.mjs` companions need
   the same treatment. The embed directives need to include `*.mjs` files.

2. **Pre-bundled companions.** Should we support a `Canvas.bundled.mjs`
   convention where the user pre-bundles their companion (resolving npm
   imports)? The compiler would prefer the bundled version. This lets users
   with npm deps avoid a full project bundler setup.

3. **Validation depth.** Go FFI runs `go build`. JS has no equivalent static
   check. Is verifying that export names exist sufficient, or should we run
   a JS parser for deeper validation?

4. **Tree-shaking companions.** If only one function from a companion is used,
   the entire file is still copied to the output. JS bundlers handle dead code
   elimination well, but in unbundled dev mode there's no tree-shaking.
   Acceptable?

5. **`send` in prelude vs runtime.** The prelude duplicates the `send`
   implementation from the actor runtime in `Main.mjs`. Should they share
   a single implementation (prelude is authoritative, runtime imports from
   it)? Or is duplication acceptable for simplicity?
