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

// compileCode runs the full pipeline: parse → typecheck → lower → WAT.
func compileCode(t *testing.T, code string) string {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	typeEnv, _, err := typechecker.CheckProgram(exprs)
	if err != nil {
		t.Fatalf("typecheck error: %v", err)
	}
	l := ir.NewLowerer()
	prog, err := l.LowerProgram(exprs)
	if err != nil {
		t.Fatalf("lower error: %v", err)
	}
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
