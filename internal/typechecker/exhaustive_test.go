package typechecker

import (
	"strings"
	"testing"

	"github.com/maggisk/rexlang/internal/parser"
)

// typecheck parses and typechecks a program, returning any error.
func typecheck(code string) error {
	exprs, err := parser.Parse(code)
	if err != nil {
		return err
	}
	_, _, err = CheckProgram(exprs, "")
	return err
}

// expectExhaustiveError asserts that code fails with a non-exhaustive pattern error.
func expectExhaustiveError(t *testing.T, name, code string, substrs ...string) {
	t.Helper()
	err := typecheck(code)
	if err == nil {
		t.Fatalf("%s: expected non-exhaustive error, got nil", name)
	}
	msg := err.Error()
	if !strings.Contains(msg, "non-exhaustive") {
		t.Fatalf("%s: expected non-exhaustive error, got: %v", name, msg)
	}
	for _, s := range substrs {
		if !strings.Contains(msg, s) {
			t.Fatalf("%s: expected error to mention %q, got: %v", name, s, msg)
		}
	}
}

// expectOK asserts that code typechecks without error.
func expectOK(t *testing.T, name, code string) {
	t.Helper()
	err := typecheck(code)
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", name, err)
	}
}

// --- Bool patterns ---

func TestExhaustive_BoolComplete(t *testing.T) {
	expectOK(t, "bool complete", `
f b =
    match b
        when true ->
            1
        when false ->
            0
`)
}

func TestExhaustive_BoolMissingTrue(t *testing.T) {
	expectExhaustiveError(t, "bool missing true", `
f b =
    match b
        when false ->
            0
`, "true")
}

func TestExhaustive_BoolMissingFalse(t *testing.T) {
	expectExhaustiveError(t, "bool missing false", `
f b =
    match b
        when true ->
            1
`, "false")
}

// --- List patterns ---

func TestExhaustive_ListComplete(t *testing.T) {
	expectOK(t, "list complete", `
f xs =
    match xs
        when [] ->
            0
        when [h|t] ->
            1
`)
}

func TestExhaustive_ListMissingNil(t *testing.T) {
	expectExhaustiveError(t, "list missing nil", `
f xs =
    match xs
        when [h|t] ->
            1
`, "[]")
}

func TestExhaustive_ListMissingCons(t *testing.T) {
	expectExhaustiveError(t, "list missing cons", `
f xs =
    match xs
        when [] ->
            0
`, "[h|t]")
}

// --- ADT patterns ---

func TestExhaustive_ADTComplete(t *testing.T) {
	expectOK(t, "ADT complete", `
type Color = Red | Green | Blue

f c =
    match c
        when Red ->
            "r"
        when Green ->
            "g"
        when Blue ->
            "b"
`)
}

func TestExhaustive_ADTMissingOne(t *testing.T) {
	expectExhaustiveError(t, "ADT missing one", `
type Color = Red | Green | Blue

f c =
    match c
        when Red ->
            "r"
        when Green ->
            "g"
`, "Blue")
}

func TestExhaustive_ADTMissingTwo(t *testing.T) {
	expectExhaustiveError(t, "ADT missing two", `
type Color = Red | Green | Blue

f c =
    match c
        when Red ->
            "r"
`, "Green", "Blue")
}

func TestExhaustive_ADTWithArgs(t *testing.T) {
	expectOK(t, "ADT with args", `
type Shape = Circle Float | Rect Float Float

area s =
    match s
        when Circle r ->
            r
        when Rect w h ->
            w
`)
}

// --- Wildcard / catch-all ---

func TestExhaustive_WildcardAlone(t *testing.T) {
	expectOK(t, "wildcard alone", `
f x =
    match x
        when _ ->
            0
`)
}

func TestExhaustive_VarAlone(t *testing.T) {
	expectOK(t, "var alone", `
f x =
    match x
        when y ->
            y
`)
}

func TestExhaustive_WildcardAfterPartial(t *testing.T) {
	expectOK(t, "wildcard after partial", `
type Color = Red | Green | Blue

f c =
    match c
        when Red ->
            "r"
        when _ ->
            "other"
`)
}

// --- Literal patterns ---

func TestExhaustive_IntLiteralNeedsCatchAll(t *testing.T) {
	expectExhaustiveError(t, "int literal needs catch-all", `
f n =
    match n
        when 0 ->
            "zero"
        when 1 ->
            "one"
`)
}

func TestExhaustive_StringLiteralNeedsCatchAll(t *testing.T) {
	expectExhaustiveError(t, "string literal needs catch-all", `
f s =
    match s
        when "hello" ->
            1
`)
}

func TestExhaustive_IntWithCatchAll(t *testing.T) {
	expectOK(t, "int with catch-all", `
f n =
    match n
        when 0 ->
            "zero"
        when _ ->
            "other"
`)
}

// --- Tuple patterns (cross-column, Maranget) ---

func TestExhaustive_TupleBoolComplete(t *testing.T) {
	expectOK(t, "tuple bool complete", `
f pair =
    match pair
        when (true, true) ->
            1
        when (true, false) ->
            2
        when (false, true) ->
            3
        when (false, false) ->
            4
`)
}

func TestExhaustive_TupleBoolCrossColumnGap(t *testing.T) {
	// Each column individually has {true, false}, but
	// (true, true) and (false, false) are not covered.
	expectExhaustiveError(t, "tuple bool cross-column gap", `
f pair =
    match pair
        when (true, false) ->
            1
        when (false, true) ->
            2
`)
}

func TestExhaustive_TupleWithWildcard(t *testing.T) {
	expectOK(t, "tuple with wildcard", `
f pair =
    match pair
        when (true, _) ->
            1
        when (false, _) ->
            2
`)
}

func TestExhaustive_TupleADTCrossColumn(t *testing.T) {
	expectExhaustiveError(t, "tuple ADT cross-column gap", `
import Std:Maybe (Just, Nothing)

f pair =
    match pair
        when (Just _, Nothing) ->
            1
        when (Nothing, Just _) ->
            2
`)
}

func TestExhaustive_TupleADTComplete(t *testing.T) {
	expectOK(t, "tuple ADT complete", `
import Std:Maybe (Just, Nothing)

f pair =
    match pair
        when (Just _, Just _) ->
            1
        when (Just _, Nothing) ->
            2
        when (Nothing, Just _) ->
            3
        when (Nothing, Nothing) ->
            4
`)
}

func TestExhaustive_TupleADTWithCatchAll(t *testing.T) {
	expectOK(t, "tuple ADT with catch-all", `
import Std:Maybe (Just, Nothing)

f pair =
    match pair
        when (Just _, Nothing) ->
            1
        when (Nothing, Just _) ->
            2
        when _ ->
            3
`)
}

func TestExhaustive_TupleListCrossColumn(t *testing.T) {
	// []/cons × []/cons needs all four combos or a catch-all
	expectExhaustiveError(t, "tuple list cross-column gap", `
f pair =
    match pair
        when ([], [h|t]) ->
            1
        when ([h|t], []) ->
            2
`)
}

func TestExhaustive_TupleListComplete(t *testing.T) {
	expectOK(t, "tuple list complete", `
f pair =
    match pair
        when ([], []) ->
            0
        when ([], [h|t]) ->
            1
        when ([h|t], []) ->
            2
        when ([h|t], [h2|t2]) ->
            3
`)
}

// --- Nested patterns ---

func TestExhaustive_NestedADTInTuple(t *testing.T) {
	expectOK(t, "nested ADT in tuple", `
import Std:Maybe (Just, Nothing)

f pair =
    match pair
        when (Just (Just _), _) ->
            1
        when (Just Nothing, _) ->
            2
        when (Nothing, _) ->
            3
`)
}

func TestExhaustive_NestedADTInTupleMissing(t *testing.T) {
	// Missing (Just Nothing, _)
	expectExhaustiveError(t, "nested ADT in tuple missing", `
import Std:Maybe (Just, Nothing)

f pair =
    match pair
        when (Just (Just _), _) ->
            1
        when (Nothing, _) ->
            2
`)
}

func TestExhaustive_NestedListInADT(t *testing.T) {
	expectOK(t, "nested list in ADT", `
import Std:Maybe (Just, Nothing)

f x =
    match x
        when Just [] ->
            0
        when Just [h|t] ->
            1
        when Nothing ->
            2
`)
}

func TestExhaustive_NestedListInADTMissing(t *testing.T) {
	expectExhaustiveError(t, "nested list in ADT missing", `
import Std:Maybe (Just, Nothing)

f x =
    match x
        when Just [] ->
            0
        when Nothing ->
            1
`)
}

// --- Three-element tuples ---

func TestExhaustive_TripleBoolComplete(t *testing.T) {
	expectOK(t, "triple bool with catch-all", `
f triple =
    match triple
        when (true, true, true) ->
            1
        when _ ->
            0
`)
}

func TestExhaustive_TripleBoolMissing(t *testing.T) {
	// 2^3 = 8 combos, only 2 covered
	expectExhaustiveError(t, "triple bool missing", `
f triple =
    match triple
        when (true, true, true) ->
            1
        when (false, false, false) ->
            0
`)
}

// --- Unit and record patterns ---

func TestExhaustive_UnitPattern(t *testing.T) {
	expectOK(t, "unit pattern", `
f u =
    match u
        when () ->
            0
`)
}

func TestExhaustive_RecordPattern(t *testing.T) {
	expectOK(t, "record pattern", `
type Point = { x : Int, y : Int }

f p =
    match p
        when Point { x = x } ->
            x
`)
}
