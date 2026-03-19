package typechecker

import (
	"strings"
	"testing"

	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/types"
)

func expectConstraintError(t *testing.T, name, code string, substrs ...string) {
	t.Helper()
	err := typecheck(code)
	if err == nil {
		t.Fatalf("%s: expected type error, got nil", name)
	}
	msg := err.Error()
	for _, s := range substrs {
		if !strings.Contains(msg, s) {
			t.Fatalf("%s: expected error to mention %q, got: %v", name, s, msg)
		}
	}
}

func getScheme(t *testing.T, code, name string) types.Scheme {
	t.Helper()
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	env, _, err := CheckProgram(exprs)
	if err != nil {
		t.Fatalf("typecheck error: %v", err)
	}
	v, ok := env[name]
	if !ok {
		t.Fatalf("name %q not found in env", name)
	}
	s, ok := v.(types.Scheme)
	if !ok {
		t.Fatalf("name %q is not a Scheme: %T", name, v)
	}
	return s
}

func TestConstraintSortConcreteOK(t *testing.T) {
	// sort [1, 2, 3] should type-check (Int has Ord)
	code := `
import Std:List (sort)

test "sort ints" =
    assert sort [1, 2, 3] == [1, 2, 3]
`
	expectOK(t, "sort_ints", code)
}

func TestConstraintSortFunctionFails(t *testing.T) {
	// sort [(\x -> x)] should fail — no Ord for functions
	resetModuleCache()
	code := `
import Std:List (sort)
f = sort [(\x -> x)]
`
	expectConstraintError(t, "sort_functions", code, "no Ord instance")
}

func TestConstraintPropagation(t *testing.T) {
	// f lst = sort lst should infer Ord a => List a -> List a
	code := `
import Std:List (sort)
f lst = sort lst
`
	s := getScheme(t, code, "f")
	str := types.SchemeToString(s)
	if !strings.Contains(str, "Ord") {
		t.Fatalf("expected Ord constraint, got: %s", str)
	}
	if !strings.Contains(str, "=>") {
		t.Fatalf("expected => in scheme, got: %s", str)
	}
}

func TestConstraintEqOnFunctionsOK(t *testing.T) {
	// == uses structural equality (built-in), not Eq trait dispatch
	// So == on any type is allowed at compile time
	code := `
f = 1 == 2
`
	expectOK(t, "eq_ok", code)
}

func TestConstraintCompareConcreteOK(t *testing.T) {
	// compare 1 2 should type-check (Int has Ord)
	code := `
f = compare 1 2
`
	expectOK(t, "compare_ints", code)
}

func TestConstraintAnnotationWithConstraint(t *testing.T) {
	// Annotation with constraint should parse and check
	code := `
import Std:List (sort)
f : Ord a => [a] -> [a]
f lst = sort lst
`
	s := getScheme(t, code, "f")
	str := types.SchemeToString(s)
	if !strings.Contains(str, "Ord") {
		t.Fatalf("expected Ord constraint in annotation, got: %s", str)
	}
}

func TestConstraintAnnotationWithoutConstraint(t *testing.T) {
	// Annotation without constraint should still work (less specific is OK)
	code := `
import Std:List (sort)
f : [a] -> [a]
f lst = sort lst
`
	expectOK(t, "annotation_no_constraint", code)
}

func TestConstraintMultipleConstraints(t *testing.T) {
	// Function using both show and compare should get both Show and Ord constraints
	code := `
f x y = if compare x y == LT then show x else show y
`
	s := getScheme(t, code, "f")
	str := types.SchemeToString(s)
	if !strings.Contains(str, "Ord") {
		t.Fatalf("expected Ord constraint, got: %s", str)
	}
	if !strings.Contains(str, "Show") {
		t.Fatalf("expected Show constraint, got: %s", str)
	}
}

func TestConstraintAnnotationMultiple(t *testing.T) {
	// Parse multiple constraints in annotation
	code := `
f : (Ord a, Show a) => a -> a -> String
f x y = if compare x y == LT then show x else show y
`
	s := getScheme(t, code, "f")
	str := types.SchemeToString(s)
	if !strings.Contains(str, "Ord") || !strings.Contains(str, "Show") {
		t.Fatalf("expected Ord and Show constraints, got: %s", str)
	}
}

func TestConstraintOrdOperatorsOK(t *testing.T) {
	// < uses built-in comparison, not Ord trait dispatch
	// So < doesn't generate Ord constraints
	code := `
f = 1 < 2
`
	expectOK(t, "ord_ok", code)
}

func TestConstraintConcreteEqOK(t *testing.T) {
	// == on concrete types should work fine
	code := `
f = 1 == 2
g = "hello" == "world"
h = true == false
`
	expectOK(t, "concrete_eq", code)
}

func TestConstraintSchemeToString(t *testing.T) {
	s := types.Scheme{
		Vars:        []string{"a"},
		Constraints: []types.Constraint{{Trait: "Ord", Var: "a"}},
		Ty:          types.TFun(types.TList(types.TVar{Name: "a"}), types.TList(types.TVar{Name: "a"})),
	}
	str := types.SchemeToString(s)
	expected := "Ord a => [a] -> [a]"
	if str != expected {
		t.Fatalf("expected %q, got %q", expected, str)
	}
}

func TestConstraintSchemeToStringMultiple(t *testing.T) {
	s := types.Scheme{
		Vars:        []string{"a"},
		Constraints: []types.Constraint{{Trait: "Eq", Var: "a"}, {Trait: "Show", Var: "a"}},
		Ty:          types.TFun(types.TVar{Name: "a"}, types.TString),
	}
	str := types.SchemeToString(s)
	expected := "(Eq a, Show a) => a -> String"
	if str != expected {
		t.Fatalf("expected %q, got %q", expected, str)
	}
}

func TestConstraintSchemeToStringNoConstraints(t *testing.T) {
	s := types.Scheme{
		Vars: []string{"a"},
		Ty:   types.TFun(types.TVar{Name: "a"}, types.TVar{Name: "a"}),
	}
	str := types.SchemeToString(s)
	expected := "a -> a"
	if str != expected {
		t.Fatalf("expected %q, got %q", expected, str)
	}
}

func TestConstraintSortModuleScheme(t *testing.T) {
	// Verify sort carries Ord constraint after module loading
	resetModuleCache()

	result, err := CheckModule("Std:List")
	if err != nil {
		t.Fatalf("module error: %v", err)
	}
	if v, ok := result.Env["sort"]; ok {
		if s, ok := v.(types.Scheme); ok {
			str := types.SchemeToString(s)
			if !strings.Contains(str, "Ord") {
				t.Fatalf("expected Ord constraint on sort, got: %s", str)
			}
		}
	} else {
		t.Fatal("sort not found in module env")
	}
}

// ---------------------------------------------------------------------------
// Nested trait constraint propagation
// ---------------------------------------------------------------------------

func TestNestedConstraintEqListFunctionFails(t *testing.T) {
	// eq on a list of functions should fail -- functions do not have Eq
	resetModuleCache()
	code := `
f = eq [(\x -> x)] [(\y -> y)]
`
	expectConstraintError(t, "eq_list_function", code, "no Eq instance", "->")
}

func TestNestedConstraintSortNestedListOK(t *testing.T) {
	// sort [[1, 2], [3, 4]] should work -- List Int has Ord because Int has Ord
	resetModuleCache()
	code := `
import Std:List (sort)

test "sort nested lists" =
    assert sort [[3, 4], [1, 2]] == [[1, 2], [3, 4]]
`
	expectOK(t, "sort_nested_lists", code)
}

func TestNestedConstraintPropagateToVar(t *testing.T) {
	// A function that compares lists should propagate Eq to the element type
	code := `
f x = eq [x] [x]
`
	s := getScheme(t, code, "f")
	str := types.SchemeToString(s)
	if !strings.Contains(str, "Eq") {
		t.Fatalf("expected Eq constraint on element type, got: %s", str)
	}
	if !strings.Contains(str, "=>") {
		t.Fatalf("expected => in scheme, got: %s", str)
	}
}

func TestNestedConstraintEqTupleFunctionFails(t *testing.T) {
	// eq on tuple containing function should fail
	resetModuleCache()
	code := `
f = eq (1, (\x -> x)) (2, (\y -> y))
`
	expectConstraintError(t, "eq_tuple_function", code, "no Eq instance", "->")
}

func TestNestedConstraintShowListOK(t *testing.T) {
	// show [1, 2, 3] should work -- Int has Show
	code := `
f = show [1, 2, 3]
`
	expectOK(t, "show_list_int", code)
}

func TestNestedConstraintShowListFunctionFails(t *testing.T) {
	// show [(\x -> x)] should fail -- functions do not have Show
	resetModuleCache()
	code := `
f = show [(\x -> x)]
`
	expectConstraintError(t, "show_list_function", code, "no Show instance", "->")
}

func TestNestedConstraintDeepNesting(t *testing.T) {
	// eq on List (List Int) should work -- Int has Eq
	code := `
f = eq [[1]] [[2]]
`
	expectOK(t, "eq_nested_list", code)
}

func TestNestedConstraintDeepNestingFails(t *testing.T) {
	// eq on List (List (Int -> Int)) should fail
	resetModuleCache()
	code := `
f = eq [[(\x -> x)]] [[(\y -> y)]]
`
	expectConstraintError(t, "eq_deep_nested_function", code, "no Eq instance", "->")
}

func TestNestedConstraintMaybeOK(t *testing.T) {
	// eq on Maybe Int should work
	code := `
import Std:Maybe (Just, Nothing)
f = eq (Just 1) (Just 2)
`
	expectOK(t, "eq_maybe_int", code)
}

func TestNestedConstraintMaybeFunctionFails(t *testing.T) {
	// eq on Maybe (Int -> Int) should fail
	resetModuleCache()
	code := `
import Std:Maybe (Just, Nothing)
f = eq (Just (\x -> x)) (Just (\y -> y))
`
	expectConstraintError(t, "eq_maybe_function", code, "no Eq instance", "->")
}

func TestNestedConstraintSortListFunctionFails(t *testing.T) {
	// sort on list of functions should fail -- no Ord for functions
	resetModuleCache()
	code := `
import Std:List (sort)
f = sort [(\x -> x)]
`
	expectConstraintError(t, "sort_list_function", code, "no Ord instance", "->")
}

// ---------------------------------------------------------------------------
// Record update on TVar
// ---------------------------------------------------------------------------

func TestRecordUpdateOnTVar(t *testing.T) {
	// Record update should work when the record type is inferred from field names
	code := `
type Point = { x : Int, y : Int }

setX n p = { p | x = n }

test "record update on TVar" =
    let p = Point { x = 1, y = 2 }
    let p2 = setX 10 p
    assert p2.x == 10
    assert p2.y == 2
`
	if err := typecheck(code); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRecordUpdateOnTVarMultipleFields(t *testing.T) {
	code := `
type Person = { name : String, age : Int }

updateName n p = { p | name = n }

test "update" =
    let p = Person { name = "Alice", age = 30 }
    let p2 = updateName "Bob" p
    assert p2.name == "Bob"
`
	if err := typecheck(code); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestRecordUpdateOnTVarAmbiguous(t *testing.T) {
	// If two record types share a field name, update should report ambiguity
	code := `
type A = { shared : Int, a : Int }
type B = { shared : Int, b : Int }

setShared n r = { r | shared = n }
`
	err := typecheck(code)
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous error, got: %v", err)
	}
}

func resetModuleCache() {
	moduleCacheMu.Lock()
	moduleCache = map[string]*ModuleResult{}
	moduleCacheMu.Unlock()
	preludeTCMu.Lock()
	preludeTCCache = nil
	preludeTCMu.Unlock()
}
