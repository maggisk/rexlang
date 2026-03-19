package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strings"
	"testing"
)

func testServer() (*Server, *bytes.Buffer) {
	var outBuf bytes.Buffer
	s := &Server{
		reader:    bufio.NewReader(strings.NewReader("")),
		writer:    &outBuf,
		logger:    log.New(io.Discard, "", 0),
		documents: make(map[string]string),
	}
	return s, &outBuf
}

func readResponse(buf *bytes.Buffer) (*jsonRPCMessage, error) {
	reader := bufio.NewReader(buf)
	var contentLength int
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			fmt.Sscanf(strings.TrimPrefix(line, "Content-Length: "), "%d", &contentLength)
		}
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	if err != nil {
		return nil, err
	}
	var msg jsonRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func TestInitialize(t *testing.T) {
	s, outBuf := testServer()
	id := json.RawMessage(`1`)
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "initialize",
		Params:  mustMarshal(initializeParams{RootURI: "file:///tmp/test"}),
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
	resultBytes, _ := json.Marshal(resp.Result)
	var result map[string]interface{}
	json.Unmarshal(resultBytes, &result)
	caps, ok := result["capabilities"]
	if !ok {
		t.Fatal("missing capabilities in initialize response")
	}
	capsMap := caps.(map[string]interface{})
	if _, ok := capsMap["textDocumentSync"]; !ok {
		t.Fatal("missing textDocumentSync capability")
	}
}

func TestDiagnosticsOnParseError(t *testing.T) {
	s, outBuf := testServer()
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: mustMarshal(didOpenParams{
			TextDocument: textDocumentItem{
				URI: "file:///tmp/test.rex", LanguageID: "rex", Version: 1,
				Text: "let x =\n",
			},
		}),
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if resp.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("expected publishDiagnostics, got %s", resp.Method)
	}
	var params publishDiagnosticsParams
	json.Unmarshal(resp.Params, &params)
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for parse error, got none")
	}
	if params.Diagnostics[0].Severity != 1 {
		t.Fatalf("expected severity 1, got %d", params.Diagnostics[0].Severity)
	}
}

func TestDiagnosticsOnTypeError(t *testing.T) {
	s, outBuf := testServer()
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: mustMarshal(didOpenParams{
			TextDocument: textDocumentItem{
				URI: "file:///tmp/test.rex", LanguageID: "rex", Version: 1,
				Text: "x = 1 + \"hello\"\n",
			},
		}),
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	var params publishDiagnosticsParams
	json.Unmarshal(resp.Params, &params)
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected diagnostics for type error, got none")
	}
}

func TestDiagnosticsCleanFile(t *testing.T) {
	s, outBuf := testServer()
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: mustMarshal(didOpenParams{
			TextDocument: textDocumentItem{
				URI: "file:///tmp/test.rex", LanguageID: "rex", Version: 1,
				Text: "x = 1 + 2\n",
			},
		}),
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	var params publishDiagnosticsParams
	json.Unmarshal(resp.Params, &params)
	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics, got %d: %v", len(params.Diagnostics), params.Diagnostics[0].Message)
	}
}

func TestDiagnosticsOnChange(t *testing.T) {
	s, outBuf := testServer()
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: mustMarshal(didOpenParams{
			TextDocument: textDocumentItem{
				URI: "file:///tmp/test.rex", LanguageID: "rex", Version: 1,
				Text: "x = 1\n",
			},
		}),
	})
	readResponse(outBuf)
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/didChange",
		Params: mustMarshal(didChangeParams{
			TextDocument: versionedTextDocumentID{URI: "file:///tmp/test.rex", Version: 2},
			ContentChanges: []textDocumentContentChangeEvent{
				{Text: "x = 1 + \"hello\"\n"},
			},
		}),
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	var params publishDiagnosticsParams
	json.Unmarshal(resp.Params, &params)
	if len(params.Diagnostics) == 0 {
		t.Fatal("expected diagnostics after change, got none")
	}
}

func TestDidClose(t *testing.T) {
	s, outBuf := testServer()
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/didOpen",
		Params: mustMarshal(didOpenParams{
			TextDocument: textDocumentItem{
				URI: "file:///tmp/test.rex", LanguageID: "rex", Version: 1,
				Text: "x = 1\n",
			},
		}),
	})
	readResponse(outBuf)
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		Method:  "textDocument/didClose",
		Params: mustMarshal(didCloseParams{
			TextDocument: struct {
				URI string `json:"uri"`
			}{URI: "file:///tmp/test.rex"},
		}),
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	var params publishDiagnosticsParams
	json.Unmarshal(resp.Params, &params)
	if len(params.Diagnostics) != 0 {
		t.Fatalf("expected 0 diagnostics on close, got %d", len(params.Diagnostics))
	}
}

func TestHoverReturnsNull(t *testing.T) {
	s, outBuf := testServer()
	id := json.RawMessage(`2`)
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "textDocument/hover",
		Params: mustMarshal(hoverParams{
			TextDocument: struct {
				URI string `json:"uri"`
			}{URI: "file:///tmp/test.rex"},
			Position: position{Line: 0, Character: 0},
		}),
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %v", resp.Error.Message)
	}
}

func TestShutdown(t *testing.T) {
	s, outBuf := testServer()
	id := json.RawMessage(`3`)
	s.handleMessage(&jsonRPCMessage{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  "shutdown",
	})
	resp, err := readResponse(outBuf)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error on shutdown: %v", resp.Error.Message)
	}
	if !s.shutdown {
		t.Fatal("shutdown flag should be set")
	}
}

func TestURIToPath(t *testing.T) {
	tests := []struct {
		uri, want string
	}{
		{"file:///home/user/test.rex", "/home/user/test.rex"},
		{"file:///tmp/test.rex", "/tmp/test.rex"},
		{"/tmp/test.rex", "/tmp/test.rex"},
	}
	for _, tt := range tests {
		got := uriToPath(tt.uri)
		if got != tt.want {
			t.Errorf("uriToPath(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
