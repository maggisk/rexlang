package main

import (
	"fmt"
	"os"

	"github.com/maggisk/rexlang/internal/registry"
)

func runSearch(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: rex search <query>")
		os.Exit(1)
	}

	packages, err := registry.FetchIndex("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	results := registry.Search(packages, args[0])
	if len(results) == 0 {
		fmt.Println("No packages found.")
		return
	}

	for _, p := range results {
		fmt.Printf("  %s — %s\n", p.Name, p.Description)
		fmt.Printf("    git: %s  ref: %s\n\n", p.Git, p.LatestRef)
	}
}

// installFromRegistry looks up a package by name in the registry and installs it.
// Returns true if the package was found and installed.
func installFromRegistry(projectRoot, name string) bool {
	packages, err := registry.FetchIndex("")
	if err != nil {
		return false
	}

	pkg := registry.Lookup(packages, name)
	if pkg == nil {
		return false
	}

	fmt.Printf("Found %s in registry (%s@%s)\n", pkg.Name, pkg.Git, pkg.LatestRef)
	addAndInstallDep(projectRoot, pkg.Git, pkg.LatestRef)
	return true
}
