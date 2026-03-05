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

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		repl()
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
			fmt.Fprintln(os.Stderr, "Usage: rex --test [--only=<pattern>] <file.rex> [file.rex ...]")
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

	// Reorder top-level bindings by dependency
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	// Type check
	typeEnv, err := typechecker.CheckProgram(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

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

	// Reorder top-level bindings by dependency
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printTestErr(path, "Type error", err)
		return 1, nil
	}

	// Type check with optional extra env
	if extraTypeEnv != nil {
		if _, err := typechecker.CheckProgramWithExtraEnv(exprs, extraTypeEnv); err != nil {
			printTestErr(path, "Type error", err)
			return 1, nil
		}
	} else {
		if _, err := typechecker.CheckProgram(exprs); err != nil {
			printTestErr(path, "Type error", err)
			return 1, nil
		}
	}

	_, failed, failedNames, err := eval.RunTests(exprs, nil, extraBuiltins, path, only)
	if err != nil {
		printTestErr(path, "error", err)
		return 1, nil
	}
	return failed, failedNames
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

			tyStr := types.TypeToString(res.Ty)
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
			fmt.Printf("%s : %s\n", name, tyStr)
			fmt.Printf("=> %s\n", eval.ValueToString(val))
		}
	}
}

func printErr(kind string, err error) {
	fmt.Fprintf(os.Stderr, "%s: %v\n", kind, err)
}

func printTestErr(path, kind string, err error) {
	fmt.Println() // blank line to separate from any preceding test output
	fmt.Fprintf(os.Stderr, "%s: %s: %v\n", path, kind, err)
}

// Stubs to detect lexer error vs others
var _ = lexer.Tokenize
