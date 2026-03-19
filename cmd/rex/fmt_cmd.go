package main

import (
	"fmt"
	"os"

	"github.com/maggisk/rexlang/internal/formatter"
)

func runFmt(args []string) {
	check := false
	diff := false
	var files []string
	for _, a := range args {
		switch a {
		case "--check":
			check = true
		case "--diff":
			diff = true
		default:
			files = append(files, a)
		}
	}

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: rex fmt [--check] [--diff] <file.rex> [file.rex ...]")
		os.Exit(1)
	}

	anyUnformatted := false
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
			os.Exit(1)
		}

		original := string(src)
		formatted := formatter.Format(original)

		if original == formatted {
			continue
		}

		anyUnformatted = true

		if check {
			fmt.Fprintf(os.Stderr, "%s: not formatted\n", path)
			continue
		}

		if diff {
			fmt.Printf("--- %s\n+++ %s (formatted)\n", path, path)
			showDiff(original, formatted)
			continue
		}

		// Write formatted output back to file
		if err := os.WriteFile(path, []byte(formatted), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("formatted %s\n", path)
	}

	if check && anyUnformatted {
		os.Exit(1)
	}
}

func showDiff(a, b string) {
	aLines := splitLines(a)
	bLines := splitLines(b)
	maxLen := len(aLines)
	if len(bLines) > maxLen {
		maxLen = len(bLines)
	}
	for i := 0; i < maxLen; i++ {
		aLine := ""
		bLine := ""
		if i < len(aLines) {
			aLine = aLines[i]
		}
		if i < len(bLines) {
			bLine = bLines[i]
		}
		if aLine != bLine {
			if aLine != "" {
				fmt.Printf("-%s\n", aLine)
			}
			if bLine != "" {
				fmt.Printf("+%s\n", bLine)
			}
		}
	}
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := make([]string, 0)
	for s != "" {
		idx := 0
		for idx < len(s) && s[idx] != '\n' {
			idx++
		}
		lines = append(lines, s[:idx])
		if idx < len(s) {
			s = s[idx+1:]
		} else {
			break
		}
	}
	return lines
}
