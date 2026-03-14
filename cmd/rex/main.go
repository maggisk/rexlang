// Command rex is the RexLang interpreter and REPL.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/chzyer/readline"
	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/codegen"
	"github.com/maggisk/rexlang/internal/eval"
	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/lexer"
	"github.com/maggisk/rexlang/internal/manifest"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/stdlib"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// safeMode is set by the --safe flag; it promotes warnings (todo usage) to errors.
var safeMode bool

// targetMode is set by the --target flag; defaults to "native".
var targetMode = "native"

// moduleMode is set by the --module flag; defaults to "global:Rex".
var moduleMode = "global:Rex"

// packageRoots maps package names to their src/ directories (from rex.toml).
var packageRoots map[string]string

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		repl()
		return
	}

	// Strip global flags before dispatching.
	var filtered []string
	for _, a := range args {
		if a == "--safe" {
			safeMode = true
		} else if strings.HasPrefix(a, "--target=") {
			targetMode = strings.TrimPrefix(a, "--target=")
		} else if strings.HasPrefix(a, "--module=") {
			moduleMode = strings.TrimPrefix(a, "--module=")
		} else {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	if len(args) == 0 {
		repl()
		return
	}
	if args[0] == "init" {
		runInit()
		return
	}
	if args[0] == "install" {
		runInstall(args[1:])
		return
	}
	if args[0] == "--compile" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --compile <file.rex>")
			os.Exit(1)
		}
		if targetMode == "browser" {
			compileJSFile(args[1])
		} else {
			compileFile(args[1])
		}
		return
	}
	if args[0] == "--compile-go" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --compile-go <file.rex>")
			os.Exit(1)
		}
		compileGoFile(args[1])
		return
	}
	if args[0] == "--types" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --types <file.rex | Std:Module>")
			os.Exit(1)
		}
		showTypes(args[1])
		return
	}
	if args[0] == "--test" {
		var only string
		var files []string
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "--only=") {
				only = strings.TrimPrefix(a, "--only=")
			} else {
				files = append(files, a)
			}
		}
		if len(files) == 0 {
			fmt.Fprintln(os.Stderr, "Usage: rex --test [--safe] [--only=<pattern>] <file.rex> [file.rex ...]")
			os.Exit(1)
		}
		totalFailed := 0
		var allFailed []struct {
			path string
			t    eval.FailedTest
		}
		for _, path := range files {
			n, tests := runTests(path, only)
			totalFailed += n
			for _, t := range tests {
				allFailed = append(allFailed, struct {
					path string
					t    eval.FailedTest
				}{path, t})
			}
		}
		if len(allFailed) > 0 {
			fmt.Printf("\nFailures:\n")
			for _, f := range allFailed {
				fmt.Printf("  %s  (%s:%d)\n", f.t.Name, f.path, f.t.Line)
			}
		}
		if totalFailed > 0 {
			os.Exit(1)
		}
		return
	}
	runFile(args[0], args[1:])
}

// ---------------------------------------------------------------------------
// Color helpers (TTY-aware)
// ---------------------------------------------------------------------------

func stderrIsTTY() bool {
	fi, err := os.Stderr.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

var (
	colorRed    string
	colorYellow string
	colorReset  string
)

func initColors() {
	if stderrIsTTY() {
		colorRed = "\033[31m"
		colorYellow = "\033[33m"
		colorReset = "\033[0m"
	}
}

func init() {
	initColors()
}

// ---------------------------------------------------------------------------
// Warning / error printing
// ---------------------------------------------------------------------------

func printErr(kind string, err error) {
	fmt.Fprintf(os.Stderr, "%s%s%s: %v\n", colorRed, kind, colorReset, err)
}

func printTestErr(path, kind string, err error) {
	fmt.Println() // blank line to separate from any preceding test output
	fmt.Fprintf(os.Stderr, "%s%s: %s%s: %v\n", colorRed, path, kind, colorReset, err)
}

func printWarnings(path string, warnings []typechecker.Warning) {
	for _, w := range warnings {
		if w.Line > 0 {
			fmt.Fprintf(os.Stderr, "%sWarning%s: %s:%d: %s\n", colorYellow, colorReset, path, w.Line, w.Msg)
		} else {
			fmt.Fprintf(os.Stderr, "%sWarning%s: %s: %s\n", colorYellow, colorReset, path, w.Msg)
		}
	}
}

// handleWarnings prints warnings and, if --safe is active, exits with an error.
func handleWarnings(path string, warnings []typechecker.Warning) {
	if len(warnings) == 0 {
		return
	}
	printWarnings(path, warnings)
	if safeMode {
		fmt.Fprintf(os.Stderr, "%s--safe: warnings are errors%s\n", colorRed, colorReset)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// setupSrcRoot
// ---------------------------------------------------------------------------

// setupSrcRoot detects a src/ directory in cwd, validates the entry file if needed,
// and sets the srcRoot for both typechecker and eval. Also detects rex.toml and
// populates package roots.
func setupSrcRoot(entryFile string) {
	// Set target for typechecker and eval
	typechecker.SetTarget(targetMode)
	eval.SetTarget(targetMode)

	cwd, err := os.Getwd()
	if err != nil {
		return
	}

	// Detect rex.toml and set up package roots
	projectRoot := manifest.FindProjectRoot(cwd)
	if projectRoot != "" {
		_, deps, err := manifest.Load(projectRoot)
		if err == nil {
			roots, err := manifest.PackageRoots(projectRoot, deps)
			if err == nil {
				packageRoots = roots
				typechecker.SetPackageRoots(roots)
				eval.SetPackageRoots(roots)
			}
		}
	}

	srcDir := filepath.Join(cwd, "src")
	info, err := os.Stat(srcDir)
	if err != nil || !info.IsDir() {
		// No src/ directory — user module imports will error at load time
		return
	}
	absEntry, err := filepath.Abs(entryFile)
	if err != nil {
		return
	}
	absSrc, _ := filepath.Abs(srcDir)
	if !strings.HasPrefix(absEntry, absSrc+string(filepath.Separator)) {
		// Entry file is NOT under src/ — don't set srcRoot.
		// User module imports will error at load time.
		return
	}
	typechecker.SetSrcRoot(absSrc)
	eval.SetSrcRoot(absSrc)
}

// ---------------------------------------------------------------------------
// rex install
// ---------------------------------------------------------------------------

func runInit() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	manifestPath := filepath.Join(cwd, "rex.toml")
	if _, err := os.Stat(manifestPath); err == nil {
		fmt.Fprintln(os.Stderr, "rex.toml already exists")
		os.Exit(1)
	}
	name := filepath.Base(cwd)
	content := fmt.Sprintf("[package]\nname = %q\nversion = \"0.1.0\"\n", name)
	if err := os.WriteFile(manifestPath, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Created rex.toml (package: %s)\n", name)
}

func runInstall(args []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(args) >= 1 {
		target := args[0]
		if isLocalPath(target) {
			// rex install <path> — add a local path dependency
			addPathDep(cwd, target)
		} else if len(args) >= 2 {
			// rex install <url> <ref> — add a git dependency
			addAndInstallDep(cwd, target, args[1])
		} else {
			fmt.Fprintln(os.Stderr, "Usage: rex install <path>")
			fmt.Fprintln(os.Stderr, "       rex install <git-url> <ref>")
			fmt.Fprintln(os.Stderr, "       rex install")
			os.Exit(1)
		}
		return
	}

	// rex install — fetch all deps from rex.toml
	installAllDeps(cwd)
}

func isLocalPath(s string) bool {
	return strings.HasPrefix(s, ".") || strings.HasPrefix(s, "/") || strings.Contains(s, string(filepath.Separator))
}

func addPathDep(projectRoot, path string) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	// Check it exists and has src/
	srcDir := filepath.Join(absPath, "src")
	if _, err := os.Stat(srcDir); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s has no src/ directory\n", absPath)
		os.Exit(1)
	}

	// Read package name from its rex.toml
	pkgName := ""
	if data, err := os.ReadFile(filepath.Join(absPath, "rex.toml")); err == nil {
		var pm manifest.Manifest
		if err := toml.Unmarshal(data, &pm); err == nil && pm.Package.Name != "" {
			pkgName = pm.Package.Name
		}
	}
	if pkgName == "" {
		pkgName = filepath.Base(absPath)
	}

	// Make path relative to project root if possible
	relPath, err := filepath.Rel(projectRoot, absPath)
	if err != nil {
		relPath = absPath
	}

	// Read or create rex.toml
	manifestPath := filepath.Join(projectRoot, "rex.toml")
	var m manifest.Manifest
	if data, err := os.ReadFile(manifestPath); err == nil {
		toml.Unmarshal(data, &m)
	}
	if m.Deps == nil {
		m.Deps = make(map[string]manifest.Dependency)
	}
	m.Deps[pkgName] = manifest.Dependency{Path: relPath}

	f, err := os.Create(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing rex.toml: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(m); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding rex.toml: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added %s = { path = \"%s\" }\n", pkgName, relPath)
}

func installAllDeps(projectRoot string) {
	_, deps, err := manifest.Load(projectRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(deps) == 0 {
		fmt.Println("No dependencies to install.")
		return
	}

	for _, dep := range deps {
		if dep.Path != "" {
			fmt.Printf("  %s → %s (local)\n", dep.Name, dep.Path)
			continue
		}
		installGitDep(projectRoot, dep.Name, dep.Git, dep.Ref)
	}
}

func installGitDep(projectRoot, name, gitURL, ref string) {
	destDir := filepath.Join(projectRoot, "rex_modules", name)

	// Check if already installed at correct ref
	if info, err := os.Stat(destDir); err == nil && info.IsDir() {
		// Check the current ref
		cmd := exec.Command("git", "-C", destDir, "describe", "--tags", "--exact-match", "HEAD")
		if out, err := cmd.Output(); err == nil && strings.TrimSpace(string(out)) == ref {
			fmt.Printf("  %s@%s (up to date)\n", name, ref)
			return
		}
		// Check if it's a commit SHA
		cmd = exec.Command("git", "-C", destDir, "rev-parse", "HEAD")
		if out, err := cmd.Output(); err == nil && strings.HasPrefix(strings.TrimSpace(string(out)), ref) {
			fmt.Printf("  %s@%s (up to date)\n", name, ref)
			return
		}
		// Different version — remove and re-clone
		os.RemoveAll(destDir)
	}

	fmt.Printf("  %s@%s ... ", name, ref)

	// Clone at specific ref
	if err := os.MkdirAll(filepath.Dir(destDir), 0755); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
	cmd := exec.Command("git", "clone", "--depth=1", "--branch", ref, gitURL, destDir)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try as a commit SHA — need full clone then checkout
		cmd = exec.Command("git", "clone", gitURL, destDir)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if out2, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("error: git clone failed\n%s\n%s\n", out, out2)
			os.Exit(1)
		}
		cmd = exec.Command("git", "-C", destDir, "checkout", ref)
		if out, err := cmd.CombinedOutput(); err != nil {
			os.RemoveAll(destDir)
			fmt.Printf("error: git checkout %s failed\n%s\n", ref, out)
			os.Exit(1)
		}
	}
	fmt.Println("ok")
}

func addAndInstallDep(projectRoot, gitURL, ref string) {
	manifestPath := filepath.Join(projectRoot, "rex.toml")

	// Clone to a temp dir to read the package's rex.toml for its name
	tmpDir, err := os.MkdirTemp("", "rex-install-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "clone", "--depth=1", "--branch", ref, gitURL, tmpDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		// Try as commit SHA
		cmd = exec.Command("git", "clone", gitURL, tmpDir)
		os.RemoveAll(tmpDir)
		os.MkdirAll(tmpDir, 0755)
		if out2, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "Error cloning: %s\n%s\n", out, out2)
			os.Exit(1)
		}
		cmd = exec.Command("git", "-C", tmpDir, "checkout", ref)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "Error checking out %s: %s\n", ref, out)
			os.Exit(1)
		}
	}

	// Read the package's name from its rex.toml
	pkgName := ""
	pkgManifestPath := filepath.Join(tmpDir, "rex.toml")
	if data, err := os.ReadFile(pkgManifestPath); err == nil {
		var pm manifest.Manifest
		if err := toml.Unmarshal(data, &pm); err == nil && pm.Package.Name != "" {
			pkgName = pm.Package.Name
		}
	}
	if pkgName == "" {
		// Fall back to deriving from URL
		base := filepath.Base(gitURL)
		pkgName = strings.TrimSuffix(base, ".git")
		pkgName = strings.TrimPrefix(pkgName, "rex-")
	}

	// Read or create rex.toml
	var m manifest.Manifest
	if data, err := os.ReadFile(manifestPath); err == nil {
		toml.Unmarshal(data, &m)
	}
	if m.Deps == nil {
		m.Deps = make(map[string]manifest.Dependency)
	}
	m.Deps[pkgName] = manifest.Dependency{Git: gitURL, Ref: ref}

	// Write rex.toml
	f, err := os.Create(manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing rex.toml: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	enc := toml.NewEncoder(f)
	if err := enc.Encode(m); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding rex.toml: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Added %s = { git = \"%s\", ref = \"%s\" }\n", pkgName, gitURL, ref)

	// Now install it
	installGitDep(projectRoot, pkgName, gitURL, ref)
}

// ---------------------------------------------------------------------------
// compileFile
// ---------------------------------------------------------------------------

func compileFile(path string) {
	setupSrcRoot(path)

	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse
	exprs, err := parser.Parse(string(source))
	if err != nil {
		printErr("Parse error", err)
		os.Exit(1)
	}

	if err := eval.ValidateToplevel(exprs); err != nil {
		printErr("Syntax error", err)
		os.Exit(1)
	}

	if err := parser.ValidateIndentation(exprs); err != nil {
		printErr("Indentation error", err)
		os.Exit(1)
	}

	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	typeEnv, warnings, err := typechecker.CheckProgram(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(path, warnings)

	// Resolve module imports: collect type/trait/impl/function declarations
	// from imported modules so codegen has full program visibility.
	importInfo, err := ir.ResolveImports(exprs, typechecker.GetSrcRoot(), targetMode, packageRoots)
	if err != nil {
		printErr("Import resolution error", err)
		os.Exit(1)
	}
	userExprs := ir.ApplyAliases(exprs, importInfo.Aliases)
	allExprs := append(importInfo.Decls, userExprs...)

	// Lower to IR
	l := ir.NewLowerer()
	prog, err := l.LowerProgram(allExprs)
	if err != nil {
		printErr("IR error", err)
		os.Exit(1)
	}

	// Tree shake: remove functions not reachable from main
	prog = ir.Shake(prog)

	// Emit WAT
	wat, err := codegen.EmitWAT(prog, typeEnv)
	if err != nil {
		printErr("Codegen error", err)
		os.Exit(1)
	}

	// Determine output paths
	base := strings.TrimSuffix(filepath.Base(path), ".rex")
	watPath := base + ".wat"
	wasmPath := base + ".wasm"

	if err := os.WriteFile(watPath, []byte(wat), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing WAT: %v\n", err)
		os.Exit(1)
	}

	// Assemble with wasm-tools
	out, err := exec.Command("wasm-tools", "parse", watPath, "-o", wasmPath).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wasm-tools failed: %v\n%s", err, out)
		os.Exit(1)
	}

	fmt.Printf("Compiled %s → %s (%s)\n", path, wasmPath, watPath)
}

// ---------------------------------------------------------------------------
// compileGoFile
// ---------------------------------------------------------------------------

func compileGoFile(path string) {
	setupSrcRoot(path)

	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse
	exprs, err := parser.Parse(string(source))
	if err != nil {
		printErr("Parse error", err)
		os.Exit(1)
	}

	if err := eval.ValidateToplevel(exprs); err != nil {
		printErr("Syntax error", err)
		os.Exit(1)
	}

	if err := parser.ValidateIndentation(exprs); err != nil {
		printErr("Indentation error", err)
		os.Exit(1)
	}

	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	typeEnv, warnings, err := typechecker.CheckProgram(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(path, warnings)

	// Resolve module imports
	importInfo, err := ir.ResolveImports(exprs, typechecker.GetSrcRoot(), targetMode, packageRoots)
	if err != nil {
		printErr("Import resolution error", err)
		os.Exit(1)
	}
	userExprs := ir.ApplyAliases(exprs, importInfo.Aliases)
	allExprs := append(importInfo.Decls, userExprs...)

	// Lower to IR
	l := ir.NewLowerer()
	prog, err := l.LowerProgram(allExprs)
	if err != nil {
		printErr("IR error", err)
		os.Exit(1)
	}

	// Tree shake
	prog = ir.Shake(prog)

	// Emit Go source
	goSrc, err := codegen.EmitGo(prog, typeEnv)
	if err != nil {
		printErr("Codegen error", err)
		os.Exit(1)
	}

	// Determine output paths
	base := strings.TrimSuffix(filepath.Base(path), ".rex")
	goDir := base + "_go"
	goFile := filepath.Join(goDir, "main.go")
	binaryPath := base

	// Create output directory
	if err := os.MkdirAll(goDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(goFile, []byte(goSrc), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing Go source: %v\n", err)
		os.Exit(1)
	}

	// Create a go.mod for the generated code
	goMod := fmt.Sprintf("module rex_%s\n\ngo 1.24\n", base)
	goModFile := filepath.Join(goDir, "go.mod")
	if err := os.WriteFile(goModFile, []byte(goMod), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing go.mod: %v\n", err)
		os.Exit(1)
	}

	// Build with go build
	absOutput, _ := filepath.Abs(binaryPath)
	cmd := exec.Command("go", "build", "-o", absOutput, ".")
	cmd.Dir = goDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "go build failed: %v\n%s\nGenerated Go source: %s\n", err, out, goFile)
		os.Exit(1)
	}

	fmt.Printf("Compiled %s → %s (%s)\n", path, binaryPath, goFile)
}

// ---------------------------------------------------------------------------
// compileJSFile
// ---------------------------------------------------------------------------

func compileJSFile(path string) {
	setupSrcRoot(path)

	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	exprs, err := parser.Parse(string(source))
	if err != nil {
		printErr("Parse error", err)
		os.Exit(1)
	}

	if err := eval.ValidateToplevel(exprs); err != nil {
		printErr("Syntax error", err)
		os.Exit(1)
	}

	if err := parser.ValidateIndentation(exprs); err != nil {
		printErr("Indentation error", err)
		os.Exit(1)
	}

	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	typeEnv, warnings, err := typechecker.CheckProgram(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(path, warnings)

	importInfo, err := ir.ResolveImports(exprs, typechecker.GetSrcRoot(), targetMode, packageRoots)
	if err != nil {
		printErr("Import resolution error", err)
		os.Exit(1)
	}
	userExprs := ir.ApplyAliases(exprs, importInfo.Aliases)
	allExprs := append(importInfo.Decls, userExprs...)

	l := ir.NewLowerer()
	prog, err := l.LowerProgram(allExprs)
	if err != nil {
		printErr("IR error", err)
		os.Exit(1)
	}

	prog = ir.Shake(prog)

	jsSrc, err := codegen.EmitJS(prog, typeEnv, importInfo.JsBindings, moduleMode)
	if err != nil {
		printErr("Codegen error", err)
		os.Exit(1)
	}

	base := strings.TrimSuffix(filepath.Base(path), ".rex")
	jsFile := base + ".js"
	htmlFile := base + ".html"

	if err := os.WriteFile(jsFile, []byte(jsSrc), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing JS file: %v\n", err)
		os.Exit(1)
	}

	htmlSrc := codegen.EmitBrowserHTML(jsFile)
	if err := os.WriteFile(htmlFile, []byte(htmlSrc), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing HTML file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Compiled %s → %s + %s\n", path, jsFile, htmlFile)
}

// ---------------------------------------------------------------------------
// runFile
// ---------------------------------------------------------------------------

func runFile(path string, programArgs []string) {
	setupSrcRoot(path)

	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Parse
	exprs, err := parser.Parse(string(source))
	if err != nil {
		printErr("Lexer/Parse error", err)
		os.Exit(1)
	}

	// Validate no bare expressions at top level
	if err := eval.ValidateToplevel(exprs); err != nil {
		printErr("Syntax error", err)
		os.Exit(1)
	}

	// Validate indentation
	if err := parser.ValidateIndentation(exprs); err != nil {
		printErr("Indentation error", err)
		os.Exit(1)
	}

	// Reorder top-level bindings by dependency
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	// Type check
	typeEnv, warnings, err := typechecker.CheckProgram(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(path, warnings)

	// Validate main exists and unifies with List String -> Int
	mainScheme, ok := typeEnv["main"]
	if !ok {
		printErr("Type error", fmt.Errorf("no main function — add 'export let main args = ...'"))
		os.Exit(1)
	}
	scheme, ok := mainScheme.(types.Scheme)
	if !ok {
		printErr("Type error", fmt.Errorf("main must be a function"))
		os.Exit(1)
	}
	mainTy := typechecker.Instantiate(scheme)
	expectedTy := types.TFun(types.TList(types.TString), types.TInt)
	if _, err := types.Unify(mainTy, expectedTy); err != nil {
		printErr("Type error", fmt.Errorf("main must have type List String -> Int, got %s", types.TypeToString(scheme.Ty)))
		os.Exit(1)
	}

	// Evaluate
	result, err := eval.RunProgram(exprs, programArgs)
	if err != nil {
		printErr("Runtime error", err)
		os.Exit(1)
	}
	exitCode := result.(eval.VInt).V
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

// ---------------------------------------------------------------------------
// runTests
// ---------------------------------------------------------------------------

func runTests(path string, only string) (int, []eval.FailedTest) {
	setupSrcRoot(path)

	source, err := os.ReadFile(path)
	if err != nil {
		printTestErr(path, "error", err)
		return 1, nil
	}
	src := string(source)

	// Detect if this is a stdlib file and inject extra builtins/type-env
	var extraBuiltins map[string]eval.Value
	var extraTypeEnv typechecker.TypeEnv

	absPath, _ := filepath.Abs(path)
	moduleName := stdlibModuleForPath(absPath)
	if moduleName != "" {
		extraBuiltins = eval.BuiltinsForModule(moduleName, nil)
		extraTypeEnv = typechecker.TypeEnvForModule(moduleName)
	}

	// Parse
	exprs, err := parser.Parse(src)
	if err != nil {
		printTestErr(path, "Parse error", err)
		return 1, nil
	}

	// Validate no bare expressions at top level
	if err := eval.ValidateToplevel(exprs); err != nil {
		printTestErr(path, "Syntax error", err)
		return 1, nil
	}

	// Validate indentation
	if err := parser.ValidateIndentation(exprs); err != nil {
		printTestErr(path, "Indentation error", err)
		return 1, nil
	}

	// Reorder top-level bindings by dependency
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printTestErr(path, "Type error", err)
		return 1, nil
	}

	// Type check with optional extra env
	var warnings []typechecker.Warning
	if extraTypeEnv != nil {
		if _, warnings, err = typechecker.CheckProgramWithExtraEnv(exprs, extraTypeEnv); err != nil {
			printTestErr(path, "Type error", err)
			return 1, nil
		}
	} else {
		if _, warnings, err = typechecker.CheckProgram(exprs); err != nil {
			printTestErr(path, "Type error", err)
			return 1, nil
		}
	}
	handleWarnings(path, warnings)

	_, failed, failedNames, err := eval.RunTests(exprs, nil, extraBuiltins, path, only)
	if err != nil {
		printTestErr(path, "error", err)
		return 1, nil
	}
	return failed, failedNames
}

// ---------------------------------------------------------------------------
// showTypes
// ---------------------------------------------------------------------------

// showTypes prints the types of all top-level bindings in a file or stdlib module.
func showTypes(target string) {
	// Check if target is a stdlib module (starts with "Std:")
	if strings.HasPrefix(target, "Std:") {
		result, err := typechecker.CheckModule(target)
		if err != nil {
			printErr("Type error", err)
			os.Exit(1)
		}
		printTypeEnv(result.Env)
		return
	}

	// Otherwise treat as a .rex file
	setupSrcRoot(target)

	source, err := os.ReadFile(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	exprs, err := parser.Parse(string(source))
	if err != nil {
		printErr("Parse error", err)
		os.Exit(1)
	}

	if err := eval.ValidateToplevel(exprs); err != nil {
		printErr("Syntax error", err)
		os.Exit(1)
	}

	if err := parser.ValidateIndentation(exprs); err != nil {
		printErr("Indentation error", err)
		os.Exit(1)
	}

	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	typeEnv, warnings, err := typechecker.CheckProgram(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(target, warnings)

	// Collect top-level names from the AST (in order)
	names := collectToplevelNames(exprs)

	for _, name := range names {
		v, ok := typeEnv[name]
		if !ok {
			continue
		}
		scheme, ok := v.(types.Scheme)
		if !ok {
			continue
		}
		fmt.Printf("%s : %s\n", name, types.SchemeToString(scheme))
	}
}

// collectToplevelNames returns the names of all top-level bindings in AST order.
func collectToplevelNames(exprs []ast.Expr) []string {
	seen := map[string]bool{}
	var names []string
	add := func(name string) {
		if !seen[name] && !strings.HasPrefix(name, "__") && name != "_" {
			seen[name] = true
			names = append(names, name)
		}
	}
	for _, expr := range exprs {
		switch e := expr.(type) {
		case ast.Let:
			if e.InExpr == nil {
				add(e.Name)
			}
		case ast.LetRec:
			if e.InExpr == nil {
				for _, b := range e.Bindings {
					add(b.Name)
				}
			}
		case ast.TypeDecl:
			for _, ctor := range e.Ctors {
				add(ctor.Name)
			}
			if len(e.RecordFields) > 0 {
				add(e.Name)
			}
		}
	}
	return names
}

// printTypeEnv prints a TypeEnv sorted alphabetically.
func printTypeEnv(env typechecker.TypeEnv) {
	var names []string
	for name := range env {
		if !strings.HasPrefix(name, "__") {
			names = append(names, name)
		}
	}
	sortStrings(names)
	for _, name := range names {
		scheme, ok := env[name].(types.Scheme)
		if !ok {
			continue
		}
		fmt.Printf("%s : %s\n", name, types.SchemeToString(scheme))
	}
}

// sortStrings sorts a string slice in place.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// stdlibModuleForPath detects if path is inside the embedded stdlib.
// Returns the module name (e.g. "List") or empty string.
func stdlibModuleForPath(absPath string) string {
	if !strings.HasSuffix(absPath, ".rex") {
		return ""
	}
	// Try the base name first (e.g., "List" from "List.rex")
	base := strings.TrimSuffix(filepath.Base(absPath), ".rex")
	if _, err := stdlib.Source(base); err == nil {
		return base
	}
	// Try parent.base for subdirectory modules (e.g., "Http.Server" from "Http/Server.rex")
	dir := filepath.Base(filepath.Dir(absPath))
	dotted := dir + "." + base
	if _, err := stdlib.Source(dotted); err == nil {
		return dotted
	}
	return ""
}

// ---------------------------------------------------------------------------
// REPL
// ---------------------------------------------------------------------------

func replHistoryPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".rexlang_history")
}

func repl() {
	fmt.Println("RexLang v0.1.0 (Go)")
	fmt.Println("Press Enter on a blank line to evaluate. Ctrl-D to exit.")
	fmt.Println()

	preludeTC, err := typechecker.LoadPreludeTC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load prelude: %v\n", err)
		os.Exit(1)
	}
	evalEnv, err := eval.LoadPreludeForREPL(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load prelude: %v\n", err)
		os.Exit(1)
	}
	typeEnv := preludeTC.Env.Clone()
	typeDefs := typechecker.CopyTypeDefs(preludeTC.TypeDefs)
	tc := typechecker.NewTypeChecker()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "rex> ",
		HistoryFile:     replHistoryPath(),
		InterruptPrompt: "^C",
		EOFPrompt:       "",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize readline: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

	var buf []string

	for {
		if len(buf) > 0 {
			rl.SetPrompt("  .. ")
		} else {
			rl.SetPrompt("rex> ")
		}

		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(buf) > 0 {
				buf = buf[:0]
				continue
			}
			break
		}
		if err == io.EOF {
			fmt.Println()
			break
		}
		if err != nil {
			break
		}

		if strings.TrimSpace(line) != "" {
			buf = append(buf, line)
			continue
		}
		if len(buf) == 0 {
			continue
		}

		source := strings.Join(buf, "\n") + "\n"
		buf = buf[:0]

		exprs, err := parser.Parse(source)
		if err != nil {
			fmt.Printf("Parse error: %v\n", err)
			continue
		}

		for _, expr := range exprs {
			res, err := tc.InferToplevel(typeEnv, typeDefs, types.Subst{}, expr)
			if err != nil {
				fmt.Printf("Type error: %v\n", err)
				break
			}
			typeEnv = res.Env
			typeDefs = res.TypeDefs

			// Print any warnings from the typechecker
			if len(tc.Warnings) > 0 {
				for _, w := range tc.Warnings {
					fmt.Fprintf(os.Stderr, "%sWarning%s: %s\n", colorYellow, colorReset, w.Msg)
				}
				tc.Warnings = tc.Warnings[:0]
			}

			val, newEnv, err := eval.EvalToplevel(evalEnv, expr, nil)
			if err != nil {
				fmt.Printf("Runtime error: %v\n", err)
				break
			}
			evalEnv = newEnv

			// Skip display for declarations
			switch expr.(type) {
			case ast.TypeDecl, ast.Import, ast.Export, ast.TraitDecl, ast.ImplDecl, ast.TestDecl, ast.TypeAnnotation:
				continue
			}

			var name string
			switch x := expr.(type) {
			case ast.Let:
				name = x.Name
			case ast.LetRec:
				name = x.Bindings[len(x.Bindings)-1].Name
			case ast.LetPat:
				name = "_"
			default:
				name = "it"
			}
			// Use scheme display if available (shows constraints)
			tyStr := types.TypeToString(res.Ty)
			lookupName := name
			if name == "it" {
				// For bare variable expressions, look up the scheme by var name
				if v, ok := expr.(ast.Var); ok {
					lookupName = v.Name
				}
			}
			if lookupName != "it" && lookupName != "_" {
				if s, ok := typeEnv[lookupName].(types.Scheme); ok {
					tyStr = types.SchemeToString(s)
				}
			}
			fmt.Printf("%s : %s\n", name, tyStr)
			fmt.Printf("=> %s\n", eval.ValueToString(val))
		}
	}
}

// Stubs to detect lexer error vs others
var _ = lexer.Tokenize
