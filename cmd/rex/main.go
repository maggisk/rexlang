// Command rex is the RexLang interpreter and REPL.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/eval"
	"github.com/maggisk/rexlang/internal/lexer"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/stdlib"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// safeMode is set by the --safe flag; it promotes warnings (todo usage) to errors.
var safeMode bool

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
		} else {
			filtered = append(filtered, a)
		}
	}
	args = filtered

	if len(args) == 0 {
		repl()
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
// and sets the srcRoot for both typechecker and eval.
func setupSrcRoot(entryFile string) {
	cwd, err := os.Getwd()
	if err != nil {
		return
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
	base := filepath.Base(absPath)
	if !strings.HasSuffix(base, ".rex") {
		return ""
	}
	name := strings.TrimSuffix(base, ".rex")
	// Check if this module exists in stdlib
	if _, err := stdlib.Source(name); err == nil {
		return name
	}
	return ""
}

// ---------------------------------------------------------------------------
// REPL
// ---------------------------------------------------------------------------

func repl() {
	fmt.Println("RexLang v0.1.0 (Go)")
	fmt.Println("Press Enter on a blank line to evaluate. Ctrl-D to exit.")
	fmt.Println()

	preludeTC, err := typechecker.LoadPreludeTC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load prelude: %v\n", err)
		os.Exit(1)
	}
	// Use the exported fields via the type
	evalEnv, err := eval.LoadPreludeForREPL(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load prelude: %v\n", err)
		os.Exit(1)
	}
	typeEnv := preludeTC.Env.Clone()
	typeDefs := typechecker.CopyTypeDefs(preludeTC.TypeDefs)
	tc := typechecker.NewTypeChecker()

	scanner := bufio.NewScanner(os.Stdin)
	var buf []string

	for {
		prompt := "rex> "
		if len(buf) > 0 {
			prompt = "  .. "
		}
		fmt.Print(prompt)

		if !scanner.Scan() {
			fmt.Println()
			break
		}
		line := scanner.Text()

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
