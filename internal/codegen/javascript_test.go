package codegen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maggisk/rexlang/internal/ir"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/typechecker"
)

// compileJSCode runs the full pipeline: parse → typecheck → resolve → lower → JS source.
func compileJSCode(t *testing.T, code string) string {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	typeEnv, _, err := typechecker.CheckProgram(exprs)
	if err != nil {
		t.Fatalf("typecheck error: %v", err)
	}
	importInfo, err := ir.ResolveImports(exprs, "", "")
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
	jsSrc, err := EmitJS(prog, typeEnv)
	if err != nil {
		t.Fatalf("EmitJS error: %v", err)
	}
	return jsSrc
}

// runJS compiles Rex source to JS and runs it with Node.js, returning the exit code and stdout.
// The generated JS uses rex_main(null) without process.exit (browser target),
// so we append process.exit for testability with node.
func runJS(t *testing.T, code string) (int, string) {
	t.Helper()
	jsSrc := compileJSCode(t, code)

	// Replace the browser-style entry point with a node-testable one
	jsSrc = strings.Replace(jsSrc, "\nrex_main(null);\n", "\nprocess.exit(rex_main(null));\n", 1)

	dir := t.TempDir()
	jsFile := filepath.Join(dir, "main.js")

	if err := os.WriteFile(jsFile, []byte(jsSrc), 0644); err != nil {
		t.Fatalf("write js file: %v", err)
	}

	cmd := exec.Command("node", jsFile)
	stdout, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("run failed: %v\n%s\nJS source:\n%s", err, stdout, jsSrc)
		}
	}
	return exitCode, string(stdout)
}

// ---------------------------------------------------------------------------
// Step 1: Scaffold + Hello World
// ---------------------------------------------------------------------------

func TestJSMainZero(t *testing.T) {
	code, _ := runJS(t, "main _ = 0\n")
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

func TestJSMainNonZero(t *testing.T) {
	code, _ := runJS(t, "main _ = 42\n")
	if code != 42 {
		t.Errorf("expected exit 42, got %d", code)
	}
}

// ---------------------------------------------------------------------------
// Step 2: Primitives + Arithmetic
// ---------------------------------------------------------------------------

func TestJSArithmetic(t *testing.T) {
	code, _ := runJS(t, "main _ = 1 + 2 + 3\n")
	if code != 6 {
		t.Errorf("expected 6, got %d", code)
	}
}

func TestJSSubtract(t *testing.T) {
	code, _ := runJS(t, "main _ = 10 - 3\n")
	if code != 7 {
		t.Errorf("expected 7, got %d", code)
	}
}

func TestJSMultiply(t *testing.T) {
	code, _ := runJS(t, "main _ = 6 * 7\n")
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestJSDivide(t *testing.T) {
	code, _ := runJS(t, "main _ = 100 / 10\n")
	if code != 10 {
		t.Errorf("expected 10, got %d", code)
	}
}

func TestJSModulo(t *testing.T) {
	code, _ := runJS(t, "main _ = 17 % 5\n")
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestJSLetBinding(t *testing.T) {
	code, _ := runJS(t, `
main _ =
    let x = 10
    in let y = 20
    in x + y
`)
	if code != 30 {
		t.Errorf("expected 30, got %d", code)
	}
}

func TestJSIfThenElse(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSPrintln(t *testing.T) {
	code, stdout := runJS(t, `
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

func TestJSComparisons(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSLogicalOps(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSSimpleFunction(t *testing.T) {
	code, _ := runJS(t, `
add x y = x + y

main _ = add 20 22
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestJSRecursiveFunction(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSHigherOrderFunction(t *testing.T) {
	code, _ := runJS(t, `
apply f x = f x

double x = x * 2

main _ = apply double 21
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestJSLambda(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSADTSimple(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSADTWithFields(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSMaybe(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSListLength(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSTuple(t *testing.T) {
	code, _ := runJS(t, `
fst pair =
    match pair
        when (a, _) -> a

main _ = fst (42, 0)
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestJSStringPrint(t *testing.T) {
	_, stdout := runJS(t, `
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

func TestJSRecord(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSTailRecursion(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSNestedMatch(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSWildcardMatch(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSStringConcat(t *testing.T) {
	_, stdout := runJS(t, `
import Std:IO (println)

main _ =
    let _ = println ("hello" ++ " " ++ "world")
    in 0
`)
	if stdout != "hello world\n" {
		t.Errorf("expected 'hello world\\n', got %q", stdout)
	}
}

func TestJSListMap(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSTraitDispatch(t *testing.T) {
	_, stdout := runJS(t, `
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

func TestJSStdlibMaybe(t *testing.T) {
	code, _ := runJS(t, `
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

func TestJSStdlibList(t *testing.T) {
	code, _ := runJS(t, `
import Std:List (length, map)

main _ = length (map (\x -> x + 1) [1, 2, 3, 4, 5])
`)
	if code != 5 {
		t.Errorf("expected 5, got %d", code)
	}
}

func TestJSStdlibListFilter(t *testing.T) {
	code, _ := runJS(t, `
import Std:List (length, filter)

main _ = length (filter (\x -> x > 3) [1, 2, 3, 4, 5])
`)
	if code != 2 {
		t.Errorf("expected 2, got %d", code)
	}
}

func TestJSStdlibListFoldl(t *testing.T) {
	code, _ := runJS(t, `
import Std:List (foldl)

main _ = foldl (\acc x -> acc + x) 0 [1, 2, 3, 4, 5]
`)
	if code != 15 {
		t.Errorf("expected 15, got %d", code)
	}
}

func TestJSStdlibMultipleImports(t *testing.T) {
	_, stdout := runJS(t, `
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

func TestJSSpawnSendReceive(t *testing.T) {
	_, stdout := runJS(t, `
import Std:IO (println)
import Std:Process (spawn, send, receive, self)

main _ =
    let me = self
    in let pid = spawn \_ ->
        let msg = receive ()
        in send me msg
    in let _ = send pid "hello from actor"
    in let reply = receive ()
    in let _ = println reply
    in 0
`)
	if stdout != "hello from actor\n" {
		t.Errorf("expected 'hello from actor\\n', got %q", stdout)
	}
}

func TestJSCall(t *testing.T) {
	code, _ := runJS(t, `
import Std:Process (spawn, send, receive, self, call)

main _ =
    let actor = spawn \_ ->
        let msg = receive ()
        in match msg
            when (replyTo, n) ->
                send replyTo (n + 1)
    in call actor (\replyTo -> (replyTo, 41))
`)
	if code != 42 {
		t.Errorf("expected 42, got %d", code)
	}
}

func TestJSActorLoop(t *testing.T) {
	_, stdout := runJS(t, `
import Std:IO (println)
import Std:Process (spawn, send, receive, self, call)
import Std:String (toString)

type Msg = Inc | Get (Pid Int) | Stop

counter =
    spawn \_ ->
        let rec loop n =
            match receive ()
                when Inc ->
                    loop (n + 1)
                when Get replyTo ->
                    let _ = send replyTo n
                    in loop n
                when Stop ->
                    ()
        in
        loop 0

_ = send counter Inc
_ = send counter Inc
_ = send counter Inc
n = call counter Get

main _ =
    let _ = n |> toString |> println
    in 0
`)
	if stdout != "3\n" {
		t.Errorf("expected '3\\n', got %q", stdout)
	}
}

func TestJSLetRec(t *testing.T) {
	code, _ := runJS(t, `
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

// ---------------------------------------------------------------------------
// Browser HTML output
// ---------------------------------------------------------------------------

func TestJSBrowserHTMLOutput(t *testing.T) {
	html := EmitBrowserHTML("counter.js")
	if !strings.Contains(html, `<script src="counter.js">`) {
		t.Error("HTML should include script tag for counter.js")
	}
	if !strings.Contains(html, `<div id="app">`) {
		t.Error("HTML should include app div")
	}
}

func TestJSNoProcessExit(t *testing.T) {
	src := compileJSCode(t, `
export main _ = 0
`)
	if strings.Contains(src, "process.exit") {
		t.Error("JS output should not contain process.exit")
	}
	if strings.Contains(src, "process.stdout") {
		t.Error("JS output should not reference process.stdout")
	}
}
