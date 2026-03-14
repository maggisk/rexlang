package rexfiles

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/maggisk/rexlang/internal/eval"
)

var JsonFFI = map[string]any{
	"jsonParse": eval.MakeBuiltin("jsonParse", func(v eval.Value) (eval.Value, error) {
		s, err := eval.CheckStr("jsonParse", v)
		if err != nil {
			return nil, err
		}
		var raw any
		if jsonErr := json.Unmarshal([]byte(s), &raw); jsonErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: jsonErr.Error()}}}, nil
		}
		result, convErr := jsonValToRex(raw)
		if convErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: convErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{result}}, nil
	}),
}

func jsonValToRex(v any) (eval.Value, error) {
	if v == nil {
		return eval.VCtor{Name: "JNull", Args: nil}, nil
	}
	switch val := v.(type) {
	case bool:
		return eval.VCtor{Name: "JBool", Args: []eval.Value{eval.VBool{V: val}}}, nil
	case float64:
		return eval.VCtor{Name: "JNum", Args: []eval.Value{eval.VFloat{V: val}}}, nil
	case string:
		return eval.VCtor{Name: "JStr", Args: []eval.Value{eval.VString{V: val}}}, nil
	case []any:
		items := make([]eval.Value, len(val))
		for i, elem := range val {
			item, err := jsonValToRex(elem)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return eval.VCtor{Name: "JArr", Args: []eval.Value{eval.VList{Items: items}}}, nil
	case map[string]any:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]eval.Value, len(keys))
		for i, k := range keys {
			item, err := jsonValToRex(val[k])
			if err != nil {
				return nil, err
			}
			pairs[i] = eval.VTuple{Items: []eval.Value{eval.VString{V: k}, item}}
		}
		return eval.VCtor{Name: "JObj", Args: []eval.Value{eval.VList{Items: pairs}}}, nil
	}
	return nil, &eval.RuntimeError{Msg: fmt.Sprintf("jsonParse: unexpected type %T", v)}
}
