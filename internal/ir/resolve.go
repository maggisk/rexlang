// Module resolution for whole-program compilation.
//
// When compiling to Wasm, all imported module type declarations, trait
// definitions, and impl blocks must be available. This file provides
// ResolveImports which collects these declarations from imported modules
// (recursively) so they can be prepended to the main program before lowering.
package ir

import (
	"fmt"
	"os"
	"strings"

	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/stdlib"
)

// ResolveImports collects type declarations, trait declarations, impl blocks,
// and function definitions from all transitively imported modules.
// Returns them in dependency order (deepest imports first).
func ResolveImports(exprs []ast.Expr, srcRoot string) ([]ast.Expr, error) {
	r := &resolver{
		visited: make(map[string]bool),
		srcRoot: srcRoot,
	}
	if err := r.resolve(exprs); err != nil {
		return nil, err
	}
	return r.decls, nil
}

type resolver struct {
	visited map[string]bool
	decls   []ast.Expr
	srcRoot string
	stack   []string // for circular import detection
}

func (r *resolver) resolve(exprs []ast.Expr) error {
	for _, e := range exprs {
		imp, ok := e.(ast.Import)
		if !ok {
			continue
		}
		if r.visited[imp.Module] {
			continue
		}

		// Circular import detection
		for _, s := range r.stack {
			if s == imp.Module {
				return fmt.Errorf("circular import: %s", imp.Module)
			}
		}

		r.visited[imp.Module] = true
		r.stack = append(r.stack, imp.Module)

		src, err := r.loadSource(imp.Module)
		if err != nil {
			// Skip modules we can't load (e.g., IO which is builtins-only)
			r.stack = r.stack[:len(r.stack)-1]
			continue
		}

		modExprs, err := parser.Parse(src)
		if err != nil {
			r.stack = r.stack[:len(r.stack)-1]
			return fmt.Errorf("parse error in module %s: %w", imp.Module, err)
		}

		// Recursively resolve this module's imports first (depth-first)
		if err := r.resolve(modExprs); err != nil {
			r.stack = r.stack[:len(r.stack)-1]
			return err
		}

		// Extract type declarations from this module.
		// Only TypeDecl is needed for ADT struct generation in codegen.
		// TraitDecl and ImplDecl are skipped for now — trait dispatch for
		// imported types requires additional work. Function bodies are also
		// skipped since the codegen can't yet compile all stdlib patterns.
		for _, me := range modExprs {
			switch me.(type) {
			case ast.TypeDecl:
				r.decls = append(r.decls, me)
			}
		}

		r.stack = r.stack[:len(r.stack)-1]
	}
	return nil
}

func (r *resolver) loadSource(module string) (string, error) {
	if strings.Contains(module, ":") {
		parts := strings.SplitN(module, ":", 2)
		namespace, name := parts[0], parts[1]
		if namespace != "Std" {
			return "", fmt.Errorf("unknown namespace '%s'", namespace)
		}
		return stdlib.Source(name)
	}
	// User module — resolve from srcRoot
	if r.srcRoot == "" {
		return "", fmt.Errorf("user module import '%s' requires src/ directory", module)
	}
	modPath := strings.ReplaceAll(module, ".", "/")
	data, err := os.ReadFile(r.srcRoot + "/" + modPath + ".rex")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
