// Package registry provides a simple git-based package registry for Rex.
// The registry is a JSON index file hosted on GitHub.
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const DefaultIndexURL = "https://raw.githubusercontent.com/maggisk/rex-packages/main/index.json"

// Package represents an entry in the package registry.
type Package struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Git         string `json:"git"`
	LatestRef   string `json:"latest_ref"`
}

// FetchIndex downloads and parses the package index.
func FetchIndex(url string) ([]Package, error) {
	if url == "" {
		url = DefaultIndexURL
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading registry: %w", err)
	}
	var packages []Package
	if err := json.Unmarshal(body, &packages); err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}
	return packages, nil
}

// Search filters packages by query (case-insensitive match on name or description).
func Search(packages []Package, query string) []Package {
	q := strings.ToLower(query)
	var results []Package
	for _, p := range packages {
		if strings.Contains(strings.ToLower(p.Name), q) ||
			strings.Contains(strings.ToLower(p.Description), q) {
			results = append(results, p)
		}
	}
	return results
}

// Lookup finds a package by exact name (case-insensitive).
func Lookup(packages []Package, name string) *Package {
	n := strings.ToLower(name)
	for _, p := range packages {
		if strings.ToLower(p.Name) == n {
			return &p
		}
	}
	return nil
}
