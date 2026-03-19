package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearch(t *testing.T) {
	packages := []Package{
		{Name: "tearex", Description: "Web framework for Rex", Git: "https://github.com/maggisk/tea-rex.git", LatestRef: "v0.1.0"},
		{Name: "rexql", Description: "SQL query builder", Git: "https://github.com/example/rexql.git", LatestRef: "v0.2.0"},
		{Name: "http-client", Description: "HTTP client library", Git: "https://github.com/example/http-client.git", LatestRef: "v1.0.0"},
	}

	tests := []struct {
		query    string
		expected int
	}{
		{"", 3},
		{"rex", 2},
		{"web", 1},
		{"sql", 1},
		{"http", 1},
		{"nonexistent", 0},
		{"REX", 2},
	}

	for _, tt := range tests {
		results := Search(packages, tt.query)
		if len(results) != tt.expected {
			t.Errorf("Search(%q): got %d results, want %d", tt.query, len(results), tt.expected)
		}
	}
}

func TestLookup(t *testing.T) {
	packages := []Package{
		{Name: "tearex", Description: "Web framework", Git: "https://github.com/maggisk/tea-rex.git", LatestRef: "v0.1.0"},
		{Name: "rexql", Description: "SQL builder", Git: "https://github.com/example/rexql.git", LatestRef: "v0.2.0"},
	}

	pkg := Lookup(packages, "tearex")
	if pkg == nil {
		t.Fatal("Lookup(tearex): got nil, want package")
	}
	if pkg.Git != "https://github.com/maggisk/tea-rex.git" {
		t.Errorf("Lookup(tearex).Git = %q, want %q", pkg.Git, "https://github.com/maggisk/tea-rex.git")
	}

	pkg = Lookup(packages, "TEAREX")
	if pkg == nil {
		t.Fatal("Lookup(TEAREX): got nil, want package")
	}

	pkg = Lookup(packages, "nonexistent")
	if pkg != nil {
		t.Errorf("Lookup(nonexistent): got %v, want nil", pkg)
	}
}

func TestFetchIndex(t *testing.T) {
	packages := []Package{
		{Name: "tearex", Description: "Web framework", Git: "https://github.com/maggisk/tea-rex.git", LatestRef: "v0.1.0"},
	}
	data, _ := json.Marshal(packages)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer server.Close()

	result, err := FetchIndex(server.URL)
	if err != nil {
		t.Fatalf("FetchIndex: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("FetchIndex: got %d packages, want 1", len(result))
	}
	if result[0].Name != "tearex" {
		t.Errorf("FetchIndex: got name %q, want %q", result[0].Name, "tearex")
	}
}

func TestFetchIndexHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := FetchIndex(server.URL)
	if err == nil {
		t.Fatal("FetchIndex with 404: expected error, got nil")
	}
}

func TestFetchIndexBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := FetchIndex(server.URL)
	if err == nil {
		t.Fatal("FetchIndex with bad JSON: expected error, got nil")
	}
}
