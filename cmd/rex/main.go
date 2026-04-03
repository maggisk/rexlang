// Command rex is the RexLang compiler and runtime.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/callgraph"
	"github.com/maggisk/rexlang/internal/codegen"
	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/lsp"
	"github.com/maggisk/rexlang/internal/manifest"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/stdlib"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// version is set at build time via -ldflags "-X main.version=v1.2.3".
// Falls back to "dev" for development builds and go install without ldflags.
var version = "dev"

// safeMode is set by the --safe flag; it promotes warnings (todo usage) to errors.
var safeMode bool

// strictTodo is set automatically for build, test, and compile commands;
// todo usage becomes a compile error.
var strictTodo bool

// targetMode is set by the --target flag; defaults to "native".
var targetMode = "native"

// moduleMode is set by the --module flag; defaults to "global:Rex".
var moduleMode = "global:Rex"

// packageRoots maps package names to their src/ directories (from rex.toml).
var packageRoots map[string]string

// compileTiming tracks per-phase timing when --time is active.
var compileTiming = timing{}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: rex <file.rex> [args...]")
		fmt.Fprintln(os.Stderr, "       rex --test <file.rex> [file.rex ...]")
		fmt.Fprintln(os.Stderr, "       rex --types <file.rex>")
		fmt.Fprintln(os.Stderr, "       rex --graph <file.rex>")
		fmt.Fprintln(os.Stderr, "       rex build [--out=<path>] <file.rex>")
		fmt.Fprintln(os.Stderr, "       rex --compile-go <file.rex>")
		fmt.Fprintln(os.Stderr, "       rex --compile --target=browser <file.rex>")
		fmt.Fprintln(os.Stderr, "       rex init | install")
		os.Exit(1)
	}

	if args[0] == "--version" || args[0] == "-v" {
		fmt.Printf("rex %s\n", version)
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
		} else if a == "--time" {
			compileTiming.enabled = true
			compileTiming.start = time.Now()
		} else {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: rex <file.rex> [args...]")
		os.Exit(1)
	}
	if args[0] == "init" {
		runInit()
		return
	}
	if args[0] == "fmt" {
		runFmt(args[1:])
		return
	}
	if args[0] == "lsp" {
		lsp.Run()
		return
	}
	if args[0] == "check" {
		runCheck()
		return
	}
	if args[0] == "install" {
		runInstall(args[1:])
		return
	}
	if args[0] == "build" {
		var outPath string
		var buildArgs []string
		for _, a := range args[1:] {
			if strings.HasPrefix(a, "--out=") {
				outPath = strings.TrimPrefix(a, "--out=")
			} else {
				buildArgs = append(buildArgs, a)
			}
		}
		if len(buildArgs) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: rex build [--out=<path>] <file.rex>")
			os.Exit(1)
		}
		strictTodo = true
		buildBinary(buildArgs[0], outPath)
		return
	}
	if args[0] == "--compile" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --compile --target=browser <file.rex>")
			os.Exit(1)
		}
		if targetMode != "browser" {
			fmt.Fprintln(os.Stderr, "Usage: rex --compile --target=browser <file.rex>")
			os.Exit(1)
		}
		strictTodo = true
		compileJSFile(args[1])
		return
	}
	if args[0] == "--compile-go" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --compile-go <file.rex>")
			os.Exit(1)
		}
		strictTodo = true
		compileGoFile(args[1])
		return
	}
	if args[0] == "--types" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --types <file.rex>")
			os.Exit(1)
		}
		showTypes(args[1])
		return
	}
	if args[0] == "--graph" {
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --graph <file.rex>")
			os.Exit(1)
		}
		graphFile(args[1])
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
		strictTodo = true
		if len(files) == 1 {
			if !runTests(files[0], only) {
				os.Exit(1)
			}
		} else {
			if !runTestsBatch(files, only) {
				os.Exit(1)
			}
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
	if te, ok := err.(*types.TypeError); ok && te.Source != "" && te.Line > 0 {
		printRichTypeError(kind, te)
		return
	}
	fmt.Fprintf(os.Stderr, "%s%s%s: %v\n", colorRed, kind, colorReset, err)
}

// printErrWithSource annotates a TypeError with source text before printing.
func printErrWithSource(kind string, err error, source string) {
	if te, ok := err.(*types.TypeError); ok {
		te.Source = source
	}
	printErr(kind, err)
}

// printRichTypeError prints an Elm-style error with source snippet.
func printRichTypeError(kind string, te *types.TypeError) {
	// Build header with file path if available
	header := strings.ToUpper(kind)
	if te.File != "" {
		header += " ── " + te.File
	}
	fmt.Fprintf(os.Stderr, "\n%s-- %s %s\n\n", colorRed, header, colorReset)
	fmt.Fprintf(os.Stderr, "%s\n\n", te.Msg)
	// Show source line
	lines := strings.Split(te.Source, "\n")
	if te.Line > 0 && te.Line <= len(lines) {
		line := lines[te.Line-1]
		fmt.Fprintf(os.Stderr, "  %s%4d|%s %s\n", colorRed, te.Line, colorReset, line)
	}
	fmt.Fprintln(os.Stderr)
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

// handleWarnings prints warnings and exits with an error in strict modes.
// In build/test/compile mode (strictTodo) or with --safe, todo usage is fatal.
// In dev mode (rex <file>), warnings are printed but execution continues.
func handleWarnings(path string, warnings []typechecker.Warning) {
	if len(warnings) == 0 {
		return
	}
	printWarnings(path, warnings)
	if safeMode || strictTodo {
		if safeMode {
			fmt.Fprintf(os.Stderr, "%s--safe: warnings are errors%s\n", colorRed, colorReset)
		} else {
			fmt.Fprintf(os.Stderr, "%sError%s: todo must be resolved before building or testing\n", colorRed, colorReset)
		}
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// setupSrcRoot
// ---------------------------------------------------------------------------

// setupSrcRoot detects a src/ directory in cwd, validates the entry file if needed,
// and returns the srcRoot path. Also detects rex.toml and populates package roots.
func setupSrcRoot(entryFile string) string {
	typechecker.SetTarget(targetMode)

	cwd, err := os.Getwd()
	if err != nil {
		return ""
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
			}
		}
	}

	srcDir := filepath.Join(cwd, "src")
	info, err := os.Stat(srcDir)
	if err != nil || !info.IsDir() {
		return ""
	}
	absEntry, err := filepath.Abs(entryFile)
	if err != nil {
		return ""
	}
	absSrc, _ := filepath.Abs(srcDir)
	if !strings.HasPrefix(absEntry, absSrc+string(filepath.Separator)) {
		return ""
	}
	return absSrc
}

// ---------------------------------------------------------------------------
// rex check — verify library companions compile
// ---------------------------------------------------------------------------

func runCheck() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Must be in a library with rex.toml + [package] name
	m, deps, err := manifest.Load(cwd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	pkgName := m.Package.Name
	if pkgName == "" {
		fmt.Fprintln(os.Stderr, "Error: rex.toml must have [package] name for rex check")
		os.Exit(1)
	}

	srcDir := filepath.Join(cwd, "src")
	if info, err := os.Stat(srcDir); err != nil || !info.IsDir() {
		fmt.Fprintln(os.Stderr, "Error: no src/ directory found")
		os.Exit(1)
	}

	// Set up package roots for dependencies (so imports resolve)
	typechecker.SetTarget(targetMode)
	if len(deps) > 0 {
		roots, err := manifest.PackageRoots(cwd, deps)
		if err == nil {
			packageRoots = roots
			typechecker.SetPackageRoots(roots)
		}
	}

	// Find all .rex files in src/
	var rexFiles []string
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if !strings.HasSuffix(base, ".rex") {
			return nil
		}
		// Skip overlay files (.browser.rex, .native.rex)
		name := strings.TrimSuffix(base, ".rex")
		if strings.Contains(name, ".") {
			return nil
		}
		rexFiles = append(rexFiles, path)
		return nil
	})

	if len(rexFiles) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no .rex files found in src/")
		os.Exit(1)
	}

	// Step 1: Typecheck each .rex file and collect IR declarations per module
	type moduleInfo struct {
		name  string // module name (e.g. "Db")
		dir   string // directory containing the .rex file
		decls []ir.Decl
	}
	var modules []moduleInfo

	for _, path := range rexFiles {
		source, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			os.Exit(1)
		}

		exprs, err := parser.Parse(string(source))
		if err != nil {
			printErrWithSource("Parse error", err, string(source))
			os.Exit(1)
		}
		if err := parser.ValidateToplevel(exprs); err != nil {
			printErrWithSource("Syntax error", err, string(source))
			os.Exit(1)
		}
		if err := parser.ValidateIndentation(exprs); err != nil {
			printErrWithSource("Indentation error", err, string(source))
			os.Exit(1)
		}

		typeEnv, warnings, err := typechecker.CheckProgram(exprs, srcDir)
		if err != nil {
			if te, ok := err.(*types.TypeError); ok {
				te.Source = string(source)
				te.File = path
			}
			printErr("Type error", err)
			os.Exit(1)
		}
		_ = typeEnv
		handleWarnings(path, warnings)

		// Lower only user declarations to IR to get DType for this module.
		// We don't need imported types — only the module's own type declarations.
		l := ir.NewLowerer()
		prog, err := l.LowerProgram(exprs)
		if err != nil {
			printErr("IR error", err)
			os.Exit(1)
		}

		// Derive module name from file path relative to src/
		rel, _ := filepath.Rel(srcDir, path)
		modName := strings.TrimSuffix(rel, ".rex")
		modName = strings.ReplaceAll(modName, string(filepath.Separator), ".")

		modules = append(modules, moduleInfo{
			name:  modName,
			dir:   filepath.Dir(path),
			decls: prog.Decls,
		})
	}

	fmt.Println("Rex type-check passed")

	// Step 2: Find directories with companion .go files
	companionDirs := map[string]bool{}
	filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") {
			// Skip generated files
			base := filepath.Base(path)
			if base == "rex_runtime.go" || strings.HasSuffix(base, ".types.go") {
				return nil
			}
			companionDirs[filepath.Dir(path)] = true
		}
		return nil
	})

	if len(companionDirs) == 0 {
		fmt.Println("No Go companion files found — skipping Go compilation")
		return
	}

	// Step 3: Generate rex_runtime.go in each directory with companions
	for dir := range companionDirs {
		runtimePath := filepath.Join(dir, "rex_runtime.go")
		mustWriteFile(runtimePath, codegen.GenerateLibRuntime(pkgName))
	}

	// Step 4: Generate <Module>.types.go for each module with type declarations
	for _, mod := range modules {
		typesContent := codegen.GenerateLibTypes(pkgName, mod.decls)
		if typesContent == "" {
			continue
		}
		typesPath := filepath.Join(mod.dir, mod.name+".types.go")
		mustWriteFile(typesPath, typesContent)
	}

	// Step 5: Generate go.mod at library root
	goMod := fmt.Sprintf("module rexlib/%s\n\ngo 1.24\n", pkgName)
	if len(m.Go.Requires) > 0 {
		goMod += "\nrequire (\n"
		for mod, ver := range m.Go.Requires {
			goMod += fmt.Sprintf("\t%s %s\n", mod, ver)
		}
		goMod += ")\n"
	}
	mustWriteFile(filepath.Join(cwd, "go.mod"), goMod)

	// Step 6: Run go mod tidy
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = cwd
	tidyCmd.Stdout = os.Stdout
	tidyCmd.Stderr = os.Stderr
	if err := tidyCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "go mod tidy failed: %v\n", err)
		os.Exit(1)
	}

	// Step 7: Run go build ./src/
	buildCmd := exec.Command("go", "build", "./src/")
	buildCmd.Dir = cwd
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "go build failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Go compilation passed")
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
// readSourceWithOverlay reads the entry file and, if a target-specific overlay
// exists (e.g. Foo.browser.rex for --target=browser), concatenates it.
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// showTypes
// ---------------------------------------------------------------------------

func showTypes(path string) {
	path = resolveOverlayEntry(path)
	srcRoot := setupSrcRoot(path)

	source, err := readSourceWithOverlay(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	exprs, err := parser.Parse(source)
	if err != nil {
		printErr("Parse error", err)
		os.Exit(1)
	}
	if err := parser.ValidateToplevel(exprs); err != nil {
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

	absPath, _ := filepath.Abs(path)
	extraTypeEnv := stdlibExtraTypeEnv(absPath)

	var typeEnv typechecker.TypeEnv
	var warnings []typechecker.Warning
	if extraTypeEnv != nil {
		typeEnv, warnings, err = typechecker.CheckProgramWithExtraEnv(exprs, extraTypeEnv, srcRoot)
	} else {
		typeEnv, warnings, err = typechecker.CheckProgram(exprs, srcRoot)
	}
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(path, warnings)

	// Collect top-level names in source order
	var names []string
	seen := map[string]bool{}
	for _, e := range exprs {
		switch e := e.(type) {
		case ast.Let:
			if e.InExpr == nil && e.Name != "_" && !seen[e.Name] {
				names = append(names, e.Name)
				seen[e.Name] = true
			}
		case ast.LetRec:
			if e.InExpr == nil {
				for _, b := range e.Bindings {
					if !seen[b.Name] {
						names = append(names, b.Name)
						seen[b.Name] = true
					}
				}
			}
		}
	}

	for _, name := range names {
		v, ok := typeEnv[name]
		if !ok {
			continue
		}
		if scheme, ok := v.(types.Scheme); ok {
			fmt.Printf("%s : %s\n", name, types.SchemeToString(scheme))
		}
	}
}

func graphFile(path string) {
	path = resolveOverlayEntry(path)
	srcRoot := setupSrcRoot(path)

	source, err := readSourceWithOverlay(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	exprs, err := parser.Parse(source)
	if err != nil {
		printErr("Parse error", err)
		os.Exit(1)
	}
	if err := parser.ValidateToplevel(exprs); err != nil {
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

	absPath, _ := filepath.Abs(path)
	extraTypeEnv := stdlibExtraTypeEnv(absPath)

	var warnings []typechecker.Warning
	if extraTypeEnv != nil {
		_, warnings, err = typechecker.CheckProgramWithExtraEnv(exprs, extraTypeEnv, srcRoot)
	} else {
		_, warnings, err = typechecker.CheckProgram(exprs, srcRoot)
	}
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(path, warnings)

	importInfo, err := ir.ResolveImports(exprs, srcRoot, targetMode, packageRoots)
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

	// Shake from user-defined functions only (no $ in name) to remove
	// unreachable stdlib internals while keeping all user code.
	var roots []string
	for _, d := range prog.Decls {
		switch dl := d.(type) {
		case ir.DLet:
			if callgraph.IsUserFunc(dl.Name) {
				roots = append(roots, dl.Name)
			}
		case ir.DLetRec:
			for _, b := range dl.Bindings {
				if callgraph.IsUserFunc(b.Name) {
					roots = append(roots, b.Name)
				}
			}
		}
	}
	if len(roots) > 0 {
		prog = ir.ShakeFrom(prog, roots...)
	}

	g := callgraph.Build(prog)
	g.WriteDot(os.Stdout)
}

// compileToIR runs the frontend pipeline: parse → validate → typecheck → IR.
// Errors are printed to stderr; returns (nil, nil, err) on failure.
func compileToIR(source, path string, testMode bool, srcRoot string) (*ir.Program, typechecker.TypeEnv, error) {
	done := compileTiming.phase("Parse + validate")
	exprs, err := parser.Parse(source)
	if err != nil {
		done()
		printErr("Parse error", err)
		return nil, nil, err
	}
	if err := parser.ValidateToplevel(exprs); err != nil {
		done()
		printErr("Syntax error", err)
		return nil, nil, err
	}
	if err := parser.ValidateIndentation(exprs); err != nil {
		done()
		printErr("Indentation error", err)
		return nil, nil, err
	}
	done()

	// Detect if this is a stdlib file for extra type env
	absPath, _ := filepath.Abs(path)
	extraTypeEnv := stdlibExtraTypeEnv(absPath)

	done = compileTiming.phase("Typecheck")
	var typeEnv typechecker.TypeEnv
	var warnings []typechecker.Warning
	if extraTypeEnv != nil {
		typeEnv, warnings, err = typechecker.CheckProgramWithExtraEnv(exprs, extraTypeEnv, srcRoot)
	} else {
		typeEnv, warnings, err = typechecker.CheckProgram(exprs, srcRoot)
	}
	done()
	if err != nil {
		if te, ok := err.(*types.TypeError); ok {
			te.Source = source
			te.File = path
		}
		printErr("Type error", err)
		return nil, nil, err
	}
	handleWarnings(path, warnings)

	done = compileTiming.phase("Import resolution")
	importInfo, err := ir.ResolveImports(exprs, srcRoot, targetMode, packageRoots)
	done()
	if err != nil {
		printErr("Import resolution error", err)
		return nil, nil, err
	}
	userExprs := ir.ApplyAliases(exprs, importInfo.Aliases)
	allExprs := append(importInfo.Decls, userExprs...)

	done = compileTiming.phase("IR lowering")
	l := ir.NewLowerer()
	prog, err := l.LowerProgram(allExprs)
	done()
	if err != nil {
		printErr("IR error", err)
		return nil, nil, err
	}

	// If testing a stdlib file directly, prefix bare external names with module qualifier
	if stdlibModName := stdlibModuleName(path); stdlibModName != "" {
		ir.PrefixExternals(prog, stdlibModName)
	}

	if testMode {
		prog = ir.ShakeForTests(prog)
	} else {
		prog = ir.Shake(prog)
	}

	return prog, typeEnv, nil
}

// stdlibModuleName returns the stdlib module name for a file path, or "" if not a stdlib file.
// e.g., ".../rexfiles/Bitwise.rex" → "Bitwise", ".../rexfiles/Http.Server.rex" → "Http.Server"
func stdlibModuleName(path string) string {
	absPath, _ := filepath.Abs(path)
	if !strings.HasSuffix(absPath, ".rex") {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(absPath), ".rex")
	if typechecker.TypeEnvForModule(base) != nil {
		return base
	}
	dir := filepath.Base(filepath.Dir(absPath))
	dotted := dir + "." + base
	if typechecker.TypeEnvForModule(dotted) != nil {
		return dotted
	}
	return ""
}

// stdlibExtraTypeEnv returns extra type environment for stdlib module testing.
func stdlibExtraTypeEnv(absPath string) typechecker.TypeEnv {
	if !strings.HasSuffix(absPath, ".rex") {
		return nil
	}
	base := strings.TrimSuffix(filepath.Base(absPath), ".rex")
	if typechecker.TypeEnvForModule(base) != nil {
		return typechecker.TypeEnvForModule(base)
	}
	dir := filepath.Base(filepath.Dir(absPath))
	dotted := dir + "." + base
	if typechecker.TypeEnvForModule(dotted) != nil {
		return typechecker.TypeEnvForModule(dotted)
	}
	return nil
}

// buildGoProgram compiles an IR program to a Go binary in .cache/rex-build/.
// Returns the binary path. The build dir is a stable location so that Go's
// own content-based build cache makes repeated runs fast.
func buildGoProgram(prog *ir.Program, typeEnv typechecker.TypeEnv, path string, testMode bool) string {
	// Emit Go source
	doneCodegen := compileTiming.phase("Go codegen")
	var goSrc string
	var err error
	if testMode {
		goSrc, err = codegen.EmitGoTests(prog, typeEnv)
	} else {
		goSrc, err = codegen.EmitGo(prog, typeEnv)
	}
	doneCodegen()
	if err != nil {
		printErr("Codegen error", err)
		os.Exit(1)
	}

	// Determine build directory: .cache/rex-build/ under project root (or cwd)
	cwd, _ := os.Getwd()
	root := manifest.FindProjectRoot(cwd)
	if root == "" {
		root = cwd
	}
	buildDir := filepath.Join(root, ".cache", "rex-build")

	// Create build dir (preserve existing for Go build cache)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating build dir: %v\n", err)
		os.Exit(1)
	}

	// Track which files we write so we can clean up stale ones
	written := map[string]bool{}

	// Write main.go
	goFile := filepath.Join(buildDir, "main.go")
	mustWriteFile(goFile, goSrc)
	written[goFile] = true

	// Write go.mod (with Go requires from rex.toml if any)
	base := strings.TrimSuffix(filepath.Base(path), ".rex")
	goRequires := collectGoRequires()
	goMod := fmt.Sprintf("module rex_%s\n\ngo 1.24\n", base)
	if len(goRequires) > 0 {
		goMod += "\nrequire (\n"
		for mod, ver := range goRequires {
			goMod += fmt.Sprintf("\t%s %s\n", mod, ver)
		}
		goMod += ")\n"
	}
	goModPath := filepath.Join(buildDir, "go.mod")
	mustWriteFile(goModPath, goMod)
	written[goModPath] = true

	// Extract runtime
	runtimePath := filepath.Join(buildDir, "runtime.go")
	mustWriteFile(runtimePath, codegen.RuntimeSource())
	written[runtimePath] = true

	// Extract companion files for needed stdlib modules
	modules := codegen.NeededModules(prog, typeEnv)
	for _, mod := range modules {
		src := stdlib.GoCompanion(mod)
		if src != "" {
			p := filepath.Join(buildDir, "stdlib_"+strings.ToLower(strings.ReplaceAll(mod, ".", "_"))+".go")
			mustWriteFile(p, src)
			written[p] = true
		}
	}

	// Extract companion files for needed user package modules
	pkgCompanions := codegen.NeededPackageCompanions(prog, typeEnv)
	for _, pc := range pkgCompanions {
		pkgSrc, ok := packageRoots[pc.Namespace]
		if !ok {
			continue
		}
		modPath := strings.ReplaceAll(pc.Module, ".", "/")
		companionPath := filepath.Join(pkgSrc, modPath+".go")
		data, err := os.ReadFile(companionPath)
		if err != nil {
			continue // no companion file — externals may be provided by user code
		}
		src := rewriteCompanionPkg(string(data))
		destName := "pkg_" + pc.Namespace + "_" + strings.ToLower(strings.ReplaceAll(pc.Module, ".", "_")) + ".go"
		p := filepath.Join(buildDir, destName)
		mustWriteFile(p, src)
		written[p] = true
	}

	// Remove stale .go files from previous builds
	entries, _ := os.ReadDir(buildDir)
	for _, e := range entries {
		p := filepath.Join(buildDir, e.Name())
		if strings.HasSuffix(e.Name(), ".go") && !written[p] {
			os.Remove(p)
		}
	}

	// Fetch Go dependencies if needed
	if len(goRequires) > 0 {
		doneTidy := compileTiming.phase("go mod tidy")
		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = buildDir
		if tidyOut, tidyErr := tidyCmd.CombinedOutput(); tidyErr != nil {
			fmt.Fprintf(os.Stderr, "go mod tidy failed: %v\n%s\n", tidyErr, tidyOut)
			os.Exit(1)
		}
		doneTidy()
	}

	// Build
	binaryPath := filepath.Join(buildDir, "program")
	doneBuild := compileTiming.phase("go build")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = buildDir
	out, err := cmd.CombinedOutput()
	doneBuild()
	if err != nil {
		fmt.Fprintf(os.Stderr, "go build failed: %v\n%s\nGenerated Go source: %s\n", err, out, goFile)
		os.Exit(1)
	}

	return binaryPath
}

// collectGoRequires gathers [go.requires] from the project and all dependency rex.toml files.
func collectGoRequires() map[string]string {
	requires := map[string]string{}

	// Collect from the project's own rex.toml
	cwd, _ := os.Getwd()
	projectRoot := manifest.FindProjectRoot(cwd)
	if projectRoot != "" {
		m, _, err := manifest.Load(projectRoot)
		if err == nil {
			for mod, ver := range m.Go.Requires {
				requires[mod] = ver
			}
		}
	}

	// Collect from each dependency package's rex.toml
	for _, srcDir := range packageRoots {
		pkgRoot := filepath.Dir(srcDir) // src/ → package root
		m, _, err := manifest.Load(pkgRoot)
		if err != nil {
			continue
		}
		for mod, ver := range m.Go.Requires {
			if _, exists := requires[mod]; !exists {
				requires[mod] = ver
			}
		}
	}
	return requires
}

func mustWriteFile(path, content string) {
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
		os.Exit(1)
	}
}

// rewriteCompanionPkg rewrites a companion .go file's package declaration to
// "package main" for inclusion in the flat build directory. Handles both the
// old format (//go:build ignore + package main) and the new format (package <name>).
var pkgDeclRe = regexp.MustCompile(`(?m)^package\s+\w+`)

func rewriteCompanionPkg(src string) string {
	// Strip old-style build tag if present (backwards compat)
	if strings.HasPrefix(src, "//go:build ignore\n") {
		src = src[len("//go:build ignore\n"):]
	}
	// Rewrite package declaration to main
	return pkgDeclRe.ReplaceAllString(src, "package main")
}

// resolveOverlayEntry detects when the user passes a target-overlay file
// (e.g., Foo.browser.rex or Foo.native.rex) and resolves to the base file.
// If no base file exists, treats it as a target-only module (like Std:Js).
// Returns the (possibly resolved) path and switches targetMode to match.
func resolveOverlayEntry(path string) string {
	knownTargets := []string{"native", "browser"}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for _, t := range knownTargets {
		suffix := "." + t
		if strings.HasSuffix(base, suffix) {
			basePath := strings.TrimSuffix(base, suffix) + ext
			if _, err := os.Stat(basePath); err == nil {
				fmt.Fprintf(os.Stderr, "Note: resolved %s → %s + %s overlay\n", filepath.Base(path), filepath.Base(basePath), t)
				targetMode = t
				return basePath
			}
			// No base file — target-only module (e.g., browser-only). Just set target mode.
			targetMode = t
			return path
		}
	}
	return path
}

func readSourceWithOverlay(path string) (string, error) {
	base, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	src := string(base)

	// Build overlay path: Foo.rex -> Foo.<target>.rex
	ext := filepath.Ext(path)
	overlayPath := strings.TrimSuffix(path, ext) + "." + targetMode + ext
	if overlayPath == path {
		return src, nil
	}
	overlay, err := os.ReadFile(overlayPath)
	if err != nil {
		return src, nil // no overlay, that's fine
	}
	return src + "\n" + string(overlay), nil
}

// ---------------------------------------------------------------------------
// compileGoFile
// ---------------------------------------------------------------------------

func compileGoFile(path string) {
	path = resolveOverlayEntry(path)
	srcRoot := setupSrcRoot(path)

	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	prog, typeEnv, err := compileToIR(string(source), path, false, srcRoot)
	if err != nil {
		os.Exit(1)
	}

	goSrc, err := codegen.EmitGo(prog, typeEnv)
	if err != nil {
		printErr("Codegen error", err)
		os.Exit(1)
	}

	base := strings.TrimSuffix(filepath.Base(path), ".rex")
	goDir := base + "_go"
	goFile := filepath.Join(goDir, "main.go")
	binaryPath := base

	if err := os.MkdirAll(goDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(goFile, []byte(goSrc), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing Go source: %v\n", err)
		os.Exit(1)
	}

	// Extract runtime
	if err := os.WriteFile(filepath.Join(goDir, "runtime.go"), []byte(codegen.RuntimeSource()), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing runtime.go: %v\n", err)
		os.Exit(1)
	}

	// Extract companion files
	modules := codegen.NeededModules(prog, typeEnv)
	for _, mod := range modules {
		src := stdlib.GoCompanion(mod)
		if src != "" {
			p := filepath.Join(goDir, "stdlib_"+strings.ToLower(mod)+".go")
			if err := os.WriteFile(p, []byte(src), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing companion file: %v\n", err)
				os.Exit(1)
			}
		}
	}

	// Extract companion files for needed user package modules
	pkgCompanions := codegen.NeededPackageCompanions(prog, typeEnv)
	for _, pc := range pkgCompanions {
		pkgSrc, ok := packageRoots[pc.Namespace]
		if !ok {
			continue
		}
		modPath := strings.ReplaceAll(pc.Module, ".", "/")
		companionPath := filepath.Join(pkgSrc, modPath+".go")
		data, err := os.ReadFile(companionPath)
		if err != nil {
			continue
		}
		src := rewriteCompanionPkg(string(data))
		destName := "pkg_" + pc.Namespace + "_" + strings.ToLower(strings.ReplaceAll(pc.Module, ".", "_")) + ".go"
		p := filepath.Join(goDir, destName)
		if err := os.WriteFile(p, []byte(src), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing package companion file: %v\n", err)
			os.Exit(1)
		}
	}

	goRequires := collectGoRequires()
	goMod := fmt.Sprintf("module rex_%s\n\ngo 1.24\n", base)
	if len(goRequires) > 0 {
		goMod += "\nrequire (\n"
		for mod, ver := range goRequires {
			goMod += fmt.Sprintf("\t%s %s\n", mod, ver)
		}
		goMod += ")\n"
	}
	if err := os.WriteFile(filepath.Join(goDir, "go.mod"), []byte(goMod), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing go.mod: %v\n", err)
		os.Exit(1)
	}

	// Fetch Go dependencies if needed
	if len(goRequires) > 0 {
		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = goDir
		if tidyOut, tidyErr := tidyCmd.CombinedOutput(); tidyErr != nil {
			fmt.Fprintf(os.Stderr, "go mod tidy failed: %v\n%s\n", tidyErr, tidyOut)
			os.Exit(1)
		}
	}

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
	path = resolveOverlayEntry(path)
	srcRoot := setupSrcRoot(path)

	source, err := readSourceWithOverlay(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	exprs, err := parser.Parse(source)
	if err != nil {
		printErr("Parse error", err)
		os.Exit(1)
	}

	if err := parser.ValidateToplevel(exprs); err != nil {
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

	typeEnv, warnings, err := typechecker.CheckProgram(exprs, srcRoot)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}
	handleWarnings(path, warnings)

	importInfo, err := ir.ResolveImports(exprs, srcRoot, targetMode, packageRoots)
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
	path = resolveOverlayEntry(path)
	srcRoot := setupSrcRoot(path)

	source, err := readSourceWithOverlay(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	// Frontend: parse → typecheck → IR
	prog, typeEnv, err := compileToIR(source, path, false, srcRoot)
	if err != nil {
		os.Exit(1) // errors already printed by compileToIR
	}

	// Validate main exists and unifies with List String -> Int
	mainScheme, ok := typeEnv["main"]
	if !ok {
		printErr("Type error", fmt.Errorf("no main function — add 'export main args = ...'"))
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

	// Compile to Go and execute
	binary := buildGoProgram(prog, typeEnv, path, false)
	compileTiming.print()

	cmd := exec.Command(binary, programArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// buildBinary
// ---------------------------------------------------------------------------

func buildBinary(path string, outPath string) {
	path = resolveOverlayEntry(path)
	srcRoot := setupSrcRoot(path)

	src, err := readSourceWithOverlay(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	prog, typeEnv, err := compileToIR(src, path, false, srcRoot)
	if err != nil {
		os.Exit(1)
	}

	binary := buildGoProgram(prog, typeEnv, path, false)
	compileTiming.print()

	// Default output: lowercase basename without extension
	if outPath == "" {
		base := strings.TrimSuffix(filepath.Base(path), ".rex")
		outPath = strings.ToLower(base)
	}

	// Copy binary to output path
	data, err := os.ReadFile(binary)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading binary: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, data, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing binary: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Built %s\n", outPath)
}

// ---------------------------------------------------------------------------
// runTestsBatch — compile multiple test files into a single binary
// ---------------------------------------------------------------------------

func runTestsBatch(files []string, only string) bool {
	// Phase 1: compile all files to IR sequentially (uses shared global state)
	type compiled struct {
		path    string
		prog    *ir.Program
		typeEnv typechecker.TypeEnv
	}
	var items []compiled
	for _, path := range files {
		path = resolveOverlayEntry(path)
		srcRoot := setupSrcRoot(path)
		src, err := readSourceWithOverlay(path)
		if err != nil {
			printTestErr(path, "error", err)
			continue
		}
		prog, typeEnv, err := compileToIR(src, path, true, srcRoot)
		if err != nil {
			continue
		}
		items = append(items, compiled{path: path, prog: prog, typeEnv: typeEnv})
	}

	// Phase 2: build and run in parallel (each gets its own build dir)
	type result struct {
		idx    int
		output string
		ok     bool
	}
	results := make(chan result, len(items))
	sem := make(chan struct{}, 4) // limit to 4 parallel go builds

	for i, item := range items {
		go func(idx int, item compiled) {
			sem <- struct{}{}
			defer func() { <-sem }()

			// Emit Go source
			goSrc, err := codegen.EmitGoTests(item.prog, item.typeEnv)
			if err != nil {
				results <- result{idx: idx, output: fmt.Sprintf("Codegen error: %v\n", err), ok: false}
				return
			}

			// Build in a temp dir
			buildDir, err := os.MkdirTemp("", "rex-test-*")
			if err != nil {
				results <- result{idx: idx, output: fmt.Sprintf("Error: %v\n", err), ok: false}
				return
			}
			defer os.RemoveAll(buildDir)

			base := strings.TrimSuffix(filepath.Base(item.path), ".rex")
			os.WriteFile(filepath.Join(buildDir, "main.go"), []byte(goSrc), 0644)
			os.WriteFile(filepath.Join(buildDir, "go.mod"), []byte(fmt.Sprintf("module rex_%s_%d\n\ngo 1.24\n", base, idx)), 0644)
			os.WriteFile(filepath.Join(buildDir, "runtime.go"), []byte(codegen.RuntimeSource()), 0644)

			for _, mod := range codegen.NeededModules(item.prog, item.typeEnv) {
				src := stdlib.GoCompanion(mod)
				if src != "" {
					p := filepath.Join(buildDir, "stdlib_"+strings.ToLower(strings.ReplaceAll(mod, ".", "_"))+".go")
					os.WriteFile(p, []byte(src), 0644)
				}
			}

			binaryPath := filepath.Join(buildDir, "program")
			cmd := exec.Command("go", "build", "-o", binaryPath, ".")
			cmd.Dir = buildDir
			out, err := cmd.CombinedOutput()
			if err != nil {
				goFile := filepath.Join(buildDir, "main.go")
				results <- result{idx: idx, output: fmt.Sprintf("go build failed: %v\n%s\nGenerated Go source: %s\n", err, out, goFile), ok: false}
				return
			}

			var runCmd *exec.Cmd
			if only != "" {
				runCmd = exec.Command(binaryPath, only)
			} else {
				runCmd = exec.Command(binaryPath)
			}
			var buf strings.Builder
			runCmd.Stdout = &buf
			runCmd.Stderr = &buf
			err = runCmd.Run()
			results <- result{idx: idx, output: buf.String(), ok: err == nil}
		}(i, item)
	}

	// Collect results in order
	resultMap := make(map[int]result)
	for range items {
		r := <-results
		resultMap[r.idx] = r
	}
	anyFailed := false
	for i := range items {
		r := resultMap[i]
		if r.output != "" {
			fmt.Print(r.output)
		}
		if !r.ok {
			anyFailed = true
		}
	}
	return !anyFailed
}

// ---------------------------------------------------------------------------
// runTests
// ---------------------------------------------------------------------------

func runTests(path string, only string) bool {
	path = resolveOverlayEntry(path)
	srcRoot := setupSrcRoot(path)

	src, err := readSourceWithOverlay(path)
	if err != nil {
		printTestErr(path, "error", err)
		return false
	}

	// Frontend: parse → typecheck → IR (test mode)
	prog, typeEnv, err := compileToIR(src, path, true, srcRoot)
	if err != nil {
		return false // errors already printed
	}

	// Compile and run tests
	binary := buildGoProgram(prog, typeEnv, path, true)
	compileTiming.print()

	var cmd *exec.Cmd
	if only != "" {
		cmd = exec.Command(binary, only)
	} else {
		cmd = exec.Command(binary)
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			return false
		}
		printTestErr(path, "error", err)
		return false
	}
	return true
}
