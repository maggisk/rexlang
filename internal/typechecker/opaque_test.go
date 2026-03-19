package typechecker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maggisk/rexlang/internal/ast"
	"github.com/maggisk/rexlang/internal/parser"
)

// setupUserModule creates a temp src/ directory with the given module files,
// sets the typechecker and eval src roots, and returns a cleanup function.
func setupUserModule(t *testing.T, modules map[string]string) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "rexlang-opaque-test")
	if err != nil {
		t.Fatal(err)
	}
	srcDir := filepath.Join(dir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, code := range modules {
		path := filepath.Join(srcDir, name+".rex")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(code), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return srcDir, func() {
		os.RemoveAll(dir)
	}
}

func TestOpaqueTypeBlocksConstructor(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Email": `
export opaque type Email = Email String

export
make : String -> Email
make s = Email s

export
toString : Email -> String
toString e =
    match e
        when Email s ->
            s
`,
	})
	defer cleanup()

	// Importing the module and using the smart constructor should work
	code := `
import Email (make, toString)
x = make "test@example.com"
y = toString x
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err != nil {
		t.Fatalf("expected OK, got: %v", err)
	}
}

func TestOpaqueTypeBlocksDirectConstruction(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Email": `
export opaque type Email = Email String

export
make : String -> Email
make s = Email s
`,
	})
	defer cleanup()

	// Trying to use the constructor directly should fail
	code := `
import Email (make)
x = Email "test@example.com"
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err == nil {
		t.Fatal("expected type error when using opaque constructor, got nil")
	}
}

func TestOpaqueTypeBlocksPatternMatch(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Email": `
export opaque type Email = Email String

export
make : String -> Email
make s = Email s
`,
	})
	defer cleanup()

	// Trying to pattern match on opaque constructor should fail
	code := `
import Email (make)
f e =
    match e
        when Email s ->
            s
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err == nil {
		t.Fatal("expected type error when pattern matching on opaque type, got nil")
	}
}

func TestOpaqueTypeInAnnotation(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Email": `
export opaque type Email = Email String

export
make : String -> Email
make s = Email s

export
toString : Email -> String
toString e =
    match e
        when Email s ->
            s
`,
	})
	defer cleanup()

	// The opaque type name should be usable in annotations
	code := `
import Email (make, toString)

process : Email -> String
process e = toString e
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err != nil {
		t.Fatalf("expected OK with opaque type annotation, got: %v", err)
	}
}

func TestOpaqueTypeCannotImportConstructor(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Email": `
export opaque type Email = Email String

export
make : String -> Email
make s = Email s
`,
	})
	defer cleanup()

	// Trying to import the constructor by name should fail
	code := `
import Email (Email)
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err == nil {
		t.Fatal("expected error importing opaque constructor, got nil")
	}
	if !strings.Contains(err.Error(), "not exported") {
		t.Fatalf("expected 'not exported' error, got: %v", err)
	}
}

func TestOpaqueRecordBlocksFieldAccess(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Token": `
export opaque type Token = { value : String, kind : Int }

export
make : String -> Int -> Token
make v k = Token { value = v, kind = k }

export
getValue : Token -> String
getValue t = t.value
`,
	})
	defer cleanup()

	// Using smart constructor and accessor should work
	code := `
import Token (make, getValue)
x = make "hello" 1
y = getValue x
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err != nil {
		t.Fatalf("expected OK, got: %v", err)
	}
}

func TestOpaqueRecordBlocksDirectFieldAccess(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Token": `
export opaque type Token = { value : String, kind : Int }

export
make : String -> Int -> Token
make v k = Token { value = v, kind = k }
`,
	})
	defer cleanup()

	// Direct field access on opaque record should fail
	code := `
import Token (make)
x = make "hello" 1
y = x.value
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err == nil {
		t.Fatal("expected error accessing opaque record field, got nil")
	}
}

func TestOpaqueADTMultipleConstructors(t *testing.T) {
	resetModuleCache()
	srcRoot, cleanup := setupUserModule(t, map[string]string{
		"Color": `
export opaque type Color = Red | Green | Blue

export
red = Red

export
green = Green

export
blue = Blue

export
name : Color -> String
name c =
    match c
        when Red ->
            "red"
        when Green ->
            "green"
        when Blue ->
            "blue"
`,
	})
	defer cleanup()

	// Using exported functions should work
	code := `
import Color (red, green, blue, name)
x = name red
y = name green
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	_, _, err = CheckProgram(exprs, srcRoot)
	if err != nil {
		t.Fatalf("expected OK, got: %v", err)
	}
}

func TestOpaqueParseBasic(t *testing.T) {
	code := `
export opaque type Email = Email String
`
	exprs, err := parser.Parse(code)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	found := false
	for _, e := range exprs {
		if td, ok := e.(ast.TypeDecl); ok {
			if td.Name == "Email" && td.Exported && td.Opaque {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("expected TypeDecl with Exported=true, Opaque=true")
	}
}
