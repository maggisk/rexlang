# Go FFI: Library Development Support

## Problem

Library authors (rex-db, tea-rex) can't verify their Go companion code compiles.
Today companions have `//go:build ignore` and only compile when an app imports
them. Typos in type names, wrong field access, missing imports — all discovered
late.

Companion code references types that don't exist as Go source:
- Runtime types: `RexList`, `Tuple2` (from the Rex runtime)
- Module types: `Rex_SqlValue_SqlText` (from the library's own Rex types)

## Solution

Generate the missing Go source files so companions compile during library dev.

### Generated files

Given a library with `rex.toml` containing `[package] name = "db"`:

```
rex-db/
  rex.toml
  src/
    Db.rex               # defines type SqlValue, external rawOpen, etc.
    Db.go                # companion — references RexList, Rex_SqlValue_*
    Db.types.go          # GENERATED, COMMITTED — Go types for SqlValue ADT
    rex_runtime.go       # GENERATED, COMMITTED — RexList, Tuple2, etc.
  go.mod                 # GENERATED, COMMITTED — from [go.requires] in rex.toml
  go.sum                 # GENERATED, COMMITTED
```

Generated files are committed to the repo (like `go.sum` or protobuf stubs).
This means Go LSP works immediately after clone — no setup step needed.
CI verifies freshness: `rex check && git diff --exit-code`.

**1. `<Module>.types.go`** — per-module, generated next to `<Module>.rex`

Contains Go struct/interface definitions for types with explicit `type`
declarations (ADTs and records) in that Rex module. Same shapes the codegen
already emits in `emitTypeDefinitions()`.

Types use `any` erasure for non-primitive fields — matching the app build
exactly. E.g., a field of type `List SqlValue` becomes `any`, not
`*RexList`. This is correct: companion code uses type assertions to work
with these values, just like in the app build.

Phantom/opaque-handle types like `Connection` that appear in `external`
signatures but have no `type` declaration are NOT generated — they're `any`
at the Go level and don't need struct definitions.

Example `Db.types.go`:
```go
package db

type Rex_SqlValue interface{ tagRex_SqlValue() int }

type Rex_SqlValue_SqlText struct{ F0 string }
func (Rex_SqlValue_SqlText) tagRex_SqlValue() int { return 0 }

type Rex_SqlValue_SqlInt struct{ F0 int64 }
func (Rex_SqlValue_SqlInt) tagRex_SqlValue() int { return 1 }

// ... etc
```

**2. `rex_runtime.go`** — one per directory that contains companion `.go` files

Named `rex_runtime.go` (not `rex.go`) to avoid case-insensitive filesystem
collisions on macOS — a module named `Rex.rex` with companion `Rex.go`
would conflict with `rex.go` on HFS+/APFS.

Contains the Rex runtime types that companion code needs. NOT the full runtime —
just the types and helpers that companions actually reference:

```go
package db

type RexList struct {
    Head any
    Tail *RexList
}

type Tuple2 struct{ F0, F1 any }
type Tuple3 struct{ F0, F1, F2 any }
type Tuple4 struct{ F0, F1, F2, F3 any }
```

That's it. ~10 lines. No `rex_eq`, `rex_display`, actor primitives, etc. —
those are called by generated code, never by companions.

**3. `go.mod`** — at library root, from `[go.requires]`

```
module rexlib/db
go 1.24
require modernc.org/sqlite v1.37.1
```

Module path is synthetic (`rexlib/<name>`) — it only exists so `go build`
can resolve imports during library dev. Never published as a Go module.

`go mod tidy` runs as part of `rex check`, which may add indirect
dependencies to `go.mod`. This is expected — commit the result.

### Companion format change

Drop `//go:build ignore`. Use the library's package name:

```go
// Before (current)
//go:build ignore

package main

import "database/sql"
func db_Db_rawOpen(path string) (any, error) { ... }
```

```go
// After
package db

import "database/sql"
func db_Db_rawOpen(path string) (any, error) { ... }
```

Stdlib companions (`internal/stdlib/rexfiles/`) keep `//go:build ignore` — they're
embedded in the binary and compiled differently.

### New command: `rex check`

Run in a library directory:

```bash
cd rex-db && rex check
```

1. Rex type-check all `.rex` files. **Abort on failure** — generated types
   would be garbage from a partially-checked program.
2. Generate `rex_runtime.go` in each directory containing companions
3. Generate `<Module>.types.go` for each module with `type` declarations
4. Generate `go.mod` at library root from `[go.requires]`
5. Run `go mod tidy` (fetches third-party Go deps)
6. Run `go build ./src/` to verify compilation
7. Report success or Go compiler errors

For CI: `rex check` exits non-zero if Rex type-checking or Go compilation fails.

If no companion `.go` files exist, skip Go compilation (pure Rex library).

### App build changes

The app build pipeline (`.cache/rex-build/`) is mostly unchanged.
When copying library companions:

1. Read companion `.go` file
2. Rewrite package declaration: `(?m)^package\s+\w+` → `package main`
3. Write to `.cache/rex-build/pkg_<ns>_<mod>.go`

**Important**: there are two companion-copy code paths in `cmd/rex/main.go` —
one for `rex file.rex` (run mode) and one for `rex build` (build mode). Both
need the package-name rewrite. Currently both do prefix stripping of
`//go:build ignore` — replace with regex-based package rewrite.

Types and runtime are generated centrally in the build dir as before:
- `runtime.go` — full runtime (superset of `rex_runtime.go`)
- Type definitions emitted in `main.go` — covers ALL modules from app + deps

Library-local `rex_runtime.go` and `*.types.go` are NOT copied to the build dir.
They served their purpose during library dev; the app build regenerates everything.

---

## The `rex_runtime.go` sharing question

The worry: library companions are written against library-local `rex_runtime.go`.
At app build time, they're compiled against `runtime.go` in `.cache/rex-build/`.
Are the types guaranteed to match?

### Why it works

**Same compiler, same types.** `rex_runtime.go` and `runtime.go` are both
generated by the Rex compiler. The type definitions are identical because they
come from the same Go source code (`internal/codegen/runtime.go`).
`rex_runtime.go` is a strict subset of `runtime.go`.

**No separate compilation.** Rex libraries don't produce binary artifacts.
At app build time, everything is recompiled from source. The library's
`rex_runtime.go` is discarded — `runtime.go` takes its place. As long as the
shapes match (guaranteed by construction), companion code compiles against
either one.

**What could go wrong:**

1. *Compiler version mismatch* — Library author runs `rex check` with Rex v1,
   app builds with Rex v2 which changed `RexList`'s field names. The companion
   references `.Head` but v2 renamed it to `.Val`.

   This is a backwards-incompatible change to the runtime. It would break
   everything, not just library dev. The fix is: don't break runtime types
   (or bump a major version). Same contract as any language runtime.

2. *Stale generated files* — Author runs `rex check`, edits types in `.rex`,
   doesn't re-run `rex check`. The `*.types.go` is stale. Go LSP shows errors.

   Self-correcting: the author re-runs `rex check` to fix it. Stale types
   never affect app builds (types are always regenerated).

### What `rex_runtime.go` must NOT contain

- **Functions** (`rex_eq`, `rex_display`, etc.) — companions never call these.
  Including them would create a maintenance surface. If we later change
  `rex_eq`'s implementation, we'd need to worry about library-local copies.

- **Actor types** (`RexPid`, channel setup) — if a library needs actor FFI,
  we add `RexPid` to `rex_runtime.go` later. Not needed for any current use case.

- **Import statements** — `rex_runtime.go` should have zero imports. The types are
  self-contained (`any`, `*RexList`, primitive Go types). This eliminates
  dependency issues.

### What `rex_runtime.go` MUST contain

The minimal set of types that companion code references:

| Type | Why companions need it |
|------|----------------------|
| `RexList` | List params/returns in external functions |
| `Tuple2` | Tuple params/returns |
| `Tuple3` | Tuple params/returns |
| `Tuple4` | Tuple params/returns |

If we later find companions need more runtime types, we add them to
`rex_runtime.go` AND ensure `runtime.go` still defines the same types (which
it will, since `rex_runtime.go` is generated from a subset of the same source).

---

## Cross-module type references

A companion may need types from OTHER modules in the same library.
E.g., a `Query.go` companion that references types from `Db.rex`.

Since all companions in `src/` share one Go package, all `*.types.go` files
are visible to all companions in the same directory. This works naturally.

For nested modules (`src/Http/Server.go` needing types from `src/Db.rex`):
this doesn't work because Go treats subdirectories as separate packages.

**Decision: Go FFI companions must be in the same directory** (typically `src/`).
Nested Rex modules can exist but can't have Go companions that reference types
from other directories. This covers all current use cases. If needed later,
we can add a flattened dev-build approach.

---

## Migration

1. Library companions: remove `//go:build ignore`, change `package main` →
   `package <name>`. One-time manual change.
2. App build: update companion copy logic — replace build-tag stripping with
   regex-based package-name rewrite. Update BOTH code paths (run + build).
3. Run `rex check` in each library to generate files, commit them.

---

## Decisions

- **go.mod location**: library root. `go build ./src/` from root. Keeps
  `go.mod` next to `rex.toml` — both config files in one place.

- **Companion format**: drop `//go:build ignore`, use `package <name>` from
  rex.toml. Breaking change — acceptable since we're pre-public with only
  1-2 libraries.

- **Generated file naming**: `rex_runtime.go` (not `rex.go`) to avoid
  case-insensitive collision with `Rex.go` on macOS.

- **Type generation scope**: only types with explicit `type` declarations
  (ADTs + records). Phantom types in `external` signatures are `any`.
  Generated types use `any` erasure for non-primitive fields — same as
  app build.

- **Cross-package type references**: not supported during library dev.
  Companions reference only their own library's types + runtime types.
  At app build time, the flat build dir makes all types available.
  If needed later, could add `replace` directives in generated `go.mod`
  pointing to dependency paths from `rex.toml`.

- **Generated files committed**: no gitignore. Go LSP works on clone.
  CI checks freshness via `rex check && git diff --exit-code`.

---

## Open questions

1. **Auto-generate on `rex install`?** When someone runs `rex install` in a
   library, should it also generate types? Probably yes — they may be about
   to edit companions.

2. **`go build` vs `go vet`?** `go vet` is faster (no linking) but `go build`
   catches more issues. Probably `go build` for correctness.
