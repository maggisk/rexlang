package main

import (
	"fmt"
	"os"

	"github.com/maggisk/rexlang/internal/formatter"
)

func runFmt(args []string) {
	var checkMode bool
	var diffMode bool
	var files []string

	for _, a := range args {
		switch a {
		case "--check":
			checkMode = true
		case "--diff":
			diffMode = true
		default:
			files = append(files, a)
		}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: rex fmt [--check] [--diff] <file.rex> [file.rex ...]")
		os.Exit(1)
	}

	opts := formatter.Options{
		SortImports: true,
	}

	anyUnformatted := false

	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			os.Exit(1)
		}

		original := string(data)
		formatted := formatter.Format(original, opts)

		if original == formatted {
			continue
		}

		anyUnformatted = true

		if diffMode {
			d := formatter.Diff(path, original, formatted)
			fmt.Print(d)
		}

		if checkMode {
			fmt.Fprintf(os.Stderr, "%s needs formatting\n", path)
			continue
		}

		if !diffMode {
			if err := os.WriteFile(path, []byte(formatted), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
				os.Exit(1)
			}
			fmt.Printf("Formatted %s\n", path)
		}
	}

	if checkMode && anyUnformatted {
		os.Exit(1)
	}
}
