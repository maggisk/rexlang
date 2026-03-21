// Package manifest handles rex.toml and rex.local.toml parsing.
package manifest

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Manifest represents a rex.toml file.
type Manifest struct {
	Package  PackageInfo            `toml:"package"`
	Deps     map[string]Dependency  `toml:"dependencies"`
	Go       GoSection              `toml:"go"`
}

// GoSection is the [go] section for Go-level dependencies.
type GoSection struct {
	Requires map[string]string `toml:"requires"` // Go module path → version
}

// PackageInfo is the [package] section.
type PackageInfo struct {
	Name    string `toml:"name"`
	Version string `toml:"version"`
}

// Dependency is a single entry in [dependencies].
// Either git+ref (remote) or path (local) must be set.
type Dependency struct {
	Git  string `toml:"git,omitempty"`
	Ref  string `toml:"ref,omitempty"`
	Path string `toml:"path,omitempty"`
}

// LocalOverrides represents a rex.local.toml file.
type LocalOverrides struct {
	Overrides map[string]LocalDep `toml:"overrides"`
}

// LocalDep is a local path override for a dependency.
type LocalDep struct {
	Path string `toml:"path"`
}

// ResolvedDep is a dependency with overrides applied.
type ResolvedDep struct {
	Name string // import namespace
	Git  string // git URL (empty if local override)
	Ref  string // git tag or commit (empty if local override)
	Path string // absolute path to package root (set after resolution)
}

// Load reads rex.toml from the given directory, applies rex.local.toml
// overrides if present, and returns the resolved dependencies.
func Load(dir string) (*Manifest, []ResolvedDep, error) {
	manifestPath := filepath.Join(dir, "rex.toml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, nil, fmt.Errorf("no rex.toml found in %s", dir)
	}

	var m Manifest
	if err := toml.Unmarshal(data, &m); err != nil {
		return nil, nil, fmt.Errorf("rex.toml: %w", err)
	}

	// Load local overrides if present
	var local LocalOverrides
	localPath := filepath.Join(dir, "rex.local.toml")
	if localData, err := os.ReadFile(localPath); err == nil {
		if err := toml.Unmarshal(localData, &local); err != nil {
			return nil, nil, fmt.Errorf("rex.local.toml: %w", err)
		}
	}

	// Resolve dependencies
	var deps []ResolvedDep
	for name, dep := range m.Deps {
		rd := ResolvedDep{
			Name: name,
			Git:  dep.Git,
			Ref:  dep.Ref,
		}

		if dep.Path != "" {
			// Path dependency (local/monorepo)
			p := dep.Path
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			absPath, err := filepath.Abs(p)
			if err != nil {
				return nil, nil, fmt.Errorf("rex.toml: bad path for '%s': %w", name, err)
			}
			rd.Path = absPath
		} else if dep.Git != "" {
			// Git dependency — ref required
			if dep.Ref == "" {
				return nil, nil, fmt.Errorf("rex.toml: dependency '%s' must have a 'ref' field (tag or commit)", name)
			}
		} else {
			return nil, nil, fmt.Errorf("rex.toml: dependency '%s' must have either 'path' or 'git' + 'ref'", name)
		}

		// rex.local.toml override takes precedence
		if lo, ok := local.Overrides[name]; ok && lo.Path != "" {
			p := lo.Path
			if !filepath.IsAbs(p) {
				p = filepath.Join(dir, p)
			}
			absPath, err := filepath.Abs(p)
			if err != nil {
				return nil, nil, fmt.Errorf("rex.local.toml: bad path for '%s': %w", name, err)
			}
			rd.Path = absPath
		}
		deps = append(deps, rd)
	}

	return &m, deps, nil
}

// FindProjectRoot walks up from dir looking for a rex.toml file.
// Returns the directory containing it, or empty string if not found.
func FindProjectRoot(dir string) string {
	for {
		if _, err := os.Stat(filepath.Join(dir, "rex.toml")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// PackageRoots resolves all dependencies to a map of package name → src/ path.
// For local overrides, it uses the override path directly.
// For git dependencies, it looks in rex_modules/<name>/src/.
func PackageRoots(projectRoot string, deps []ResolvedDep) (map[string]string, error) {
	roots := make(map[string]string)
	for _, dep := range deps {
		if dep.Path != "" {
			// Local override — use path directly
			srcDir := filepath.Join(dep.Path, "src")
			if _, err := os.Stat(srcDir); err != nil {
				return nil, fmt.Errorf("package '%s' local override has no src/ directory: %s", dep.Name, srcDir)
			}
			roots[dep.Name] = srcDir
		} else {
			// Git dependency — use rex_modules/<name>/src/
			srcDir := filepath.Join(projectRoot, "rex_modules", dep.Name, "src")
			if _, err := os.Stat(srcDir); err != nil {
				return nil, fmt.Errorf("package '%s' not installed (run 'rex install'): %s", dep.Name, srcDir)
			}
			roots[dep.Name] = srcDir
		}
	}
	return roots, nil
}
