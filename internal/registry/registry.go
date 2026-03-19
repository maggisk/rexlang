// Package registry provides access to the Rex package index,
// a simple JSON file hosted on GitHub that lists known packages.
package registry

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultIndexURL is the URL of the official Rex package index.
const DefaultIndexURL = "https://raw.githubusercontent.com/maggisk/rex-packages/main/index.json"

// Package represents a single entry in the package index.
type Package struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Git         string `json:"git"`
	LatestRef   string `json:"latest_ref"`
}

// FetchIndex downloads and parses the package index from the given URL.
// If url is empty, DefaultIndexURL is used.
func FetchIndex(url string) ([]Package, error) {
	if url == "" {
		url = DefaultIndexURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("package index returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read package index: %w", err)
	}

	var packages []Package
	if err := json.Unmarshal(body, &packages); err != nil {
		return nil, fmt.Errorf("failed to parse package index: %w", err)
	}

	return packages, nil
}

// Search filters packages by a query string, matching against the name
// and description (case-insensitive).
func Search(packages []Package, query string) []Package {
	if query == "" {
		return packages
	}

	q := strings.ToLower(query)
	var results []Package
	for _, pkg := range packages {
		if strings.Contains(strings.ToLower(pkg.Name), q) ||
			strings.Contains(strings.ToLower(pkg.Description), q) {
			results = append(results, pkg)
		}
	}
	return results
}

// Lookup finds a package by exact name (case-insensitive).
func Lookup(packages []Package, name string) *Package {
	n := strings.ToLower(name)
	for _, pkg := range packages {
		if strings.ToLower(pkg.Name) == n {
			return &pkg
		}
	}
	return nil
}
