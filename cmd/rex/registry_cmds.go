package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/maggisk/rexlang/internal/registry"
)

// isGitURL returns true if s looks like a git URL rather than a package name.
func isGitURL(s string) bool {
	return strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "git@") || strings.HasSuffix(s, ".git")
}

// installFromRegistry looks up a package by name in the package index,
// then installs it as a git dependency.
func installFromRegistry(projectRoot, name string) {
	fmt.Printf("Looking up '%s' in package registry...\n", name)
	packages, err := registry.FetchIndex("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintln(os.Stderr, "You can install directly with: rex install <git-url> <ref>")
		os.Exit(1)
	}

	pkg := registry.Lookup(packages, name)
	if pkg == nil {
		fmt.Fprintf(os.Stderr, "Error: package '%s' not found in registry\n", name)
		fmt.Fprintln(os.Stderr, "Search for packages with: rex search <query>")
		fmt.Fprintln(os.Stderr, "Or install directly with:  rex install <git-url> <ref>")
		os.Exit(1)
	}

	fmt.Printf("Found %s (%s) at %s\n", pkg.Name, pkg.LatestRef, pkg.Git)
	addAndInstallDep(projectRoot, pkg.Git, pkg.LatestRef)
}

// runSearch implements the `rex search <query>` command.
func runSearch(args []string) {
	query := ""
	if len(args) >= 1 {
		query = strings.Join(args, " ")
	}

	fmt.Println("Fetching package index...")
	packages, err := registry.FetchIndex("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	results := registry.Search(packages, query)
	if len(results) == 0 {
		if query == "" {
			fmt.Println("No packages in the registry yet.")
		} else {
			fmt.Printf("No packages matching '%s'.\n", query)
		}
		return
	}

	if query != "" {
		fmt.Printf("Found %d package(s) matching '%s':\n\n", len(results), query)
	} else {
		fmt.Printf("%d package(s) available:\n\n", len(results))
	}
	for _, pkg := range results {
		fmt.Printf("  %s (%s)\n", pkg.Name, pkg.LatestRef)
		if pkg.Description != "" {
			fmt.Printf("    %s\n", pkg.Description)
		}
		fmt.Printf("    %s\n\n", pkg.Git)
	}
}
