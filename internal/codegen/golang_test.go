package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/typechecker"
)

// compileGoCode runs the full pipeline: parse → typecheck → resolve → lower → Go source.
func compileGoCode(t *testing.T, code string) string {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	typeEnv, _, err := typechecker.CheckProgram(exprs)
	if err != nil {
		t.Fatalf("typecheck error: %v", err)
	}
	importInfo, err := ir.ResolveImports(exprs, "", "", nil)
	if err != nil {
		t.Fatalf("resolve imports: %v", err)
	}
	userExprs := ir.ApplyAliases(exprs, importInfo.Aliases)
	allExprs := append(importInfo.Decls, userExprs...)
	l := ir.NewLowerer()
	prog, err := l.LowerProgram(allExprs)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
	prog = ir.Shake(prog)
	goSrc, err := EmitGo(prog, typeEnv)
	if err != nil {
		t.Fatalf("EmitGo error: %v", err)
	}
	return goSrc
}

// runGo compiles Rex source to Go, builds, and runs it, returning the exit code and stdout.
func runGo(t *testing.T, code string) (int, string) {
	t.Helper()
	goSrc := compileGoCode(t, code)

	dir := t.TempDir()
	goFile := filepath.Join(dir, "main.go")
	goMod := filepath.Join(dir, "go.mod")

	if err := os.WriteFile(goFile, []byte(goSrc), 0644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	if err := os.WriteFile(goMod, []byte("module test\n\ngo 1.24\n"), 0644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	binary := filepath.Join(dir, "program")

	buildCmd := exec.Command("go", "build", "-o", binary, ".")
	buildCmd.Dir = dir
	out, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s\nGo source:\n%s", err, out, goSrc)
	}

	cmd := exec.Command(binary)
	stdout, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run failed: %v\n%s", err, stdout)
		}
	}
	return exitCode, string(stdout)
}

// ---------------------------------------------------------------------------
// Step 1: Scaffold + Hello World
// ---------------------------------------------------------------------------

func TestGoMainZero(t *testing.T) {
	code, _ := runGo(t, "main _ = 0\n")
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestGoMainNonZero(t *testing.T) {
	code, _ := runGo(t, "main _ = 42\n")
	if code != 42 {
		t.Errorf("expected exit 42, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Step 2: Primitives + Arithmetic
// ---------------------------------------------------------------------------

func TestGoArithmetic(t *testing.T) {
	code, _ := runGo(t, "main _ = 1 + 2 + 3\n")
	if code != 6 {
		t.Errorf("expected 6, got %d", code)
	}
}

func TestGoSubtract(t *testing.T) {
	code, _ := runGo(t, "main _ = 10 - 3\n")
	if code != 7 {
		t.Errorf("expected 7, got %d", code)
	}
}

func TestGoMultiply(t *testing.T) {
	code, _ := runGo(t, "main _ = 6 * 7\n")
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoDivide(t *testing.T) {
	code, _ := runGo(t, "main _ = 100 / 10\n")
	if code != 10 {
		t.Errorf("expected 10, got %d", code)
	}
}

func TestGoModulo(t *testing.T) {
	code, _ := runGo(t, "main _ = 17 % 5\n")
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestGoLetBinding(t *testing.T) {
	code, _ := runGo(t, `
main _ =
    let x = 10
    in let y = 20
    in x + y
`)
	if code != 30 {
		t.Errorf("expected 30, got %d", code)
	}
}

func TestGoIfThenElse(t *testing.T) {
	code, _ := runGo(t, `
main _ =
    if 1 == 1 then
        10
    else
        20
`)
	if code != 10 {
		t.Errorf("expected 10, got %d", code)
	}
}

func TestGoPrintln(t *testing.T) {
	code, stdout := runGo(t, `
import Std:IO (println)

main _ =
    let _ = println "hello world"
    in 0
`)
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
	if stdout != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", stdout)
	}
}

func TestGoComparisons(t *testing.T) {
	code, _ := runGo(t, `
main _ =
    if 5 > 3 then
        if 2 < 4 then
            if 3 >= 3 then
                if 3 <= 3 then
                    1
                else 0
            else 0
        else 0
    else 0
`)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestGoLogicalOps(t *testing.T) {
	code, _ := runGo(t, `
main _ =
    if true && true then
        if not (true && false) then
            if true || false then
                1
            else 0
        else 0
    else 0
`)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Step 3: Functions + Closures
// ---------------------------------------------------------------------------

func TestGoSimpleFunction(t *testing.T) {
	code, _ := runGo(t, `
add x y = x + y

main _ = add 20 22
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoRecursiveFunction(t *testing.T) {
	code, _ := runGo(t, `
factorial n =
    if n == 0 then
        1
    else
        n * factorial (n - 1)

main _ = factorial 5
`)
	if code != 120 {
		t.Errorf("expected 120, got %d", code)
	}
}

func TestGoHigherOrderFunction(t *testing.T) {
	code, _ := runGo(t, `
apply f x = f x

double x = x * 2

main _ = apply double 21
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoLambda(t *testing.T) {
	code, _ := runGo(t, `
apply f x = f x

main _ = apply (\x -> x + 1) 41
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Step 4: ADTs + Pattern Matching
// ---------------------------------------------------------------------------

func TestGoADTSimple(t *testing.T) {
	code, _ := runGo(t, `
type Color = Red | Green | Blue

toInt c =
    match c
        when Red -> 1
        when Green -> 2
        when Blue -> 3

main _ = toInt Green
`)
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestGoADTWithFields(t *testing.T) {
	code, _ := runGo(t, `
type Shape = Circle Int | Rect Int Int

area s =
    match s
        when Circle r -> r * r
        when Rect w h -> w * h

main _ = area (Rect 6 7)
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoMaybe(t *testing.T) {
	code, _ := runGo(t, `
type Maybe a = Nothing | Just a

withDefault d m =
    match m
        when Nothing -> d
        when Just x -> x

main _ = withDefault 0 (Just 42)
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Step 5: Strings, Lists, Tuples
// ---------------------------------------------------------------------------

func TestGoListLength(t *testing.T) {
	code, _ := runGo(t, `
length lst =
    match lst
        when [] -> 0
        when [_|t] -> 1 + length t

main _ = length [10, 20, 30]
`)
	if code != 3 {
		t.Errorf("expected 3, got %d", code)
	}
}

func TestGoTuple(t *testing.T) {
	code, _ := runGo(t, `
fst pair =
    match pair
        when (a, _) -> a

main _ = fst (42, 0)
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoStringPrint(t *testing.T) {
	_, stdout := runGo(t, `
import Std:IO (println)

main _ =
    let _ = println "hello"
    in 0
`)
	if stdout != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// Step 6: Records
// ---------------------------------------------------------------------------

func TestGoRecord(t *testing.T) {
	code, _ := runGo(t, `
type Point = { x : Int, y : Int }

main _ =
    let p = Point { x = 10, y = 32 }
    in p.x + p.y
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Step 7: Tail Call Optimization
// ---------------------------------------------------------------------------

func TestGoTailRecursion(t *testing.T) {
	// This would stack overflow without TCO (or at least a large stack)
	// For now, just test that basic recursion works
	code, _ := runGo(t, `
sum acc n =
    if n == 0 then
        acc
    else
        sum (acc + n) (n - 1)

main _ = sum 0 100
`)
	// 100 * 101 / 2 = 5050, but exit codes are mod 256
	if code != 5050%256 {
		t.Errorf("expected %d, got %d", 5050%256, code)
	}
}

// ---------------------------------------------------------------------------
// Step 8: Traits
// ---------------------------------------------------------------------------

func TestGoNestedMatch(t *testing.T) {
	code, _ := runGo(t, `
type Op = Add | Sub

eval op a b =
    match op
        when Add -> a + b
        when Sub -> a - b

main _ = eval Add 20 22
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoWildcardMatch(t *testing.T) {
	code, _ := runGo(t, `
type Color = Red | Green | Blue

isRed c =
    match c
        when Red -> 1
        when _ -> 0

main _ = isRed Red
`)
	if code != 1 {
		t.Errorf("expected 1, got %d", code)
	}
}

func TestGoStringConcat(t *testing.T) {
	_, stdout := runGo(t, `
import Std:IO (println)

main _ =
    let _ = println ("hello" ++ " " ++ "world")
    in 0
`)
	if stdout != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", stdout)
	}
}

func TestGoListMap(t *testing.T) {
	code, _ := runGo(t, `
length lst =
    match lst
        when [] -> 0
        when [_|t] -> 1 + length t

myMap f lst =
    match lst
        when [] -> []
        when [h|t] -> f h :: myMap f t

main _ = length (myMap (\x -> x + 1) [1, 2, 3])
`)
	if code != 3 {
		t.Errorf("expected 3, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Step 8: Traits
// ---------------------------------------------------------------------------

func TestGoTraitDispatch(t *testing.T) {
	_, stdout := runGo(t, `
import Std:IO (println)

trait MyShow a where
    myShow : a -> String

impl MyShow Int where
    myShow _ = "an int"

impl MyShow String where
    myShow _ = "a string"

main _ =
    let _ = println (myShow 42)
    in let _ = println (myShow "hello")
    in 0
`)
	if stdout != "an int\na string\n" {
		t.Errorf("expected 'an int\\na string\\n', got %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// Step 9: Stdlib
// ---------------------------------------------------------------------------

func TestGoStdlibMaybe(t *testing.T) {
	code, _ := runGo(t, `
import Std:Maybe (Just, Nothing)

withDefault d m =
    match m
        when Nothing -> d
        when Just x -> x

main _ = withDefault 0 (Just 42)
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoStdlibList(t *testing.T) {
	code, _ := runGo(t, `
import Std:List (length, map)

main _ = length (map (\x -> x + 1) [1, 2, 3, 4, 5])
`)
	if code != 5 {
		t.Errorf("expected 5, got %d", code)
	}
}

func TestGoStdlibListFilter(t *testing.T) {
	code, _ := runGo(t, `
import Std:List (length, filter)

main _ = length (filter (\x -> x > 3) [1, 2, 3, 4, 5])
`)
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestGoStdlibListFoldl(t *testing.T) {
	code, _ := runGo(t, `
import Std:List (foldl)

main _ = foldl (\acc x -> acc + x) 0 [1, 2, 3, 4, 5]
`)
	if code != 15 {
		t.Errorf("expected 15, got %d", code)
	}
}

func TestGoStdlibMultipleImports(t *testing.T) {
	_, stdout := runGo(t, `
import Std:IO (println)
import Std:List (map, foldl)

sum lst = foldl (\acc x -> acc + x) 0 lst

main _ =
    let result = sum (map (\x -> x * 2) [1, 2, 3])
    in let _ = println result
    in 0
`)
	if stdout != "12\n" {
		t.Errorf("expected '12\\n', got %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// Step 10: Actors
// ---------------------------------------------------------------------------

func TestGoSpawnSendReceive(t *testing.T) {
	_, stdout := runGo(t, `
import Std:IO (println)
import Std:Process (spawn, send, receive, call)

main _ =
    let pid = spawn \me ->
        let msg = receive me
        in match msg
            when (replyTo, payload) ->
                send replyTo payload
    in let reply = call pid (\replyTo -> (replyTo, "hello from actor"))
    in let _ = println reply
    in 0
`)
	if stdout != "hello from actor\n" {
		t.Errorf("expected 'hello from actor\\n', got %q", stdout)
	}
}

func TestGoCall(t *testing.T) {
	code, _ := runGo(t, `
import Std:Process (spawn, send, receive, call)

main _ =
    let actor = spawn \me ->
        let msg = receive me
        in match msg
            when (replyTo, n) ->
                send replyTo (n + 1)
    in call actor (\replyTo -> (replyTo, 41))
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestGoLetRec(t *testing.T) {
	code, _ := runGo(t, `
main _ =
    let rec countdown n =
        if n == 0 then
            0
        else
            countdown (n - 1)
    in countdown 10
`)
	if code != 0 {
		t.Errorf("expected 0, got %d", code)
	}
}
