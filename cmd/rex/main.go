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
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: rex --test <file.rex> [file.rex ...]")
			os.Exit(1)
		}
		totalFailed := 0
		for _, path := range args[1:] {
			totalFailed += runTests(path)
		}
		if totalFailed > 0 {
			os.Exit(1)
		}
		return
	}
	runFile(args[0], args[1:])
}

func runFile(path string, programArgs []string) {
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

	// Reorder top-level bindings by dependency
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	// Type check
	if _, err := typechecker.CheckProgram(exprs); err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	// Evaluate
	if _, err := eval.RunProgram(exprs, programArgs); err != nil {
		printErr("Runtime error", err)
		os.Exit(1)
	}
}

func runTests(path string) int {
	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
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
		printErr("Lexer/Parse error", err)
		os.Exit(1)
	}

	// Reorder top-level bindings by dependency
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		printErr("Type error", err)
		os.Exit(1)
	}

	// Type check with optional extra env
	if extraTypeEnv != nil {
		if _, err := typechecker.CheckProgramWithExtraEnv(exprs, extraTypeEnv); err != nil {
			printErr("Type error", err)
			os.Exit(1)
		}
	} else {
		if _, err := typechecker.CheckProgram(exprs); err != nil {
			printErr("Type error", err)
			os.Exit(1)
		}
	}

	_, failed, err := eval.RunTests(exprs, nil, extraBuiltins, path)
	if err != nil {
		printErr("Error", err)
		os.Exit(1)
	}
	return failed
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
	evalEnv = eval.WithProcessBuiltins(evalEnv)

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

// Stubs to detect lexer error vs others
var _ = lexer.Tokenize
