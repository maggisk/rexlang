package stdlib

import (
	"strings"
	"testing"
)

func TestSourceForTargetNative(t *testing.T) {
	// Native target should return the same as Source
	src, err := SourceForTarget("List", "native")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	baseSrc, err := Source("List")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src != baseSrc {
		t.Fatal("SourceForTarget with native should match Source")
	}
}

func TestSourceForTargetEmpty(t *testing.T) {
	// Empty target should return the same as Source
	src, err := SourceForTarget("List", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	baseSrc, err := Source("List")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src != baseSrc {
		t.Fatal("SourceForTarget with empty target should match Source")
	}
}

func TestSourceForTargetNoOverlay(t *testing.T) {
	// Browser target with no overlay should return base
	src, err := SourceForTarget("List", "browser")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	baseSrc, err := Source("List")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src != baseSrc {
		t.Fatal("SourceForTarget with no overlay should match Source")
	}
}

func TestSourceForTargetUnknownModule(t *testing.T) {
	_, err := SourceForTarget("NonExistent", "browser")
	if err == nil {
		t.Fatal("expected error for unknown module")
	}
	if !strings.Contains(err.Error(), "unknown stdlib module") {
		t.Fatalf("unexpected error: %v", err)
	}
}
