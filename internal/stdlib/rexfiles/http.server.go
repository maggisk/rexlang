package rexfiles

import (
	"fmt"
	"io"
	"net/http"

	"github.com/maggisk/rexlang/internal/eval"
)

var HttpServerFFI = map[string]any{
	"httpServe": eval.Curried2("httpServe", func(portV, handlerV eval.Value) (eval.Value, error) {
		port, err := eval.AsInt(portV)
		if err != nil {
			return nil, err
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			headerItems := make([]eval.Value, 0, len(r.Header))
			for k, vals := range r.Header {
				for _, v := range vals {
					headerItems = append(headerItems, eval.VTuple{Items: []eval.Value{eval.VString{V: k}, eval.VString{V: v}}})
				}
			}

			queryItems := make([]eval.Value, 0, len(r.URL.Query()))
			for k, vals := range r.URL.Query() {
				for _, v := range vals {
					queryItems = append(queryItems, eval.VTuple{Items: []eval.Value{eval.VString{V: k}, eval.VString{V: v}}})
				}
			}

			bodyBytes, _ := io.ReadAll(r.Body)

			reqRecord := eval.VRecord{
				TypeName: "Request",
				Fields: map[string]eval.Value{
					"method":  eval.VString{V: r.Method},
					"path":    eval.VString{V: r.URL.Path},
					"headers": eval.VList{Items: headerItems},
					"body":    eval.VString{V: string(bodyBytes)},
					"query":   eval.VList{Items: queryItems},
				},
			}

			respV, err := eval.ApplyValue(handlerV, reqRecord)
			if err != nil {
				w.WriteHeader(500)
				w.Write([]byte("Internal Server Error: " + err.Error()))
				return
			}

			resp, ok := respV.(eval.VRecord)
			if !ok {
				w.WriteHeader(500)
				w.Write([]byte("handler did not return a Response record"))
				return
			}

			if hdrs, ok := resp.Fields["headers"].(eval.VList); ok {
				for _, item := range hdrs.Items {
					if tup, ok := item.(eval.VTuple); ok && len(tup.Items) == 2 {
						if name, ok := tup.Items[0].(eval.VString); ok {
							if val, ok := tup.Items[1].(eval.VString); ok {
								w.Header().Set(name.V, val.V)
							}
						}
					}
				}
			}

			statusCode := 200
			if s, ok := resp.Fields["status"].(eval.VInt); ok {
				statusCode = s.V
			}
			w.WriteHeader(statusCode)

			if body, ok := resp.Fields["body"].(eval.VString); ok {
				w.Write([]byte(body.V))
			}
		})

		addr := fmt.Sprintf(":%d", port)
		fmt.Printf("Listening on http://localhost:%d\n", port)
		if listenErr := http.ListenAndServe(addr, mux); listenErr != nil {
			return eval.VCtor{Name: "Err", Args: []eval.Value{eval.VString{V: listenErr.Error()}}}, nil
		}
		return eval.VCtor{Name: "Ok", Args: []eval.Value{eval.VUnit{}}}, nil
	}),
}
