package rexfiles

import (
	"sort"

	"github.com/maggisk/rexlang/internal/eval"
)

var ListFFI = map[string]any{
	"sortWith": eval.Curried2("sortWith", func(cmpFn, lstV eval.Value) (eval.Value, error) {
		lst, ok := lstV.(eval.VList)
		if !ok {
			return nil, &eval.RuntimeError{Msg: "sortWith: expected list"}
		}
		items := make([]eval.Value, len(lst.Items))
		copy(items, lst.Items)
		var sortErr error
		sort.SliceStable(items, func(i, j int) bool {
			if sortErr != nil {
				return false
			}
			partial, err := eval.ApplyValue(cmpFn, items[i])
			if err != nil {
				sortErr = err
				return false
			}
			result, err := eval.ApplyValue(partial, items[j])
			if err != nil {
				sortErr = err
				return false
			}
			ctor, ok := result.(eval.VCtor)
			if !ok {
				sortErr = &eval.RuntimeError{Msg: "sortWith: comparison must return Ordering"}
				return false
			}
			return ctor.Name == "LT"
		})
		if sortErr != nil {
			return nil, sortErr
		}
		return eval.VList{Items: items}, nil
	}),
}
