# Rex Package Registry

The Rex package registry is a simple GitHub-based package index -- a single git repository that contains a list of known packages with their git URLs and versions. No web service is needed.

## Index Format

The package index is a JSON file (`index.json`) hosted at:

```
https://raw.githubusercontent.com/maggisk/rex-packages/main/index.json
```

It contains an array of package entries:

```json
[
  {
    "name": "tearex",
    "description": "Web framework for Rex with TEA architecture",
    "git": "https://github.com/maggisk/tea-rex.git",
    "latest_ref": "v0.1.0"
  },
  {
    "name": "rexql",
    "description": "SQL query builder for Rex",
    "git": "https://github.com/example/rexql.git",
    "latest_ref": "v0.2.0"
  }
]
```

### Fields

| Field        | Type   | Description                                    |
|-------------|--------|------------------------------------------------|
| `name`       | string | Package name (used in `rex install <name>`)    |
| `description`| string | Short description of the package               |
| `git`        | string | Git clone URL                                  |
| `latest_ref` | string | Latest version tag or commit SHA               |

## CLI Commands

### Search for packages

```bash
rex search <query>
```

Searches the package index by name and description (case-insensitive). Without a query, lists all available packages.

### Install by name

```bash
rex install <name>
```

Looks up the package in the registry, resolves to its git URL and latest ref, and installs it as a git dependency in `rex.toml`.

This is equivalent to:

```bash
rex install <git-url> <latest-ref>
```

### Install by URL (existing)

```bash
rex install <git-url> <ref>
```

Installs a package directly by git URL, without using the registry.

## How to Publish a Package

1. Create a Rex package with a `rex.toml` and `src/` directory
2. Push it to a public git repository
3. Tag a release (e.g., `v0.1.0`)
4. Submit a pull request to [maggisk/rex-packages](https://github.com/maggisk/rex-packages) adding your package to `index.json`

### Package requirements

- Must have a `rex.toml` with `[package]` section (name and version)
- Must have a `src/` directory with `.rex` source files
- Should use semantic versioning for tags (e.g., `v1.0.0`, `v0.2.1`)

## Versioning Conventions

- Use semantic versioning: `vMAJOR.MINOR.PATCH`
- The `latest_ref` in the index points to the latest stable release
- Breaking changes should bump the major version
- Tags should be git annotated tags (not lightweight)

## Future Plans

- Version ranges in `rex.toml` (e.g., `"^0.1.0"`)
- Automatic index updates via CI
- Package validation (check that the package builds before accepting PRs)
- Multiple versions per package in the index
