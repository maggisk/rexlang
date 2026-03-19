package formatter

import (
	"sort"
	"strings"
)

type Options struct {
	SortImports bool
}

func Format(source string, opts Options) string {
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		lines[i] = convertTabs(strings.TrimRight(line, " \t\r"))
	}
	if opts.SortImports {
		lines = sortImportBlocks(lines)
	}
	lines = normalizeBlankLines(lines)
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n") + "\n"
}

func convertTabs(line string) string {
	if !strings.Contains(line, "\t") {
		return line
	}
	return strings.ReplaceAll(line, "\t", "    ")
}

func isBlank(line string) bool { return strings.TrimSpace(line) == "" }

func isSectionHeader(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "-- #")
}

func isImportLine(line string) bool {
	return strings.HasPrefix(strings.TrimSpace(line), "import ")
}

func isTopLevel(line string) bool {
	return !isBlank(line) && len(line) > 0 && line[0] != ' ' && line[0] != '\t'
}

func isTopLevelStart(line string) bool {
	if !isTopLevel(line) {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if isSectionHeader(line) || strings.HasPrefix(trimmed, "-- |") || strings.HasPrefix(trimmed, "--") {
		return true
	}
	keywords := []string{"type ", "let ", "rec ", "export", "test ", "import ", "external ", "trait ", "impl ", "alias ", "opaque "}
	for _, kw := range keywords {
		if strings.HasPrefix(trimmed, kw) || trimmed == strings.TrimSpace(kw) {
			return true
		}
	}
	if len(trimmed) > 0 && trimmed[0] >= 'a' && trimmed[0] <= 'z' {
		return true
	}
	return false
}

func normalizeBlankLines(lines []string) []string {
	if len(lines) == 0 {
		return lines
	}
	var result []string
	for i := 0; i < len(lines); i++ {
		if isBlank(lines[i]) {
			blankCount := 0
			for i < len(lines) && isBlank(lines[i]) {
				blankCount++
				i++
			}
			if i >= len(lines) {
				break
			}
			nextLine := lines[i]
			if len(result) == 0 {
				result = append(result, nextLine)
				continue
			}
			if isSectionHeader(nextLine) {
				result = append(result, "", "")
			} else if isTopLevelStart(nextLine) && !isIndented(nextLine) {
				if isSectionHeaderBlock(result) {
					result = append(result, "", "")
				} else {
					result = append(result, "")
				}
			} else if isIndented(nextLine) {
				if blankCount >= 1 {
					result = append(result, "")
				}
			} else {
				for j := 0; j < blankCount && j < 2; j++ {
					result = append(result, "")
				}
			}
			result = append(result, nextLine)
			continue
		}
		result = append(result, lines[i])
	}
	return result
}

func isSectionHeaderBlock(lines []string) bool {
	for i := len(lines) - 1; i >= 0; i-- {
		if !isBlank(lines[i]) {
			return isSectionHeader(lines[i])
		}
	}
	return false
}

func isIndented(line string) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

func sortImportBlocks(lines []string) []string {
	result := make([]string, len(lines))
	copy(result, lines)
	i := 0
	for i < len(result) {
		if isImportLine(result[i]) {
			start := i
			var imports []string
			imports = append(imports, result[i])
			i++
			for i < len(result) {
				if isImportLine(result[i]) {
					imports = append(imports, result[i])
					i++
				} else if isBlank(result[i]) {
					j := i
					for j < len(result) && isBlank(result[j]) {
						j++
					}
					if j < len(result) && isImportLine(result[j]) {
						i = j
					} else {
						break
					}
				} else {
					break
				}
			}
			end := i
			if len(imports) > 1 {
				sort.Strings(imports)
				var newResult []string
				newResult = append(newResult, result[:start]...)
				newResult = append(newResult, imports...)
				newResult = append(newResult, result[end:]...)
				result = newResult
				i = start + len(imports)
			}
		} else {
			i++
		}
	}
	return result
}

func Diff(filename, oldContent, newContent string) string {
	if oldContent == newContent {
		return ""
	}
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")
	var diff strings.Builder
	diff.WriteString("--- " + filename + "\n")
	diff.WriteString("+++ " + filename + " (formatted)\n")
	type hunk struct {
		oldStart int
		oldLines []string
		newStart int
		newLines []string
	}
	var hunks []hunk
	i, j := 0, 0
	for i < len(oldLines) || j < len(newLines) {
		if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
			i++
			j++
			continue
		}
		hOS := i
		hNS := j
		var hO, hN []string
		for i < len(oldLines) || j < len(newLines) {
			if i < len(oldLines) && j < len(newLines) && oldLines[i] == newLines[j] {
				break
			}
			mO, mN := false, false
			if i < len(oldLines) {
				for k := j; k < len(newLines) && k < j+3; k++ {
					if oldLines[i] == newLines[k] { mN = true; break }
				}
			}
			if j < len(newLines) {
				for k := i; k < len(oldLines) && k < i+3; k++ {
					if newLines[j] == oldLines[k] { mO = true; break }
				}
			}
			if mN && !mO {
				hN = append(hN, newLines[j]); j++
			} else if mO && !mN {
				hO = append(hO, oldLines[i]); i++
			} else {
				if i < len(oldLines) { hO = append(hO, oldLines[i]); i++ }
				if j < len(newLines) { hN = append(hN, newLines[j]); j++ }
			}
		}
		if len(hO) > 0 || len(hN) > 0 {
			hunks = append(hunks, hunk{oldStart: hOS + 1, oldLines: hO, newStart: hNS + 1, newLines: hN})
		}
	}
	for _, h := range hunks {
		diff.WriteString(formatHunkHeader(h.oldStart, len(h.oldLines), h.newStart, len(h.newLines)))
		for _, line := range h.oldLines { diff.WriteString("-" + line + "\n") }
		for _, line := range h.newLines { diff.WriteString("+" + line + "\n") }
	}
	return diff.String()
}

func formatHunkHeader(oldStart, oldCount, newStart, newCount int) string {
	var b strings.Builder
	b.WriteString("@@ -"); b.WriteString(itoa(oldStart))
	if oldCount != 1 { b.WriteString(","); b.WriteString(itoa(oldCount)) }
	b.WriteString(" +"); b.WriteString(itoa(newStart))
	if newCount != 1 { b.WriteString(","); b.WriteString(itoa(newCount)) }
	b.WriteString(" @@\n")
	return b.String()
}

func itoa(n int) string {
	if n < 0 { return "-" + itoa(-n) }
	if n < 10 { return string(rune('0' + n)) }
	return itoa(n/10) + string(rune('0'+n%10))
}
