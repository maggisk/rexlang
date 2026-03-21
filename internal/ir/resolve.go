// Module resolution for whole-program compilation.
//
// When compiling (Go backend, JS backend), all imported module type
// declarations, trait definitions, and impl blocks must be available.
// This file provides ResolveImports which collects these declarations
// from imported modules (recursively) so they can be prepended to the
// main program before lowering.
package ir

import (
	"fmt"
	"os"
	"strings"

	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/stdlib"
	"github.com/maggisk/rexlang/internal/typechecker"
)

// ImportInfo holds resolved module declarations and name mappings.
type ImportInfo struct {
	Decls     []ast.Expr        // module declarations (types, functions, etc.)
	Aliases   map[string]string // imported name → prefixed module name (e.g. "length" → "Std_List__length")
	JsBindings []JsBinding      // companion .js file contents for external FFI
}

// JsBinding maps a mangled function name to its JS implementation source.
type JsBinding struct {
	MangledName string // e.g. "rex_tearex_Html__refEq"
	Source      string // contents of the .js file
}

// ModulePrefix converts a module path to a valid JS identifier prefix.
// e.g. "Std:List" → "Std$List$", "Std:Json.Decode" → "Std$Json$Decode$"
func ModulePrefix(module string) string {
	s := strings.ReplaceAll(module, ":", "$")
	s = strings.ReplaceAll(s, ".", "$")
	return s + "$"
}

// ResolveImports collects type declarations, trait declarations, impl blocks,
// and function definitions from all transitively imported modules.
// Returns them in dependency order (deepest imports first), with function
// names prefixed by module path to avoid collisions. Also returns a map
// of imported names to their prefixed equivalents.
func ResolveImports(exprs []ast.Expr, srcRoot string, target string, packageRoots map[string]string) (*ImportInfo, error) {
	r := &resolver{
		visited:      make(map[string]bool),
		srcRoot:      srcRoot,
		target:       target,
		packageRoots: packageRoots,
		aliases:      make(map[string]string),
		modTopNames:  make(map[string]map[string]bool),
	}
	// Always include Prelude declarations (types, traits, impls, functions),
	// unless the entry file IS the Prelude (detected by checking for trait Eq).
	isPrelude := false
	for _, e := range exprs {
		if td, ok := e.(ast.TraitDecl); ok && td.Name == "Eq" {
			isPrelude = true
			break
		}
	}
	if !isPrelude {
		preludeSrc, err := stdlib.Source("Prelude")
		if err == nil {
			preludeExprs, err := parser.Parse(preludeSrc)
			if err == nil {
				r.visited["Std:Prelude"] = true
				for _, me := range preludeExprs {
					switch me.(type) {
					case ast.TestDecl, ast.Export, ast.TypeAnnotation, ast.Import:
						continue
					default:
						r.decls = append(r.decls, me)
					}
				}
			}
		}
	}
	if err := r.resolve(exprs, true); err != nil {
		return nil, err
	}
	return &ImportInfo{
		Decls:     r.decls,
		Aliases:   r.aliases,
		JsBindings: r.jsBindings,
	}, nil
}

type resolver struct {
	visited      map[string]bool
	decls        []ast.Expr
	aliases      map[string]string            // user-visible name → prefixed name
	modTopNames  map[string]map[string]bool    // module → set of defined function names
	srcRoot      string
	packageRoots map[string]string // package name → abs path to package src/
	target       string            // compilation target ("native", "browser", etc.)
	stack        []string          // for circular import detection
	jsBindings   []JsBinding       // companion .js file bindings
}

func (r *resolver) resolve(exprs []ast.Expr, isRoot bool) error {
	// Also collect imports from inside test bodies
	var allImports []ast.Expr
	for _, e := range exprs {
		if _, ok := e.(ast.Import); ok {
			allImports = append(allImports, e)
		} else if td, ok := e.(ast.TestDecl); ok && isRoot {
			for _, te := range td.Body {
				if _, ok := te.(ast.Import); ok {
					allImports = append(allImports, te)
				}
			}
		}
	}
	for _, e := range allImports {
		imp, ok := e.(ast.Import)
		if !ok {
			continue
		}
		if r.visited[imp.Module] {
			// Module already processed, but still need to set up aliases
			// for the user's imports. Only alias names defined in the module.
			if isRoot {
				prefix := ModulePrefix(imp.Module)
				modNames := r.modTopNames[imp.Module]
				for _, name := range imp.Names {
					if modNames[name] {
						r.aliases[name] = prefix + name
					}
				}
			}
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

		// Topologically sort declarations so dependencies come before uses.
		// This is critical for JS codegen where const bindings can't be
		// referenced before initialization.
		modExprs, err = typechecker.ReorderToplevel(modExprs)
		if err != nil {
			r.stack = r.stack[:len(r.stack)-1]
			return fmt.Errorf("reorder error in module %s: %w", imp.Module, err)
		}

		// Check for companion JS files for external declarations
		if r.target == "browser" {
			r.loadCompanionJS(imp.Module, modExprs)
		}

		// Recursively resolve this module's imports first (depth-first)
		if err := r.resolve(modExprs, false); err != nil {
			r.stack = r.stack[:len(r.stack)-1]
			return err
		}

		prefix := ModulePrefix(imp.Module)

		// Collect all top-level binding names for renaming references within the module
		topNames := make(map[string]bool)
		for _, me := range modExprs {
			switch d := me.(type) {
			case ast.Let:
				if d.InExpr == nil { // top-level binding
					topNames[d.Name] = true
				}
			case ast.LetRec:
				for _, b := range d.Bindings {
					topNames[b.Name] = true
				}
			case ast.ExternalDecl:
				topNames[d.Name] = true
			case ast.TypeDecl:
				// Note: constructors are NOT prefixed because type declarations
				// keep their original names. Constructors are resolved by the
				// type system, not by name prefixing.
			}
		}
		r.modTopNames[imp.Module] = topNames

		// Build name map for renaming references within module bodies
		modNameMap := make(map[string]string)
		for name := range topNames {
			modNameMap[name] = prefix + name
		}

		// Also resolve this module's own imports: map imported names to
		// their prefixed counterparts so that references like `map` from
		// `import Std:List (map)` become `Std$List$map` in the body.
		for _, me := range modExprs {
			innerImp, ok := me.(ast.Import)
			if !ok {
				continue
			}
			innerPrefix := ModulePrefix(innerImp.Module)
			innerNames := r.modTopNames[innerImp.Module]
			// Selective imports: import Std:List (map, filter)
			for _, name := range innerImp.Names {
				if innerNames[name] {
					modNameMap[name] = innerPrefix + name
				}
			}
			// Alias imports: import Std:List as List
			// Map "List.map" → "Std$List$map" for DotAccess resolution
			if innerImp.Alias != "" {
				for name := range innerNames {
					qualName := innerImp.Alias + "." + name
					modNameMap[qualName] = innerPrefix + name
				}
			}
		}

		// Extract declarations, prefixing function names to avoid collisions.
		// Type declarations keep their original names (resolved by the type system).
		for _, me := range modExprs {
			switch d := me.(type) {
			case ast.TestDecl, ast.Export, ast.TypeAnnotation, ast.Import:
				continue
			case ast.TypeDecl:
				r.decls = append(r.decls, d)
			case ast.Let:
				if d.InExpr != nil {
					continue // skip expression-level let
				}
				d.Name = prefix + d.Name
				d.Body = renameRefs(d.Body, modNameMap)
				r.decls = append(r.decls, d)
			case ast.LetRec:
				for i, b := range d.Bindings {
					d.Bindings[i].Name = prefix + b.Name
					d.Bindings[i].Body = renameRefs(b.Body, modNameMap)
				}
				r.decls = append(r.decls, d)
			case ast.TraitDecl:
				r.decls = append(r.decls, d)
			case ast.ImplDecl:
				r.decls = append(r.decls, d)
			case ast.ExternalDecl:
				d.Name = prefix + d.Name
				r.decls = append(r.decls, d)
			}
		}

		// Set up aliases for names imported by the user (root-level imports).
		// Only alias names that are actually defined in the module source
		// (topNames). Re-exported builtins (e.g., println from IO.rex) keep
		// their original names so codegen can handle them as builtins.
		if isRoot {
			if imp.Alias != "" {
				// Qualified import: import Foo as F → F.bar → Foo__bar
				for name := range topNames {
					r.aliases[imp.Alias+"."+name] = prefix + name
				}
			}
			for _, name := range imp.Names {
				if topNames[name] {
					r.aliases[name] = prefix + name
				}
			}
		}

		r.stack = r.stack[:len(r.stack)-1]
	}
	return nil
}

// ApplyAliases renames imported names in user expressions.
func ApplyAliases(exprs []ast.Expr, aliases map[string]string) []ast.Expr {
	if len(aliases) == 0 {
		return exprs
	}
	result := make([]ast.Expr, len(exprs))
	for i, e := range exprs {
		result[i] = renameRefs(e, aliases)
	}
	return result
}

// renameRefs renames variable references using a name mapping.
func renameRefs(expr ast.Expr, nameMap map[string]string) ast.Expr {
	switch e := expr.(type) {
	case ast.Var:
		if newName, ok := nameMap[e.Name]; ok {
			e.Name = newName
			return e
		}
	case ast.App:
		e.Func = renameRefs(e.Func, nameMap)
		e.Arg = renameRefs(e.Arg, nameMap)
		return e
	case ast.Fun:
		e.Body = renameRefs(e.Body, nameMap)
		return e
	case ast.Let:
		e.Body = renameRefs(e.Body, nameMap)
		if e.InExpr != nil {
			e.InExpr = renameRefs(e.InExpr, nameMap)
		}
		return e
	case ast.LetRec:
		for i, b := range e.Bindings {
			e.Bindings[i].Body = renameRefs(b.Body, nameMap)
		}
		if e.InExpr != nil {
			e.InExpr = renameRefs(e.InExpr, nameMap)
		}
		return e
	case ast.If:
		e.Cond = renameRefs(e.Cond, nameMap)
		e.ThenExpr = renameRefs(e.ThenExpr, nameMap)
		e.ElseExpr = renameRefs(e.ElseExpr, nameMap)
		return e
	case ast.Binop:
		e.Left = renameRefs(e.Left, nameMap)
		e.Right = renameRefs(e.Right, nameMap)
		return e
	case ast.Match:
		e.Scrutinee = renameRefs(e.Scrutinee, nameMap)
		for i, arm := range e.Arms {
			e.Arms[i].Body = renameRefs(arm.Body, nameMap)
		}
		return e
	case ast.ListLit:
		for i, item := range e.Items {
			e.Items[i] = renameRefs(item, nameMap)
		}
		return e
	case ast.TupleLit:
		for i, item := range e.Items {
			e.Items[i] = renameRefs(item, nameMap)
		}
		return e
	case ast.UnaryMinus:
		e.Expr = renameRefs(e.Expr, nameMap)
		return e
	case ast.StringInterp:
		for i, part := range e.Parts {
			e.Parts[i] = renameRefs(part, nameMap)
		}
		return e
	case ast.TaggedTemplate:
		if mapped, ok := nameMap[e.Tag]; ok {
			e.Tag = mapped
		}
		for i, val := range e.Values {
			e.Values[i] = renameRefs(val, nameMap)
		}
		return e
	case ast.RecordCreate:
		for i, f := range e.Fields {
			e.Fields[i].Value = renameRefs(f.Value, nameMap)
		}
		return e
	case ast.DotAccess:
		qualName := e.ModuleName + "." + e.FieldName
		if newName, ok := nameMap[qualName]; ok {
			return ast.Var{Name: newName}
		}
		return e
	case ast.FieldAccess:
		e.Record = renameRefs(e.Record, nameMap)
		return e
	case ast.RecordUpdate:
		e.Record = renameRefs(e.Record, nameMap)
		for i, u := range e.Updates {
			e.Updates[i].Value = renameRefs(u.Value, nameMap)
		}
		return e
	case ast.Assert:
		e.Expr = renameRefs(e.Expr, nameMap)
		return e
	case ast.LetPat:
		e.Body = renameRefs(e.Body, nameMap)
		if e.InExpr != nil {
			e.InExpr = renameRefs(e.InExpr, nameMap)
		}
		return e
	case ast.TestDecl:
		for i, bodyExpr := range e.Body {
			e.Body[i] = renameRefs(bodyExpr, nameMap)
		}
		return e
	}
	return expr
}

func (r *resolver) loadSource(module string) (string, error) {
	if strings.Contains(module, ":") {
		parts := strings.SplitN(module, ":", 2)
		namespace, name := parts[0], parts[1]
		if namespace == "Std" {
			return stdlib.SourceForTarget(name, r.target)
		}
		// Package import
		pkgSrc, ok := r.packageRoots[namespace]
		if !ok {
			return "", fmt.Errorf("unknown package '%s' in '%s' (not in rex.toml?)", namespace, namespace+":"+name)
		}
		modPath := strings.ReplaceAll(name, ".", "/")
		basePath := pkgSrc + "/" + modPath + ".rex"
		if r.target != "" {
			overlayPath := pkgSrc + "/" + modPath + "." + r.target + ".rex"
			baseData, baseErr := os.ReadFile(basePath)
			overlayData, overlayErr := os.ReadFile(overlayPath)
			if baseErr != nil && overlayErr != nil {
				return "", baseErr
			}
			if baseErr != nil {
				return string(overlayData), nil
			}
			if overlayErr != nil {
				return string(baseData), nil
			}
			return string(baseData) + "\n" + string(overlayData), nil
		}
		data, err := os.ReadFile(basePath)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	// User module — resolve from srcRoot
	if r.srcRoot == "" {
		return "", fmt.Errorf("user module import '%s' requires src/ directory", module)
	}
	modPath := strings.ReplaceAll(module, ".", "/")
	basePath := r.srcRoot + "/" + modPath + ".rex"

	// Try target-specific overlay for user modules
	if r.target != "" {
		overlayPath := r.srcRoot + "/" + modPath + "." + r.target + ".rex"
		baseData, baseErr := os.ReadFile(basePath)
		overlayData, overlayErr := os.ReadFile(overlayPath)

		if baseErr != nil && overlayErr != nil {
			return "", baseErr
		}
		if baseErr != nil {
			return string(overlayData), nil
		}
		if overlayErr != nil {
			return string(baseData), nil
		}
		return string(baseData) + "\n" + string(overlayData), nil
	}

	data, err := os.ReadFile(basePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// loadCompanionJS scans module source for external declarations and looks for
// per-function companion JS files: ModuleName.functionName.js
// Each file's contents are wrapped in an IIFE and assigned to the mangled name.
func (r *resolver) loadCompanionJS(module string, modExprs []ast.Expr) {
	// Determine the source directory and module file prefix
	var srcDir, modFilePrefix, prefix string

	if strings.Contains(module, ":") {
		parts := strings.SplitN(module, ":", 2)
		namespace, name := parts[0], parts[1]
		if namespace == "Std" {
			return // stdlib builtins handled by codegen preamble
		}
		pkgSrc, ok := r.packageRoots[namespace]
		if !ok {
			return
		}
		srcDir = pkgSrc
		modFilePrefix = strings.ReplaceAll(name, ".", "/")
		prefix = ModulePrefix(module)
	} else {
		if r.srcRoot == "" {
			return
		}
		srcDir = r.srcRoot
		modFilePrefix = strings.ReplaceAll(module, ".", "/")
		prefix = ModulePrefix(module)
	}

	// Scan for external declarations
	for _, expr := range modExprs {
		ext, ok := expr.(ast.ExternalDecl)
		if !ok {
			continue
		}
		// Look for ModuleName.functionName.js
		jsPath := srcDir + "/" + modFilePrefix + "." + ext.Name + ".js"
		data, err := os.ReadFile(jsPath)
		if err != nil {
			continue // no companion file — might be a builtin handled by codegen
		}
		mangledName := prefix + ext.Name
		r.jsBindings = append(r.jsBindings, JsBinding{
			MangledName: mangledName,
			Source:      string(data),
		})
	}
}
