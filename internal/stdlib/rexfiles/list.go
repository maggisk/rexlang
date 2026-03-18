//go:build ignore

package main

import "sort"

func Stdlib_List_sortWith(cmpFn any, lst *RexList) *RexList {
	var items []any
	for l := lst; l != nil; l = l.Tail {
		items = append(items, l.Head)
	}
	sort.SliceStable(items, func(i, j int) bool {
		partial := rex__apply(cmpFn, items[i])
		result := rex__apply(partial, items[j])
		_, ok := result.(Rex_Ordering_LT)
		return ok
	})
	var result *RexList
	for i := len(items) - 1; i >= 0; i-- {
		result = &RexList{Head: items[i], Tail: result}
	}
	return result
}
