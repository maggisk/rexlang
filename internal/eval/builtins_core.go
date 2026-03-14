package eval

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------------
// Builtin helpers
// ---------------------------------------------------------------------------

func makeBuiltin(name string, fn func(Value) (Value, error)) Value {
	return VBuiltin{Name: name, Fn: fn}
}

func curried2(name string, fn func(Value, Value) (Value, error)) Value {
	return makeBuiltin(name, func(a Value) (Value, error) {
		return makeBuiltin(name+"$1", func(b Value) (Value, error) {
			return fn(a, b)
		}), nil
	})
}

// CoreBuiltins returns the minimal builtins: not, error.
func CoreBuiltins() map[string]Value {
	return map[string]Value{
		"not": makeBuiltin("not", func(v Value) (Value, error) {
			b, err := AsBool(v)
			if err != nil {
				return nil, err
			}
			return VBool{V: !b}, nil
		}),
		"error": makeBuiltin("error", func(v Value) (Value, error) {
			s, err := CheckStr("error", v)
			if err != nil {
				return nil, err
			}
			return nil, &RuntimeError{Msg: s}
		}),
		"todo": makeBuiltin("todo", func(v Value) (Value, error) {
			s, err := CheckStr("todo", v)
			if err != nil {
				return nil, err
			}
			return nil, &RuntimeError{Msg: "TODO: " + s}
		}),
		"showInt": makeBuiltin("showInt", func(v Value) (Value, error) {
			i, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			return VString{V: fmt.Sprintf("%d", i)}, nil
		}),
		"showFloat": makeBuiltin("showFloat", func(v Value) (Value, error) {
			f, err := AsFloat(v)
			if err != nil {
				return nil, err
			}
			return VString{V: floatToStr(f)}, nil
		}),
	}
}

// BuiltinsForModule returns builtins for a stdlib module: CoreBuiltins + module-specific ones.
func BuiltinsForModule(name string, programArgs []string) map[string]Value {
	result := make(map[string]Value)
	for k, v := range CoreBuiltins() {
		result[k] = v
	}

	// Auto-discover builtins from companion Go files
	loadRegisteredBuiltins(name, result)

	// Module-specific builtins that need eval internals
	switch name {
	case "IO":
		// print/println need Display() which is in eval
		result["print"] = makeBuiltin("print", func(v Value) (Value, error) {
			fmt.Print(Display(v))
			return v, nil
		})
		result["println"] = makeBuiltin("println", func(v Value) (Value, error) {
			fmt.Println(Display(v))
			return v, nil
		})
	case "Math":
		// Polymorphic builtins that handle both Int and Float
		result["abs"] = makeBuiltin("abs", func(v Value) (Value, error) {
			switch n := v.(type) {
			case VInt:
				a := n.V
				if a < 0 {
					a = -a
				}
				return VInt{V: a}, nil
			case VFloat:
				return VFloat{V: math.Abs(n.V)}, nil
			}
			return nil, &RuntimeError{Msg: "abs: expected number, got " + ValueToString(v)}
		})
		result["min"] = makeBuiltin("min", func(x Value) (Value, error) {
			return makeBuiltin("min$1", func(y Value) (Value, error) {
				switch a := x.(type) {
				case VInt:
					if b, ok := y.(VInt); ok {
						if a.V <= b.V {
							return a, nil
						}
						return b, nil
					}
				case VFloat:
					if b, ok := y.(VFloat); ok {
						if a.V <= b.V {
							return a, nil
						}
						return b, nil
					}
				}
				return nil, &RuntimeError{Msg: "min: expected matching numeric types"}
			}), nil
		})
		result["max"] = makeBuiltin("max", func(x Value) (Value, error) {
			return makeBuiltin("max$1", func(y Value) (Value, error) {
				switch a := x.(type) {
				case VInt:
					if b, ok := y.(VInt); ok {
						if a.V >= b.V {
							return a, nil
						}
						return b, nil
					}
				case VFloat:
					if b, ok := y.(VFloat); ok {
						if a.V >= b.V {
							return a, nil
						}
						return b, nil
					}
				}
				return nil, &RuntimeError{Msg: "max: expected matching numeric types"}
			}), nil
		})
		result["pow"] = makeBuiltin("pow", func(x Value) (Value, error) {
			return makeBuiltin("pow$1", func(y Value) (Value, error) {
				var xf, yf float64
				switch v := x.(type) {
				case VInt:
					xf = float64(v.V)
				case VFloat:
					xf = v.V
				default:
					return nil, &RuntimeError{Msg: "pow: expected number"}
				}
				switch v := y.(type) {
				case VInt:
					yf = float64(v.V)
				case VFloat:
					yf = v.V
				default:
					return nil, &RuntimeError{Msg: "pow: expected number"}
				}
				return VFloat{V: math.Pow(xf, yf)}, nil
			}), nil
		})
	case "String":
		// toString is polymorphic — handles multiple value types
		result["toString"] = makeBuiltin("toString", func(v Value) (Value, error) {
			switch val := v.(type) {
			case VInt:
				return VString{V: fmt.Sprintf("%d", val.V)}, nil
			case VFloat:
				return VString{V: floatToStr(val.V)}, nil
			case VBool:
				if val.V {
					return VString{V: "true"}, nil
				}
				return VString{V: "false"}, nil
			case VString:
				return v, nil
			}
			return nil, &RuntimeError{Msg: "toString: cannot convert " + ValueToString(v)}
		})
	case "Env":
		// args needs programArgs parameter
		argValues := make([]Value, len(programArgs))
		for i, a := range programArgs {
			argValues[i] = VString{V: a}
		}
		result["args"] = VList{Items: argValues}
	case "Result":
		result["try"] = makeBuiltin("try", func(fnV Value) (Value, error) {
			val, err := ApplyValue(fnV, VUnit{})
			if err != nil {
				if re, ok := err.(*RuntimeError); ok {
					switch re.Msg {
					case "division by zero":
						return VCtor{Name: "Err", Args: []Value{VCtor{Name: "DivisionByZero"}}}, nil
					case "modulo by zero":
						return VCtor{Name: "Err", Args: []Value{VCtor{Name: "ModuloByZero"}}}, nil
					}
				}
				return nil, err
			}
			return VCtor{Name: "Ok", Args: []Value{val}}, nil
		})
	case "Json":
		for k, v := range JsonBuiltins() {
			result[k] = v
		}
	case "List":
		for k, v := range ListBuiltins() {
			result[k] = v
		}
	case "Process":
		for k, v := range ProcessBuiltins(VPid{}) {
			result[k] = v
		}
	case "Net":
		for k, v := range NetBuiltins() {
			result[k] = v
		}
	case "Http.Server":
		for k, v := range HttpServerBuiltins() {
			result[k] = v
		}
	}

	return result
}

// ---------------------------------------------------------------------------
// Builtins that need eval internals (kept in eval package)
// ---------------------------------------------------------------------------

// JsonBuiltins returns JSON-related builtins.
func JsonBuiltins() map[string]Value {
	return map[string]Value{
		"jsonParse": makeBuiltin("jsonParse", func(v Value) (Value, error) {
			s, err := CheckStr("jsonParse", v)
			if err != nil {
				return nil, err
			}
			var pyVal interface{}
			if jsonErr := json.Unmarshal([]byte(s), &pyVal); jsonErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: jsonErr.Error()}}}, nil
			}
			result, convErr := jsonValToRex(pyVal)
			if convErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: convErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{result}}, nil
		}),
	}
}

func jsonValToRex(v interface{}) (Value, error) {
	if v == nil {
		return VCtor{Name: "JNull", Args: nil}, nil
	}
	switch val := v.(type) {
	case bool:
		return VCtor{Name: "JBool", Args: []Value{VBool{V: val}}}, nil
	case float64:
		return VCtor{Name: "JNum", Args: []Value{VFloat{V: val}}}, nil
	case string:
		return VCtor{Name: "JStr", Args: []Value{VString{V: val}}}, nil
	case []interface{}:
		items := make([]Value, len(val))
		for i, elem := range val {
			item, err := jsonValToRex(elem)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return VCtor{Name: "JArr", Args: []Value{VList{Items: items}}}, nil
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]Value, len(keys))
		for i, k := range keys {
			item, err := jsonValToRex(val[k])
			if err != nil {
				return nil, err
			}
			pairs[i] = VTuple{Items: []Value{VString{V: k}, item}}
		}
		return VCtor{Name: "JObj", Args: []Value{VList{Items: pairs}}}, nil
	}
	return nil, &RuntimeError{Msg: fmt.Sprintf("jsonParse: unexpected type %T", v)}
}

// ---------------------------------------------------------------------------
// Process / actor builtins
// ---------------------------------------------------------------------------

// ProcessBuiltins returns the process primitives.
func ProcessBuiltins(_ VPid) map[string]Value {
	return map[string]Value{
		"receive": makeBuiltin("receive", func(pidV Value) (Value, error) {
			pid, ok := pidV.(VPid)
			if !ok {
				return nil, runtimeErr("receive: expected Pid, got %s", ValueToString(pidV))
			}
			return pid.Mailbox.Receive(), nil
		}),
		"send": curried2("send", func(pidV, msgV Value) (Value, error) {
			pid, ok := pidV.(VPid)
			if !ok {
				return nil, runtimeErr("send: expected Pid, got %s", ValueToString(pidV))
			}
			pid.Mailbox.Send(msgV)
			return VUnit{}, nil
		}),
		"spawn": makeBuiltin("spawn", func(fnV Value) (Value, error) {
			mb := newMailbox()
			pid := VPid{Mailbox: mb, ID: mb.id}
			go func() {
				ApplyValue(fnV, pid) //nolint:errcheck
			}()
			return pid, nil
		}),
		"call": curried2("call", func(pidV, makeMsgV Value) (Value, error) {
			pid, ok := pidV.(VPid)
			if !ok {
				return nil, runtimeErr("call: expected Pid, got %s", ValueToString(pidV))
			}
			replyMb := newMailbox()
			replyPid := VPid{Mailbox: replyMb, ID: replyMb.id}
			msg, err := ApplyValue(makeMsgV, replyPid)
			if err != nil {
				return nil, err
			}
			pid.Mailbox.Send(msg)
			return replyMb.Receive(), nil
		}),
	}
}

// ListBuiltins returns list-related builtins.
func ListBuiltins() map[string]Value {
	return map[string]Value{
		"sortWith": curried2("sortWith", func(cmpFn, lstV Value) (Value, error) {
			lst, ok := lstV.(VList)
			if !ok {
				return nil, &RuntimeError{Msg: "sortWith: expected list"}
			}
			items := make([]Value, len(lst.Items))
			copy(items, lst.Items)
			var sortErr error
			sort.SliceStable(items, func(i, j int) bool {
				if sortErr != nil {
					return false
				}
				partial, err := ApplyValue(cmpFn, items[i])
				if err != nil {
					sortErr = err
					return false
				}
				result, err := ApplyValue(partial, items[j])
				if err != nil {
					sortErr = err
					return false
				}
				ctor, ok := result.(VCtor)
				if !ok {
					sortErr = &RuntimeError{Msg: "sortWith: comparison must return Ordering"}
					return false
				}
				return ctor.Name == "LT"
			})
			if sortErr != nil {
				return nil, sortErr
			}
			return VList{Items: items}, nil
		}),
	}
}

// NetBuiltins returns TCP networking builtins.
func NetBuiltins() map[string]Value {
	return map[string]Value{
		"tcpListen": makeBuiltin("tcpListen", func(v Value) (Value, error) {
			port, err := AsInt(v)
			if err != nil {
				return nil, err
			}
			ln, netErr := net.Listen("tcp", fmt.Sprintf(":%d", port))
			if netErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: netErr.Error()}}}, nil
			}
			actualPort := ln.Addr().(*net.TCPAddr).Port
			return VCtor{Name: "Ok", Args: []Value{VTuple{Items: []Value{VListener{L: ln}, VInt{V: actualPort}}}}}, nil
		}),
		"tcpAccept": makeBuiltin("tcpAccept", func(v Value) (Value, error) {
			ln, ok := v.(VListener)
			if !ok {
				return nil, runtimeErr("tcpAccept: expected Listener, got %s", ValueToString(v))
			}
			conn, netErr := ln.L.Accept()
			if netErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: netErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VConn{C: conn}}}, nil
		}),
		"tcpConnect": curried2("tcpConnect", func(hostV, portV Value) (Value, error) {
			host, err := CheckStr("tcpConnect", hostV)
			if err != nil {
				return nil, err
			}
			port, err := AsInt(portV)
			if err != nil {
				return nil, err
			}
			conn, netErr := net.Dial("tcp", fmt.Sprintf("%s:%d", host, port))
			if netErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: netErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VConn{C: conn}}}, nil
		}),
		"tcpRead": makeBuiltin("tcpRead", func(v Value) (Value, error) {
			c, ok := v.(VConn)
			if !ok {
				return nil, runtimeErr("tcpRead: expected Conn, got %s", ValueToString(v))
			}
			buf := make([]byte, 4096)
			n, readErr := c.C.Read(buf)
			if readErr != nil {
				if readErr == io.EOF {
					return VCtor{Name: "Err", Args: []Value{VString{V: "EOF"}}}, nil
				}
				return VCtor{Name: "Err", Args: []Value{VString{V: readErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VString{V: string(buf[:n])}}}, nil
		}),
		"tcpWrite": curried2("tcpWrite", func(connV, dataV Value) (Value, error) {
			c, ok := connV.(VConn)
			if !ok {
				return nil, runtimeErr("tcpWrite: expected Conn, got %s", ValueToString(connV))
			}
			data, err := CheckStr("tcpWrite", dataV)
			if err != nil {
				return nil, err
			}
			_, writeErr := c.C.Write([]byte(data))
			if writeErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: writeErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
		"tcpClose": makeBuiltin("tcpClose", func(v Value) (Value, error) {
			c, ok := v.(VConn)
			if !ok {
				return nil, runtimeErr("tcpClose: expected Conn, got %s", ValueToString(v))
			}
			if closeErr := c.C.Close(); closeErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: closeErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
		"tcpCloseListener": makeBuiltin("tcpCloseListener", func(v Value) (Value, error) {
			ln, ok := v.(VListener)
			if !ok {
				return nil, runtimeErr("tcpCloseListener: expected Listener, got %s", ValueToString(v))
			}
			if closeErr := ln.L.Close(); closeErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: closeErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
	}
}

// HttpServerBuiltins returns the HTTP server builtin.
func HttpServerBuiltins() map[string]Value {
	return map[string]Value{
		"httpServe": curried2("httpServe", func(portV, handlerV Value) (Value, error) {
			port, err := AsInt(portV)
			if err != nil {
				return nil, err
			}

			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				headerItems := make([]Value, 0, len(r.Header))
				for k, vals := range r.Header {
					for _, v := range vals {
						headerItems = append(headerItems, VTuple{Items: []Value{VString{V: k}, VString{V: v}}})
					}
				}

				queryItems := make([]Value, 0, len(r.URL.Query()))
				for k, vals := range r.URL.Query() {
					for _, v := range vals {
						queryItems = append(queryItems, VTuple{Items: []Value{VString{V: k}, VString{V: v}}})
					}
				}

				bodyBytes, _ := io.ReadAll(r.Body)

				reqRecord := VRecord{
					TypeName: "Request",
					Fields: map[string]Value{
						"method":  VString{V: r.Method},
						"path":    VString{V: r.URL.Path},
						"headers": VList{Items: headerItems},
						"body":    VString{V: string(bodyBytes)},
						"query":   VList{Items: queryItems},
					},
				}

				respV, err := ApplyValue(handlerV, reqRecord)
				if err != nil {
					w.WriteHeader(500)
					w.Write([]byte("Internal Server Error: " + err.Error()))
					return
				}

				resp, ok := respV.(VRecord)
				if !ok {
					w.WriteHeader(500)
					w.Write([]byte("handler did not return a Response record"))
					return
				}

				if hdrs, ok := resp.Fields["headers"].(VList); ok {
					for _, item := range hdrs.Items {
						if tup, ok := item.(VTuple); ok && len(tup.Items) == 2 {
							if name, ok := tup.Items[0].(VString); ok {
								if val, ok := tup.Items[1].(VString); ok {
									w.Header().Set(name.V, val.V)
								}
							}
						}
					}
				}

				statusCode := 200
				if s, ok := resp.Fields["status"].(VInt); ok {
					statusCode = s.V
				}
				w.WriteHeader(statusCode)

				if body, ok := resp.Fields["body"].(VString); ok {
					w.Write([]byte(body.V))
				}
			})

			addr := fmt.Sprintf(":%d", port)
			fmt.Printf("Listening on http://localhost:%d\n", port)
			if listenErr := http.ListenAndServe(addr, mux); listenErr != nil {
				return VCtor{Name: "Err", Args: []Value{VString{V: listenErr.Error()}}}, nil
			}
			return VCtor{Name: "Ok", Args: []Value{VUnit{}}}, nil
		}),
	}
}

func floatToStr(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.Contains(s, ".") && !strings.Contains(s, "e") &&
		!strings.Contains(s, "E") && !strings.Contains(s, "n") && !strings.Contains(s, "N") {
		s += ".0"
	}
	return s
}
