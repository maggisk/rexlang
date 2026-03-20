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

// compileCode runs the full pipeline: parse → typecheck → resolve imports → lower → WAT.
func compileCode(t *testing.T, code string) string {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	typeEnv, _, err := typechecker.CheckProgram(exprs, "")
	if err != nil {
		t.Fatalf("typecheck error: %v", err)
	}
	// Resolve imports: collect module type/trait/impl/function declarations
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
	wat, err := EmitWAT(prog, typeEnv)
	if err != nil {
		t.Fatalf("EmitWAT error: %v", err)
	}
	return wat
}

func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runWasm compiles Rex source to .wasm and runs it, returning the exit code.
func runWasm(t *testing.T, code string) int {
	t.Helper()
	if !hasCmd("wasm-tools") {
		t.Skip("wasm-tools not found")
	}
	if !hasCmd("wasmtime") {
		t.Skip("wasmtime not found")
	}

	wat := compileCode(t, code)

	dir := t.TempDir()
	watFile := filepath.Join(dir, "main.wat")
	wasmFile := filepath.Join(dir, "main.wasm")

	if err := os.WriteFile(watFile, []byte(wat), 0644); err != nil {
		t.Fatalf("write wat: %v", err)
	}

	out, err := exec.Command("wasm-tools", "parse", watFile, "-o", wasmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("wasm-tools parse: %v\n%s\nWAT:\n%s", err, out, wat)
	}

	cmd := exec.Command("wasmtime", "--wasm", "gc", "--wasm", "function-references", "--wasm", "tail-call", wasmFile)
	out, err = cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
			if len(out) > 0 {
				t.Logf("wasmtime stderr: %s", out)
			}
		} else {
			t.Fatalf("wasmtime: %v\n%s", err, out)
		}
	}
	return exitCode
}

// ---------------------------------------------------------------------------
// WAT output tests (no wasm-tools needed)
// ---------------------------------------------------------------------------

func TestWATMainZero(t *testing.T) {
	wat := compileCode(t, "main _ = 0\n")
	if !strings.Contains(wat, "i64.const 0") {
		t.Error("expected i64.const 0")
	}
	if !strings.Contains(wat, "i32.wrap_i64") {
		t.Error("expected i32.wrap_i64 for proc_exit")
	}
}

func TestWATArithmetic(t *testing.T) {
	wat := compileCode(t, "main _ = 1 + 2\n")
	if !strings.Contains(wat, "i64.add") {
		t.Error("expected i64.add")
	}
}

func TestWATFloat(t *testing.T) {
	wat := compileCode(t, "main _ = 1.5 + 2.5\n")
	if !strings.Contains(wat, "f64.const") {
		t.Error("expected f64.const")
	}
	if !strings.Contains(wat, "f64.add") {
		t.Error("expected f64.add")
	}
}

func TestWATFunctionCall(t *testing.T) {
	wat := compileCode(t, "double x = x + x\nmain _ = double 21\n")
	if !strings.Contains(wat, "func $double") {
		t.Error("expected func $double")
	}
	if !strings.Contains(wat, "call $double") {
		t.Error("expected call $double")
	}
}

func TestWATNoMain(t *testing.T) {
	exprs, _ := parser.Parse("x = 42\n")
	l := ir.NewLowerer()
	prog, _ := l.LowerProgram(exprs)
	_, err := EmitWAT(prog, nil)
	if err == nil || !strings.Contains(err.Error(), "no main") {
		t.Fatalf("expected 'no main' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// End-to-end tests
// ---------------------------------------------------------------------------

func TestE2EZero(t *testing.T) {
	if got := runWasm(t, "main _ = 0\n"); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}

func TestE2ELiteral(t *testing.T) {
	if got := runWasm(t, "main _ = 42\n"); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EArithmetic(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ = 10 + 32\n", 42},
		{"main _ = 100 - 58\n", 42},
		{"main _ = 6 * 7\n", 42},
		{"main _ = 84 / 2\n", 42},
		{"main _ = 85 % 43\n", 42},
		{"main _ = 1 + 2 + 3\n", 6},
		{"main _ = 10 - 3 * 2\n", 4},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EUnaryMinus(t *testing.T) {
	if got := runWasm(t, "main _ = -(-42)\n"); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EIfThenElse(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ =\n    if true then\n        42\n    else\n        0\n", 42},
		{"main _ =\n    if false then\n        0\n    else\n        42\n", 42},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EComparison(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ =\n    if 1 < 2 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 2 > 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 <= 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 >= 1 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 42 == 42 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 1 != 2 then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if 2 < 1 then\n        1\n    else\n        0\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EBooleanOps(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ =\n    if true && true then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if true && false then\n        1\n    else\n        0\n", 0},
		{"main _ =\n    if false || true then\n        1\n    else\n        0\n", 1},
		{"main _ =\n    if false || false then\n        1\n    else\n        0\n", 0},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2EFloatArithmetic(t *testing.T) {
	tests := []struct {
		code     string
		exitCode int
	}{
		{"main _ = 1.5 + 2.5\n", 4},
		{"main _ = 10.0 - 3.0\n", 7},
		{"main _ = 6.0 * 7.0\n", 42},
		{"main _ = 84.0 / 2.0\n", 42},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			if got := runWasm(t, tt.code); got != tt.exitCode {
				t.Fatalf("expected exit %d, got %d", tt.exitCode, got)
			}
		})
	}
}

func TestE2ENestedIf(t *testing.T) {
	code := `
main _ =
    if 1 < 2 then
        if 3 > 4 then
            10
        else
            42
    else
        0
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ELetWithComparison(t *testing.T) {
	code := `
main _ =
    let x = 6 * 7
    in
    if x == 42 then
        1
    else
        0
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Function call tests
// ---------------------------------------------------------------------------

func TestE2EFunctionCall(t *testing.T) {
	code := `
double x = x + x
main _ = double 21
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EMultiArgFunction(t *testing.T) {
	code := `
add x y = x + y
main _ = add 20 22
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EThreeArgFunction(t *testing.T) {
	code := `
add3 a b c = a + b + c
main _ = add3 10 20 12
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ERecursion(t *testing.T) {
	code := `
fact n =
    if n == 0 then
        1
    else
        n * fact (n - 1)
main _ = fact 5
`
	// 5! = 120
	if got := runWasm(t, code); got != 120 {
		t.Fatalf("expected exit 120, got %d", got)
	}
}

func TestE2EMultipleFunctions(t *testing.T) {
	code := `
double x = x + x
square x = x * x
main _ = double (square 3) + square 2 + double 1
`
	// double(9) + 4 + 2 = 18 + 4 + 2 = 24
	if got := runWasm(t, code); got != 24 {
		t.Fatalf("expected exit 24, got %d", got)
	}
}

func TestE2EFunctionWithIf(t *testing.T) {
	code := `
abs x =
    if x < 0 then
        0 - x
    else
        x
main _ = abs (-42)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EClosureCapture(t *testing.T) {
	code := `
makeAdder n = \x -> n + x
main _ =
    let add5 = makeAdder 5
    in add5 37
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EClosureTwoCaptures(t *testing.T) {
	code := `
makeAdd a b = \x -> a + b + x
main _ =
    let f = makeAdd 10 20
    in f 12
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EHigherOrderFunction(t *testing.T) {
	code := `
apply f x = f x
double x = x + x
main _ = apply double 21
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EADTEnum(t *testing.T) {
	code := `
type Color = Red | Green | Blue
colorToInt c =
    match c
        when Red -> 1
        when Green -> 2
        when Blue -> 3
main _ = colorToInt Green
`
	if got := runWasm(t, code); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
}

func TestE2EADTWithFields(t *testing.T) {
	code := `
type Shape = Circle Int | Square Int
area s =
    match s
        when Circle r -> r * r * 3
        when Square side -> side * side
main _ = area (Circle 4)
`
	if got := runWasm(t, code); got != 48 {
		t.Fatalf("expected exit 48, got %d", got)
	}
}

func TestE2EADTMaybe(t *testing.T) {
	code := `
type Maybe = Nothing | Just Int
fromMaybe d m =
    match m
        when Nothing -> d
        when Just x -> x
main _ = fromMaybe 0 (Just 42)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EADTMultiField(t *testing.T) {
	code := `
type Pair = MkPair Int Int
fst p =
    match p
        when MkPair a _ -> a
main _ = fst (MkPair 42 99)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EADTWildcardArm(t *testing.T) {
	code := `
type Color = Red | Green | Blue
isRed c =
    match c
        when Red -> 1
        when _ -> 0
main _ = isRed Green
`
	if got := runWasm(t, code); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}

func TestE2EMatchInt(t *testing.T) {
	code := `
f x =
    match x
        when 0 -> 10
        when 1 -> 20
        when _ -> 30
main _ = f 1
`
	if got := runWasm(t, code); got != 20 {
		t.Fatalf("expected exit 20, got %d", got)
	}
}

func TestE2EMatchBool(t *testing.T) {
	code := `
boolToInt b =
    match b
        when true -> 1
        when false -> 0
main _ = boolToInt true
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2ETailCall(t *testing.T) {
	// This would stack overflow without TCO (1 million iterations)
	code := `
countdown n acc =
    if n == 0 then
        acc
    else
        countdown (n - 1) (acc + 1)
main _ = countdown 1000000 0
`
	// 1000000 % 256 = 64 (exit codes are 8-bit)
	if got := runWasm(t, code); got != 64 {
		t.Fatalf("expected exit 64, got %d", got)
	}
}

func TestE2ETailCallRecursion(t *testing.T) {
	code := `
fact n acc =
    if n == 0 then
        acc
    else
        fact (n - 1) (n * acc)
main _ = fact 5 1
`
	// 5! = 120
	if got := runWasm(t, code); got != 120 {
		t.Fatalf("expected exit 120, got %d", got)
	}
}

func TestE2EMutualFunctions(t *testing.T) {
	code := `
isEven n =
    if n == 0 then
        1
    else
        isOdd (n - 1)
isOdd n =
    if n == 0 then
        0
    else
        isEven (n - 1)
main _ = isEven 10
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2EStringEquality(t *testing.T) {
	code := `
check s =
    if s == "hello" then
        42
    else
        0
main _ = check "hello"
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EStringNotEqual(t *testing.T) {
	code := `
check s =
    if s == "hello" then
        42
    else
        0
main _ = check "world"
`
	if got := runWasm(t, code); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}

func TestE2EStringMatch(t *testing.T) {
	code := `
grade s =
    match s
        when "A" ->
            4
        when "B" ->
            3
        when "C" ->
            2
        when _ ->
            0
main _ = grade "B"
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
}

func TestE2EStringInADT(t *testing.T) {
	code := `
type Greeting = Hello String | Bye

getLen g =
    match g
        when Hello _ ->
            1
        when Bye ->
            0
main _ = getLen (Hello "world")
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2EListLiteral(t *testing.T) {
	code := `
length lst =
    match lst
        when [] ->
            0
        when [_|t] ->
            1 + length t
main _ = length [10, 20, 30]
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
}

func TestE2EListSum(t *testing.T) {
	code := `
sum lst =
    match lst
        when [] ->
            0
        when [h|t] ->
            h + sum t
main _ = sum [10, 20, 12]
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EEmptyList(t *testing.T) {
	code := `
isEmpty lst =
    match lst
        when [] ->
            1
        when _ ->
            0
main _ = isEmpty []
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2ETuple(t *testing.T) {
	code := `
fst p =
    match p
        when (a, _) ->
            a
main _ = fst (42, 7)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ETupleSnd(t *testing.T) {
	code := `
snd p =
    match p
        when (_, b) ->
            b
main _ = snd (10, 32)
`
	if got := runWasm(t, code); got != 32 {
		t.Fatalf("expected exit 32, got %d", got)
	}
}

func TestE2EListTailRecSum(t *testing.T) {
	// Combine lists, pattern matching, and tail calls
	code := `
sumAcc lst acc =
    match lst
        when [] ->
            acc
        when [h|t] ->
            sumAcc t (acc + h)

main _ = sumAcc [1, 2, 3, 4, 5, 6, 7, 8, 9, 10] 0
`
	// 1+2+...+10 = 55
	if got := runWasm(t, code); got != 55 {
		t.Fatalf("expected exit 55, got %d", got)
	}
}

func TestE2EADTWithListField(t *testing.T) {
	code := `
type Pair = MkPair Int Int

add p =
    match p
        when MkPair a b ->
            a + b

main _ = add (MkPair 20 22)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Polymorphic boxing tests
// ---------------------------------------------------------------------------

func TestE2EListOfStrings(t *testing.T) {
	code := `
length lst =
    match lst
        when [] ->
            0
        when [_|t] ->
            1 + length t
main _ = length ["hello", "world", "!"]
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
}

func TestE2EStringFromList(t *testing.T) {
	// Extract a string from a list and compare it
	code := `
head lst =
    match lst
        when [] ->
            ""
        when [h|_] ->
            h
check s =
    if s == "hello" then
        42
    else
        0
main _ = check (head ["hello", "world"])
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EMixedTuple(t *testing.T) {
	// Tuple with mixed types: (Int, String)
	// Extract the Int from the first position
	code := `
fst p =
    match p
        when (a, _) ->
            a
main _ = fst (42, "hello")
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EMixedTupleSnd(t *testing.T) {
	// Extract string from second position and check it
	code := `
snd p =
    match p
        when (_, b) ->
            b
check s =
    if s == "world" then
        42
    else
        0
main _ =
    let s = snd (99, "world")
    in check s
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EClosureCaptureString(t *testing.T) {
	// Closure captures a string variable and uses it
	code := `
checkStr : String -> String -> Int
checkStr s x =
    if x == s then
        1
    else
        0
makeChecker : String -> (String -> Int)
makeChecker s = \x -> checkStr s x
main _ =
    let check = makeChecker "hello"
    in check "hello"
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2EPolymorphicADT(t *testing.T) {
	// Just with a string value
	code := `
type Maybe a = Nothing | Just a

fromJust d m =
    match m
        when Nothing ->
            d
        when Just x ->
            x

check s =
    if s == "hello" then
        42
    else
        0

main _ = check (fromJust "" (Just "hello"))
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EPolymorphicADTInt(t *testing.T) {
	// Just with an int value
	code := `
type Maybe a = Nothing | Just a

fromJust d m =
    match m
        when Nothing ->
            d
        when Just x ->
            x

main _ = fromJust 0 (Just 42)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EHigherOrderWithStrings(t *testing.T) {
	// Apply a String -> Int function via closure
	code := `
apply f x = f x
strLen s =
    if s == "hello" then
        5
    else
        0
main _ = apply strLen "hello"
`
	if got := runWasm(t, code); got != 5 {
		t.Fatalf("expected exit 5, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Trait dispatch tests
// ---------------------------------------------------------------------------

func TestE2ETraitCompiles(t *testing.T) {
	// Trait + impl compiles without crash (no trait call)
	code := `
trait Show a where
    show : a -> Int

impl Show Int where
    show n = n

main _ = 0
`
	if got := runWasm(t, code); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}

func TestE2ETraitShowInt(t *testing.T) {
	// Arity-1 trait method dispatch: show Int
	code := `
trait ToInt a where
    toInt : a -> Int

impl ToInt Int where
    toInt n = n

main _ = toInt 42
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ETraitEqInt(t *testing.T) {
	// Arity-2 trait method dispatch: eq Int
	code := `
trait Eq a where
    eq : a -> a -> Bool

impl Eq Int where
    eq x y = x == y

main _ =
    if eq 42 42 then
        1
    else
        0
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2ETraitEqIntFalse(t *testing.T) {
	// Arity-2 trait method dispatch: eq Int (not equal)
	code := `
trait Eq a where
    eq : a -> a -> Bool

impl Eq Int where
    eq x y = x == y

main _ =
    if eq 42 99 then
        1
    else
        0
`
	if got := runWasm(t, code); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}

func TestE2ETraitOrdInt(t *testing.T) {
	// Trait method returning an ADT
	code := `
type Ordering = LT | EQ | GT

trait Ord a where
    compare : a -> a -> Ordering

impl Ord Int where
    compare x y =
        if x < y then
            LT
        else if x == y then
            EQ
        else
            GT

toInt o =
    match o
        when LT ->
            0
        when EQ ->
            1
        when GT ->
            2

main _ = toInt (compare 3 5)
`
	if got := runWasm(t, code); got != 0 {
		t.Fatalf("expected exit 0 (LT), got %d", got)
	}
}

func TestE2ETraitMultipleImpls(t *testing.T) {
	// Two impls for the same trait, different types
	code := `
trait ToInt a where
    toInt : a -> Int

impl ToInt Int where
    toInt n = n

impl ToInt Bool where
    toInt b =
        if b then
            1
        else
            0

main _ = toInt true + toInt 10
`
	if got := runWasm(t, code); got != 11 {
		t.Fatalf("expected exit 11, got %d", got)
	}
}

func TestE2ETraitWithADT(t *testing.T) {
	// Trait impl for a custom ADT
	code := `
type Color = Red | Green | Blue

trait ToInt a where
    toInt : a -> Int

impl ToInt Color where
    toInt c =
        match c
            when Red ->
                1
            when Green ->
                2
            when Blue ->
                3

main _ = toInt Green
`
	if got := runWasm(t, code); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
}

func TestE2ETraitRuntimeDispatch(t *testing.T) {
	// Polymorphic function calls trait method — type only known at runtime
	code := `
trait ToInt a where
    toInt : a -> Int

impl ToInt Int where
    toInt n = n

impl ToInt Bool where
    toInt b =
        if b then
            1
        else
            0

applyToInt x = toInt x

main _ = applyToInt 42
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ETraitRuntimeDispatchBool(t *testing.T) {
	// Runtime dispatch selects Bool impl
	code := `
trait ToInt a where
    toInt : a -> Int

impl ToInt Int where
    toInt n = n

impl ToInt Bool where
    toInt b =
        if b then
            1
        else
            0

applyToInt x = toInt x

main _ = applyToInt true
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2ETraitRuntimeDispatchArity2(t *testing.T) {
	// Runtime dispatch for arity-2 trait method (eq)
	code := `
trait Eq a where
    eq : a -> a -> Bool

impl Eq Int where
    eq x y = x == y

impl Eq Bool where
    eq x y =
        if x then
            y
        else
            if y then
                false
            else
                true

areEqual x y = eq x y

main _ =
    if areEqual 10 10 then
        1
    else
        0
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Builtin tests
// ---------------------------------------------------------------------------

func TestE2EBuiltinNot(t *testing.T) {
	code := `
main _ =
    if not true then
        1
    else
        0
`
	if got := runWasm(t, code); got != 0 {
		t.Fatalf("expected exit 0, got %d", got)
	}
}

func TestE2EBuiltinNotFalse(t *testing.T) {
	code := `
main _ =
    if not false then
        1
    else
        0
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

// runWasmStdout compiles Rex source to .wasm and runs it, returning stdout and exit code.
func runWasmStdout(t *testing.T, code string) (string, int) {
	t.Helper()
	if !hasCmd("wasm-tools") {
		t.Skip("wasm-tools not found")
	}
	if !hasCmd("wasmtime") {
		t.Skip("wasmtime not found")
	}

	wat := compileCode(t, code)

	dir := t.TempDir()
	watFile := filepath.Join(dir, "main.wat")
	wasmFile := filepath.Join(dir, "main.wasm")

	if err := os.WriteFile(watFile, []byte(wat), 0644); err != nil {
		t.Fatalf("write wat: %v", err)
	}

	out, err := exec.Command("wasm-tools", "parse", watFile, "-o", wasmFile).CombinedOutput()
	if err != nil {
		t.Fatalf("wasm-tools parse: %v\n%s\nWAT:\n%s", err, out, wat)
	}

	cmd := exec.Command("wasmtime", "--wasm", "gc", "--wasm", "function-references", "--wasm", "tail-call", wasmFile)
	out, err = cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("wasmtime: %v\n%s", err, out)
		}
	}
	return string(out), exitCode
}

func TestE2EPrintlnString(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let _ = println "hello world"
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if strings.TrimRight(stdout, "\n") != "hello world" {
		t.Fatalf("expected 'hello world', got %q", stdout)
	}
}

func TestE2EPrintlnMultiple(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let _ = println "hello"
    in
    let _ = println "world"
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	expected := "hello\nworld\n"
	if stdout != expected {
		t.Fatalf("expected %q, got %q", expected, stdout)
	}
}

func TestE2EShowInt(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let _ = println (showInt 42)
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if strings.TrimRight(stdout, "\n") != "42" {
		t.Fatalf("expected '42', got %q", stdout)
	}
}

func TestE2EShowIntNegative(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let _ = println (showInt (0 - 123))
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if strings.TrimRight(stdout, "\n") != "-123" {
		t.Fatalf("expected '-123', got %q", stdout)
	}
}

func TestE2EPrintlnInt(t *testing.T) {
	// println with int arg: should convert to string and print
	code := `
import Std:IO (println)

main _ =
    let _ = println 42
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if strings.TrimRight(stdout, "\n") != "42" {
		t.Fatalf("expected '42', got %q", stdout)
	}
}

// ---------------------------------------------------------------------------
// Module import tests
// ---------------------------------------------------------------------------

func TestE2EImportMaybe(t *testing.T) {
	code := `
import Std:Maybe (Just, Nothing)

main _ =
    match Just 42
        when Just n ->
            n
        when Nothing ->
            0
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EImportMaybeNothing(t *testing.T) {
	code := `
import Std:Maybe (Just, Nothing)

main _ =
    match Nothing
        when Just n ->
            n
        when Nothing ->
            99
`
	if got := runWasm(t, code); got != 99 {
		t.Fatalf("expected exit 99, got %d", got)
	}
}

func TestE2EPrintlnBool(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let _ = println true
    in
    let _ = println false
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	expected := "true\nfalse\n"
	if stdout != expected {
		t.Fatalf("expected %q, got %q", expected, stdout)
	}
}

// ---------------------------------------------------------------------------
// String concat tests
// ---------------------------------------------------------------------------

func TestE2EStringConcat(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let _ = println ("hello" ++ " " ++ "world")
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if stdout != "hello world\n" {
		t.Fatalf("expected %q, got %q", "hello world\n", stdout)
	}
}

func TestE2EStringConcatResult(t *testing.T) {
	code := `
main _ =
    let s = "ab" ++ "cd"
    in if s == "abcd" then
        42
    else
        0
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// String interpolation tests
// ---------------------------------------------------------------------------

func TestE2EStringInterpSimple(t *testing.T) {
	code := `
main _ =
    if "hello ${"world"}" == "hello world" then
        42
    else
        0
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EStringInterpInt(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let _ = println "the answer is ${42}"
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if stdout != "the answer is 42\n" {
		t.Fatalf("expected %q, got %q", "the answer is 42\n", stdout)
	}
}

func TestE2EStringInterpVar(t *testing.T) {
	code := `
import Std:IO (println)

main _ =
    let name = "world"
    in
    let _ = println "hello ${name}!"
    in 0
`
	stdout, exitCode := runWasmStdout(t, code)
	if exitCode != 0 {
		t.Fatalf("expected exit 0, got %d", exitCode)
	}
	if stdout != "hello world!\n" {
		t.Fatalf("expected %q, got %q", "hello world!\n", stdout)
	}
}

// ---------------------------------------------------------------------------
// Higher-order function tests
// ---------------------------------------------------------------------------

func TestE2ELocalFoldl(t *testing.T) {
	code := `
foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

main _ = foldl (\acc x -> acc + x) 0 [1, 2, 3, 4, 5]
`
	// 1+2+3+4+5 = 15
	if got := runWasm(t, code); got != 15 {
		t.Fatalf("expected exit 15, got %d", got)
	}
}

func TestE2EConsOperator(t *testing.T) {
	code := `
length lst =
    match lst
        when [] ->
            0
        when [_|t] ->
            1 + length t

main _ = length (1 :: 2 :: 3 :: [])
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
}

func TestE2ELocalMap(t *testing.T) {
	code := `
mymap f lst =
    match lst
        when [] ->
            []
        when [h|t] ->
            f h :: mymap f t

length lst =
    match lst
        when [] ->
            0
        when [_|t] ->
            1 + length t

main _ = length (mymap (\x -> x + 1) [10, 20, 30])
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
}

func TestE2ELocalFilter(t *testing.T) {
	code := `
foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

myfilter f lst =
    match lst
        when [] ->
            []
        when [h|t] ->
            if f h then
                h :: myfilter f t
            else
                myfilter f t

main _ = foldl (\acc x -> acc + x) 0 (myfilter (\x -> x > 2) [1, 2, 3, 4, 5])
`
	// 3+4+5 = 12
	if got := runWasm(t, code); got != 12 {
		t.Fatalf("expected exit 12, got %d", got)
	}
}

func TestE2EFuncAsValueArity2(t *testing.T) {
	code := `
add a b = a + b

apply f x = f x

main _ = apply (add 10) 32
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EMultiArgFuncAsValue(t *testing.T) {
	code := `
add a b = a + b

mymap f lst =
    match lst
        when [] ->
            []
        when [h|t] ->
            f h :: mymap f t

foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

main _ = foldl add 0 [10, 20, 12]
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EReverse(t *testing.T) {
	code := `
foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

reverse lst = foldl (\acc x -> x :: acc) [] lst

length lst =
    match lst
        when [] ->
            0
        when [_|t] ->
            1 + length t

head lst =
    match lst
        when [h|_] ->
            h
        when [] ->
            0

main _ = head (reverse [1, 2, 3])
`
	// reverse [1,2,3] = [3,2,1], head = 3
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
}

func TestE2EAppend(t *testing.T) {
	code := `
foldr f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            f h (foldr f acc t)

append a b = foldr (\x xs -> x :: xs) b a

foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

main _ = foldl (\acc x -> acc + x) 0 (append [1, 2] [3, 4, 5])
`
	// 1+2+3+4+5 = 15
	if got := runWasm(t, code); got != 15 {
		t.Fatalf("expected exit 15, got %d", got)
	}
}

func TestE2EConcatMap(t *testing.T) {
	code := `
mymap f lst =
    match lst
        when [] ->
            []
        when [h|t] ->
            f h :: mymap f t

foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

main _ = foldl (\acc x -> acc + x) 0 (mymap (\x -> x * x) [1, 2, 3, 4])
`
	// 1+4+9+16 = 30
	if got := runWasm(t, code); got != 30 {
		t.Fatalf("expected exit 30, got %d", got)
	}
}

func TestE2ERecordCreate(t *testing.T) {
	code := `
type Point = { x : Int, y : Int }

main _ =
    let p = Point { x = 10, y = 32 }
    in p.x + p.y
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ERecordFieldAccess(t *testing.T) {
	code := `
type Pair = { fst : Int, snd : Int }

getFirst p = p.fst
getSecond p = p.snd

main _ =
    let p = Pair { fst = 30, snd = 12 }
    in getFirst p + getSecond p
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ERecordUpdate(t *testing.T) {
	code := `
type Point = { x : Int, y : Int }

main _ =
    let p = Point { x = 10, y = 20 }
    in
    let q = { p | x = 30 }
    in q.x + q.y
`
	// 30 + 20 = 50
	if got := runWasm(t, code); got != 50 {
		t.Fatalf("expected exit 50, got %d", got)
	}
}

func TestE2ERecordPositionalCtor(t *testing.T) {
	code := `
type Point = { x : Int, y : Int }

main _ =
    let p = Point 15 27
    in p.x + p.y
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ERecordInFunction(t *testing.T) {
	code := `
type Point = { x : Int, y : Int }

sum p = p.x + p.y

main _ =
    let p = Point { x = 20, y = 22 }
    in sum p
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ERecordUpdateMultiple(t *testing.T) {
	code := `
type Vec3 = { x : Int, y : Int, z : Int }

main _ =
    let v = Vec3 { x = 1, y = 2, z = 3 }
    in
    let w = { v | x = 10, z = 30 }
    in w.x + w.y + w.z
`
	// 10 + 2 + 30 = 42
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2ELetRecLocalRecursion(t *testing.T) {
	code := `
main _ =
    let rec sum n =
        if n == 0 then
            0
        else
            n + sum (n - 1)
    in sum 9
`
	// 9+8+7+6+5+4+3+2+1 = 45
	if got := runWasm(t, code); got != 45 {
		t.Fatalf("expected exit 45, got %d", got)
	}
}

func TestE2ELetRecMutualRecursion(t *testing.T) {
	code := `
main _ =
    let rec
        isEven n =
            if n == 0 then
                1
            else
                isOdd (n - 1)
        and isOdd n =
            if n == 0 then
                0
            else
                isEven (n - 1)
    in isEven 10
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected exit 1, got %d", got)
	}
}

func TestE2ELetRecWithCapture(t *testing.T) {
	code := `
main _ =
    let offset = 100
    in
    let rec go n =
        if n == 0 then
            offset
        else
            n + go (n - 1)
    in go 5
`
	// 5+4+3+2+1+100 = 115
	if got := runWasm(t, code); got != 115 {
		t.Fatalf("expected exit 115, got %d", got)
	}
}

func TestE2EListMap(t *testing.T) {
	code := `
map f lst =
    match lst
        when [] ->
            []
        when [h|t] ->
            f h :: map f t

foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

main _ = foldl (\acc x -> acc + x) 0 (map (\x -> x * 2) [1, 2, 3, 4, 5])
`
	// 2+4+6+8+10 = 30
	if got := runWasm(t, code); got != 30 {
		t.Fatalf("expected exit 30, got %d", got)
	}
}

func TestE2EListFilter(t *testing.T) {
	code := `
filter f lst =
    match lst
        when [] ->
            []
        when [h|t] ->
            if f h then
                h :: filter f t
            else
                filter f t

length lst =
    match lst
        when [] ->
            0
        when [_|t] ->
            1 + length t

main _ = length (filter (\x -> x > 3) [1, 2, 3, 4, 5, 6])
`
	// [4, 5, 6] -> length 3
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected exit 3, got %d", got)
	}
}

func TestE2EMaybeADT(t *testing.T) {
	code := `
type Maybe a = Nothing | Just a

fromMaybe default m =
    match m
        when Nothing ->
            default
        when Just v ->
            v

main _ =
    let x = Just 42
    in
    let y = Nothing
    in fromMaybe 0 x + fromMaybe 10 y
`
	// 42 + 10 = 52
	if got := runWasm(t, code); got != 52 {
		t.Fatalf("expected exit 52, got %d", got)
	}
}

func TestE2EListFoldWithADT(t *testing.T) {
	code := `
type Maybe a = Nothing | Just a

foldl f acc lst =
    match lst
        when [] ->
            acc
        when [h|t] ->
            foldl f (f acc h) t

find pred lst =
    match lst
        when [] ->
            Nothing
        when [h|t] ->
            if pred h then
                Just h
            else
                find pred t

fromMaybe default m =
    match m
        when Nothing ->
            default
        when Just v ->
            v

main _ = fromMaybe 0 (find (\x -> x > 3) [1, 2, 3, 4, 5])
`
	// finds 4
	if got := runWasm(t, code); got != 4 {
		t.Fatalf("expected exit 4, got %d", got)
	}
}

func TestE2ETopLevelMutualRecursion(t *testing.T) {
	code := `
isEven n =
    if n == 0 then
        1
    else
        isOdd (n - 1)

isOdd n =
    if n == 0 then
        0
    else
        isEven (n - 1)

main _ = isEven 10 + isOdd 7
`
	// isEven 10 = 1, isOdd 7 = 1 -> 2
	if got := runWasm(t, code); got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
}

func TestE2ENestedPatternMatch(t *testing.T) {
	code := `
type Maybe a = Nothing | Just a

matchPair p =
    match p
        when (Just x, Just y) ->
            x + y
        when (Just x, Nothing) ->
            x
        when (Nothing, Just y) ->
            y
        when (Nothing, Nothing) ->
            0

main _ = matchPair (Just 30, Just 12)
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected exit 42, got %d", got)
	}
}

func TestE2EStdlibListLength(t *testing.T) {
	code := `
import Std:List (length)

main _ = length [10, 20, 30]
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestE2EStdlibListSum(t *testing.T) {
	code := `
import Std:List (sum)

main _ = sum [10, 20, 12]
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestE2EStdlibListReverse(t *testing.T) {
	code := `
import Std:List (reverse, length)

main _ =
    let xs = reverse [1, 2, 3]
    in length xs
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestE2EStdlibListMap(t *testing.T) {
	code := `
import Std:List (map, sum)

main _ =
    [1, 2, 3] |> map (\x -> x * 10) |> sum
`
	if got := runWasm(t, code); got != 60 {
		t.Fatalf("expected 60, got %d", got)
	}
}

func TestE2EStdlibListFilter(t *testing.T) {
	code := `
import Std:List (filter, length)

main _ =
    [1, 2, 3, 4, 5, 6] |> filter (\x -> x > 3) |> length
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestE2EStdlibListFoldl(t *testing.T) {
	code := `
import Std:List (foldl)

main _ = foldl (\acc x -> acc + x) 0 [1, 2, 3, 4, 5]
`
	if got := runWasm(t, code); got != 15 {
		t.Fatalf("expected 15, got %d", got)
	}
}

func TestE2EStdlibListAppend(t *testing.T) {
	code := `
import Std:List (append, length)

main _ =
    append [1, 2, 3] [4, 5] |> length
`
	if got := runWasm(t, code); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestE2EStdlibListRange(t *testing.T) {
	code := `
import Std:List (range, sum)

main _ = range 1 11 |> sum
`
	if got := runWasm(t, code); got != 55 {
		t.Fatalf("expected 55, got %d", got)
	}
}

func TestE2EStdlibListProduct(t *testing.T) {
	code := `
import Std:List (product)

main _ = product [1, 2, 3, 4, 5]
`
	if got := runWasm(t, code); got != 120 {
		t.Fatalf("expected 120, got %d", got)
	}
}

func TestE2EStdlibListFoldr(t *testing.T) {
	code := `
import Std:List (foldr, length)

main _ =
    foldr (\x acc -> x :: acc) [] [1, 2, 3] |> length
`
	if got := runWasm(t, code); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
}

func TestE2EStdlibListConcat(t *testing.T) {
	code := `
import Std:List (concat, length)

main _ = concat [[1, 2], [3], [4, 5]] |> length
`
	if got := runWasm(t, code); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestE2EStdlibListAny(t *testing.T) {
	code := `
import Std:List (any)

main _ =
    if any (\x -> x > 3) [1, 2, 3, 4, 5] then 1 else 0
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestE2EStdlibListAll(t *testing.T) {
	code := `
import Std:List (all)

main _ =
    if all (\x -> x > 0) [1, 2, 3] then 1 else 0
`
	if got := runWasm(t, code); got != 1 {
		t.Fatalf("expected 1, got %d", got)
	}
}

func TestE2EStdlibMaybeFromMaybe(t *testing.T) {
	code := `
import Std:Maybe (Just, Nothing, fromMaybe)

main _ =
    let a = fromMaybe 0 (Just 42)
    in let b = fromMaybe 10 Nothing
    in a + b
`
	if got := runWasm(t, code); got != 52 {
		t.Fatalf("expected 52, got %d", got)
	}
}

func TestE2EStdlibMaybeMap(t *testing.T) {
	code := `
import Std:Maybe (Just, Nothing, map)

main _ =
    let r = map (\x -> x * 2) (Just 21)
    in match r
        when Just n -> n
        when Nothing -> 0
`
	if got := runWasm(t, code); got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}
