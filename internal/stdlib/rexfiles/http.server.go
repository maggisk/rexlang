//go:build ignore

package main

import (
	"fmt"
	"io"
	"net/http"
)

// Stdlib_Http_Server_httpServe starts an HTTP server.
// The handler is a Rex closure: Request -> Response.
// Returns error for the standard Result () String wrapper.
func Stdlib_Http_Server_httpServe(port int64, handler any) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Build header list
		var headers *RexList
		for k, vals := range r.Header {
			for i := len(vals) - 1; i >= 0; i-- {
				headers = &RexList{Head: Tuple2{F0: k, F1: vals[i]}, Tail: headers}
			}
		}

		// Build query list
		var query *RexList
		for k, vals := range r.URL.Query() {
			for i := len(vals) - 1; i >= 0; i-- {
				query = &RexList{Head: Tuple2{F0: k, F1: vals[i]}, Tail: query}
			}
		}

		bodyBytes, _ := io.ReadAll(r.Body)

		req := Rex_Request{
			Method:  r.Method,
			Path:    r.URL.Path,
			Headers: headers,
			Body:    string(bodyBytes),
			Query:   query,
		}

		respV := rex__apply(handler, req)
		resp := respV.(Rex_Response)

		// Write response headers
		if hdrs, ok := resp.Headers.(*RexList); ok {
			for l := hdrs; l != nil; l = l.Tail {
				if tup, ok := l.Head.(Tuple2); ok {
					if name, ok := tup.F0.(string); ok {
						if val, ok := tup.F1.(string); ok {
							w.Header().Set(name, val)
						}
					}
				}
			}
		}

		statusCode := int64(200)
		if s, ok := resp.Status.(int64); ok {
			statusCode = s
		}
		w.WriteHeader(int(statusCode))

		if body, ok := resp.Body.(string); ok {
			w.Write([]byte(body))
		}
	})

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Listening on http://localhost:%d\n", port)
	return http.ListenAndServe(addr, mux)
}
