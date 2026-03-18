//go:build ignore

package main

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

func Stdlib_String_length(s string) int64 {
	return int64(utf8.RuneCountInString(s))
}

func Stdlib_String_toUpper(s string) string   { return strings.ToUpper(s) }
func Stdlib_String_toLower(s string) string   { return strings.ToLower(s) }
func Stdlib_String_trim(s string) string      { return strings.TrimSpace(s) }
func Stdlib_String_trimLeft(s string) string  { return strings.TrimLeft(s, " \t\n\r") }
func Stdlib_String_trimRight(s string) string { return strings.TrimRight(s, " \t\n\r") }

func Stdlib_String_split(sep, s string) *RexList {
	parts := strings.Split(s, sep)
	var list *RexList
	for i := len(parts) - 1; i >= 0; i-- {
		list = &RexList{Head: parts[i], Tail: list}
	}
	return list
}

func Stdlib_String_join(sep string, lst *RexList) string {
	var parts []string
	for l := lst; l != nil; l = l.Tail {
		parts = append(parts, l.Head.(string))
	}
	return strings.Join(parts, sep)
}

func Stdlib_String_contains(sub, s string) bool   { return strings.Contains(s, sub) }
func Stdlib_String_startsWith(pfx, s string) bool { return strings.HasPrefix(s, pfx) }
func Stdlib_String_endsWith(sfx, s string) bool   { return strings.HasSuffix(s, sfx) }

func Stdlib_String_charAt(idx int64, s string) *string {
	runes := []rune(s)
	i := int(idx)
	if i >= 0 && i < len(runes) {
		result := string(runes[i])
		return &result
	}
	return nil
}

func Stdlib_String_substring(start, end int64, s string) string {
	runes := []rune(s)
	n := len(runes)
	sc, ec := int(start), int(end)
	if sc < 0 {
		sc = 0
	}
	if sc > n {
		sc = n
	}
	if ec < 0 {
		ec = 0
	}
	if ec > n {
		ec = n
	}
	return string(runes[sc:ec])
}

func Stdlib_String_indexOf(needle, haystack string) *int64 {
	byteIdx := strings.Index(haystack, needle)
	if byteIdx == -1 {
		return nil
	}
	idx := int64(utf8.RuneCountInString(haystack[:byteIdx]))
	return &idx
}

func Stdlib_String_replace(find, repl, s string) string {
	return strings.ReplaceAll(s, find, repl)
}

func Stdlib_String_take(n int64, s string) string {
	runes := []rune(s)
	end := int(n)
	if end < 0 {
		end = 0
	}
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[:end])
}

func Stdlib_String_drop(n int64, s string) string {
	runes := []rune(s)
	start := int(n)
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	return string(runes[start:])
}

func Stdlib_String_repeat(n int64, s string) string {
	count := int(n)
	if count < 0 {
		count = 0
	}
	return strings.Repeat(s, count)
}

func Stdlib_String_padLeft(width int64, pad, s string) string {
	padRunes := []rune(pad)
	if len(padRunes) == 0 {
		return s
	}
	runes := []rune(s)
	w := int(width)
	for len(runes) < w {
		runes = append([]rune{padRunes[0]}, runes...)
	}
	return string(runes)
}

func Stdlib_String_padRight(width int64, pad, s string) string {
	padRunes := []rune(pad)
	if len(padRunes) == 0 {
		return s
	}
	runes := []rune(s)
	w := int(width)
	for len(runes) < w {
		runes = append(runes, padRunes[0])
	}
	return string(runes)
}

func Stdlib_String_words(s string) *RexList {
	parts := strings.Fields(s)
	var list *RexList
	for i := len(parts) - 1; i >= 0; i-- {
		list = &RexList{Head: parts[i], Tail: list}
	}
	return list
}

func Stdlib_String_lines(s string) *RexList {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	parts := strings.Split(s, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	var list *RexList
	for i := len(parts) - 1; i >= 0; i-- {
		list = &RexList{Head: parts[i], Tail: list}
	}
	return list
}

func Stdlib_String_charCode(s string) *int64 {
	if s == "" {
		return nil
	}
	r, _ := utf8.DecodeRuneInString(s)
	code := int64(r)
	return &code
}

func Stdlib_String_fromCharCode(code int64) *string {
	if code < 0 || code > 0x10FFFF {
		return nil
	}
	s := string(rune(code))
	return &s
}

func Stdlib_String_parseInt(s string) *int64 {
	str := strings.TrimSpace(s)
	i, err := strconv.Atoi(str)
	if err != nil {
		return nil
	}
	v := int64(i)
	return &v
}

func Stdlib_String_parseFloat(s string) *float64 {
	str := strings.TrimSpace(s)
	f, err := strconv.ParseFloat(str, 64)
	if err != nil {
		return nil
	}
	return &f
}

func Stdlib_String_reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

func Stdlib_String_toList(s string) *RexList {
	runes := []rune(s)
	var list *RexList
	for i := len(runes) - 1; i >= 0; i-- {
		list = &RexList{Head: string(runes[i]), Tail: list}
	}
	return list
}

func Stdlib_String_fromList(lst *RexList) string {
	var b strings.Builder
	for l := lst; l != nil; l = l.Tail {
		b.WriteString(l.Head.(string))
	}
	return b.String()
}

func Stdlib_String_toString(v any) string {
	return rex_display(v)
}
