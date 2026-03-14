package rexfiles

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/maggisk/rexlang/internal/eval"
)

var StringFFI = map[string]any{
	"length":       String_length,
	"toUpper":      String_toUpper,
	"toLower":      String_toLower,
	"trim":         String_trim,
	"trimLeft":     String_trimLeft,
	"trimRight":    String_trimRight,
	"split":        String_split,
	"join":         String_join,
	"contains":     String_contains,
	"startsWith":   String_startsWith,
	"endsWith":     String_endsWith,
	"charAt":       String_charAt,
	"substring":    String_substring,
	"indexOf":      String_indexOf,
	"replace":      String_replace,
	"take":         String_take,
	"drop":         String_drop,
	"repeat":       String_repeat,
	"padLeft":      String_padLeft,
	"padRight":     String_padRight,
	"words":        String_words,
	"lines":        String_lines,
	"charCode":     String_charCode,
	"fromCharCode": String_fromCharCode,
	"parseInt":     String_parseInt,
	"parseFloat":   String_parseFloat,
	"reverse":      String_reverse,
	"toList":       String_toList,
	"fromList":     String_fromList,
	// toString is polymorphic — handles multiple value types
	"toString": eval.MakeBuiltin("toString", func(v eval.Value) (eval.Value, error) {
		switch val := v.(type) {
		case eval.VInt:
			return eval.VString{V: fmt.Sprintf("%d", val.V)}, nil
		case eval.VFloat:
			return eval.VString{V: eval.FloatToStr(val.V)}, nil
		case eval.VBool:
			if val.V {
				return eval.VString{V: "true"}, nil
			}
			return eval.VString{V: "false"}, nil
		case eval.VString:
			return v, nil
		}
		return nil, &eval.RuntimeError{Msg: "toString: cannot convert " + eval.ValueToString(v)}
	}),
}

func String_length(s string) int       { return utf8.RuneCountInString(s) }
func String_toUpper(s string) string   { return strings.ToUpper(s) }
func String_toLower(s string) string   { return strings.ToLower(s) }
func String_trim(s string) string      { return strings.TrimSpace(s) }
func String_trimLeft(s string) string  { return strings.TrimLeft(s, " \t\n\r") }
func String_trimRight(s string) string { return strings.TrimRight(s, " \t\n\r") }

func String_split(sep, s string) []string         { return strings.Split(s, sep) }
func String_join(sep string, lst []string) string  { return strings.Join(lst, sep) }

func String_contains(sub, s string) bool          { return strings.Contains(s, sub) }
func String_startsWith(prefix, s string) bool     { return strings.HasPrefix(s, prefix) }
func String_endsWith(suffix, s string) bool       { return strings.HasSuffix(s, suffix) }

func String_charAt(idx int, s string) *string {
	runes := []rune(s)
	if idx >= 0 && idx < len(runes) {
		result := string(runes[idx])
		return &result
	}
	return nil
}

func String_substring(start, end int, s string) string {
	runes := []rune(s)
	n := len(runes)
	sc := clampInt(start, 0, n)
	ec := clampInt(end, 0, n)
	return string(runes[sc:ec])
}

func String_indexOf(needle, haystack string) *int {
	byteIdx := strings.Index(haystack, needle)
	if byteIdx == -1 {
		return nil
	}
	runeIdx := utf8.RuneCountInString(haystack[:byteIdx])
	return &runeIdx
}

func String_replace(find, repl, s string) string {
	return strings.ReplaceAll(s, find, repl)
}

func String_take(n int, s string) string {
	runes := []rune(s)
	end := clampInt(n, 0, len(runes))
	return string(runes[:end])
}

func String_drop(n int, s string) string {
	runes := []rune(s)
	start := clampInt(n, 0, len(runes))
	return string(runes[start:])
}

func String_repeat(n int, s string) string {
	if n < 0 {
		n = 0
	}
	return strings.Repeat(s, n)
}

func String_padLeft(width int, pad, s string) string {
	if utf8.RuneCountInString(pad) != 1 {
		panic("padLeft: fill must be a single character")
	}
	runes := []rune(s)
	padRunes := []rune(pad)
	for len(runes) < width {
		runes = append(padRunes, runes...)
	}
	return string(runes)
}

func String_padRight(width int, pad, s string) string {
	if utf8.RuneCountInString(pad) != 1 {
		panic("padRight: fill must be a single character")
	}
	runes := []rune(s)
	padRune := []rune(pad)[0]
	for len(runes) < width {
		runes = append(runes, padRune)
	}
	return string(runes)
}

func String_words(s string) []string { return strings.Fields(s) }

func String_lines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}

func String_charCode(s string) int {
	if s == "" {
		panic("charCode: empty string")
	}
	r, _ := utf8.DecodeRuneInString(s)
	return int(r)
}

func String_fromCharCode(code int) string {
	if code < 0 || code > 0x10FFFF {
		panic(fmt.Sprintf("fromCharCode: invalid code point %d", code))
	}
	return string(rune(code))
}

func String_parseInt(s string) *int {
	s = strings.TrimSpace(s)
	i, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &i
}

func String_parseFloat(s string) *float64 {
	s = strings.TrimSpace(s)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func String_reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func String_toList(s string) []string {
	runes := []rune(s)
	result := make([]string, len(runes))
	for i, r := range runes {
		result[i] = string(r)
	}
	return result
}

func String_fromList(lst []string) string {
	var buf strings.Builder
	for _, s := range lst {
		buf.WriteString(s)
	}
	return buf.String()
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
