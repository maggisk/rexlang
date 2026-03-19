// Package lsp implements a minimal Language Server Protocol server for Rex.
// It provides diagnostics (parse + type errors) over JSON-RPC 2.0 on stdin/stdout.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// Run starts the LSP server on stdin/stdout.
func Run() {
	s := &server{
		docs: make(map[string]string),
	}
	s.serve(os.Stdin, os.Stdout)
}

type server struct {
	docs map[string]string // URI → content
}

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 types
// ---------------------------------------------------------------------------

type jsonrpcMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------

func (s *server) serve(in io.Reader, out io.Writer) {
	reader := bufio.NewReader(in)
	for {
		msg, err := readMessage(reader)
		if err != nil {
			return // EOF or broken pipe
		}
		s.handle(msg, out)
	}
}

func readMessage(r *bufio.Reader) (*jsonrpcMessage, error) {
	// Read headers
	contentLength := 0
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLength, _ = strconv.Atoi(val)
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("no content-length")
	}

	body := make([]byte, contentLength)
	_, err := io.ReadFull(r, body)
	if err != nil {
		return nil, err
	}

	var msg jsonrpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func writeMessage(out io.Writer, resp interface{}) {
	body, _ := json.Marshal(resp)
	fmt.Fprintf(out, "Content-Length: %d\r\n\r\n%s", len(body), body)
}

func (s *server) respond(out io.Writer, id interface{}, result interface{}) {
	writeMessage(out, jsonrpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

// ---------------------------------------------------------------------------
// Request handling
// ---------------------------------------------------------------------------

func (s *server) handle(msg *jsonrpcMessage, out io.Writer) {
	switch msg.Method {
	case "initialize":
		var id interface{}
		if msg.ID != nil {
			json.Unmarshal(*msg.ID, &id)
		}
		s.respond(out, id, map[string]interface{}{
			"capabilities": map[string]interface{}{
				"textDocumentSync": 1, // Full sync
				"hoverProvider":    false,
			},
		})

	case "initialized":
		// no-op

	case "shutdown":
		var id interface{}
		if msg.ID != nil {
			json.Unmarshal(*msg.ID, &id)
		}
		s.respond(out, id, nil)

	case "exit":
		os.Exit(0)

	case "textDocument/didOpen":
		var params struct {
			TextDocument struct {
				URI  string `json:"uri"`
				Text string `json:"text"`
			} `json:"textDocument"`
		}
		json.Unmarshal(msg.Params, &params)
		s.docs[params.TextDocument.URI] = params.TextDocument.Text
		s.publishDiagnostics(out, params.TextDocument.URI, params.TextDocument.Text)

	case "textDocument/didChange":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		json.Unmarshal(msg.Params, &params)
		if len(params.ContentChanges) > 0 {
			text := params.ContentChanges[len(params.ContentChanges)-1].Text
			s.docs[params.TextDocument.URI] = text
			s.publishDiagnostics(out, params.TextDocument.URI, text)
		}

	case "textDocument/didSave":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		json.Unmarshal(msg.Params, &params)
		if text, ok := s.docs[params.TextDocument.URI]; ok {
			s.publishDiagnostics(out, params.TextDocument.URI, text)
		}

	case "textDocument/didClose":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		json.Unmarshal(msg.Params, &params)
		delete(s.docs, params.TextDocument.URI)
		// Clear diagnostics
		s.publishDiagnostics(out, params.TextDocument.URI, "")

	case "textDocument/hover":
		var id interface{}
		if msg.ID != nil {
			json.Unmarshal(*msg.ID, &id)
		}
		s.respond(out, id, nil)
	}
}

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

type diagnostic struct {
	Range    diagRange `json:"range"`
	Severity int       `json:"severity"`
	Message  string    `json:"message"`
}

type diagRange struct {
	Start diagPos `json:"start"`
	End   diagPos `json:"end"`
}

type diagPos struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

func (s *server) publishDiagnostics(out io.Writer, uri string, text string) {
	var diags []diagnostic

	if text != "" {
		diags = s.diagnose(text)
	}
	if diags == nil {
		diags = []diagnostic{} // empty array, not null
	}

	writeMessage(out, map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "textDocument/publishDiagnostics",
		"params": map[string]interface{}{
			"uri":         uri,
			"diagnostics": diags,
		},
	})
}

func (s *server) diagnose(text string) []diagnostic {
	var diags []diagnostic

	// Parse
	exprs, err := parser.Parse(text)
	if err != nil {
		diags = append(diags, errorToDiag(err, 1))
		return diags
	}

	// Validate
	if err := parser.ValidateToplevel(exprs); err != nil {
		diags = append(diags, errorToDiag(err, 1))
		return diags
	}
	if err := parser.ValidateIndentation(exprs); err != nil {
		diags = append(diags, errorToDiag(err, 1))
		return diags
	}

	// Reorder
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		diags = append(diags, errorToDiag(err, 1))
		return diags
	}

	// Typecheck
	_, warnings, err := typechecker.CheckProgram(exprs, "")
	if err != nil {
		diags = append(diags, errorToDiag(err, 1))
	}

	// Warnings (e.g., todo usage)
	for _, w := range warnings {
		diags = append(diags, diagnostic{
			Range:    lineRange(w.Line),
			Severity: 2, // Warning
			Message:  w.Msg,
		})
	}

	return diags
}

func errorToDiag(err error, severity int) diagnostic {
	line := 0
	if te, ok := err.(*types.TypeError); ok && te.Line > 0 {
		line = te.Line
	}
	return diagnostic{
		Range:    lineRange(line),
		Severity: severity,
		Message:  err.Error(),
	}
}

func lineRange(line int) diagRange {
	if line <= 0 {
		line = 1
	}
	l := line - 1 // LSP is 0-indexed
	return diagRange{
		Start: diagPos{Line: l, Character: 0},
		End:   diagPos{Line: l, Character: 1000},
	}
}

// uriToPath converts a file:// URI to a filesystem path.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	return u.Path
}
