// Package formatter provides auto-formatting for Rex source files.
// It operates at the line level (no AST parsing) to safely normalize
// formatting while preserving comments.
package formatter

import (
	"sort"
	"strings"
)

// Format normalizes a Rex source file:
//   - Removes trailing whitespace
//   - Converts tabs to 4-space indentation
//   - Normalizes blank lines (max 2 consecutive)
//   - Sorts import blocks alphabetically
//   - Ensures file ends with a single newline
func Format(src string) string {
	lines := strings.Split(src, "\n")

	// Pass 1: clean individual lines
	for i, line := range lines {
		line = strings.TrimRight(line, " \t\r")
		line = expandTabs(line)
		lines[i] = line
	}

	// Pass 2: normalize blank lines (max 2 consecutive)
	lines = normalizeBlankLines(lines)

	// Pass 3: sort import blocks
	lines = sortImports(lines)

	result := strings.Join(lines, "\n")

	// Ensure single trailing newline
	result = strings.TrimRight(result, "\n") + "\n"

	return result
}

func expandTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	return strings.ReplaceAll(line, "\t", "    ")
}

func normalizeBlankLines(lines []string) []string {
	var result []string
	blanks := 0
	for _, line := range lines {
		if line == "" {
			blanks++
			if blanks <= 2 {
				result = append(result, line)
			}
		} else {
			blanks = 0
			result = append(result, line)
		}
	}
	return result
}

func sortImports(lines []string) []string {
	var result []string
	i := 0
	for i < len(lines) {
		if isImportLine(lines[i]) {
			// Collect contiguous import block
			var block []string
			for i < len(lines) && isImportLine(lines[i]) {
				block = append(block, lines[i])
				i++
			}
			sort.Strings(block)
			result = append(result, block...)
		} else {
			result = append(result, lines[i])
			i++
		}
	}
	return result
}

func isImportLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasPrefix(trimmed, "import ")
}
