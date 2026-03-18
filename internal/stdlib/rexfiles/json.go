//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Std_Json_jsonParse parses a JSON string into a Rex Json ADT.
// Returns (Json, error) for the standard Result wrapper.
func Std_Json_jsonParse(s string) (any, error) {
	var raw any
	if err := json.Unmarshal([]byte(s), &raw); err != nil {
		return nil, err
	}
	val, err := jsonToRex(raw)
	if err != nil {
		return nil, err
	}
	return val, nil
}

func jsonToRex(v any) (any, error) {
	if v == nil {
		return Rex_Json_JNull{}, nil
	}
	switch val := v.(type) {
	case bool:
		return Rex_Json_JBool{F0: val}, nil
	case float64:
		return Rex_Json_JNum{F0: val}, nil
	case string:
		return Rex_Json_JStr{F0: val}, nil
	case []any:
		var list *RexList
		for i := len(val) - 1; i >= 0; i-- {
			item, err := jsonToRex(val[i])
			if err != nil {
				return nil, err
			}
			list = &RexList{Head: item, Tail: list}
		}
		return Rex_Json_JArr{F0: list}, nil
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var list *RexList
		for i := len(keys) - 1; i >= 0; i-- {
			k := keys[i]
			item, err := jsonToRex(val[k])
			if err != nil {
				return nil, err
			}
			list = &RexList{Head: Tuple2{F0: k, F1: item}, Tail: list}
		}
		return Rex_Json_JObj{F0: list}, nil
	}
	return nil, fmt.Errorf("jsonParse: unexpected type %T", v)
}
