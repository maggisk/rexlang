// Package lsp implements a minimal Language Server Protocol server for RexLang.
//
// It provides diagnostics (parse errors + type errors) on didOpen/didChange/didSave,
// and stubs for hover and go-to-definition.
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/maggisk/rexlang/internal/lexer"
	"github.com/maggisk/rexlang/internal/manifest"
	"github.com/maggisk/rexlang/internal/parser"
	"github.com/maggisk/rexlang/internal/typechecker"
	"github.com/maggisk/rexlang/internal/types"
)

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 types
// ---------------------------------------------------------------------------

type jsonRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  interface{}      `json:"result,omitempty"`
	Error   *jsonRPCError    `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type initializeParams struct {
	RootURI  string `json:"rootUri"`
	RootPath string `json:"rootPath"`
}

type textDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type versionedTextDocumentID struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type textDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type didChangeParams struct {
	TextDocument   versionedTextDocumentID          `json:"textDocument"`
	ContentChanges []textDocumentContentChangeEvent `json:"contentChanges"`
}

type didSaveParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

type didCloseParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
}

type hoverParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position position `json:"position"`
}

type definitionParams = hoverParams

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

type diagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

type Server struct {
	reader  *bufio.Reader
	writer  io.Writer
	writeMu sync.Mutex
	logger  *log.Logger

	documents   map[string]string
	documentsMu sync.Mutex

	rootPath string
	shutdown bool
}

// Run starts the LSP server, reading from stdin and writing to stdout.
func Run() {
	logFile, err := os.CreateTemp("", "rex-lsp-*.log")
	if err != nil {
		logFile = nil
	}
	var logger *log.Logger
	if logFile != nil {
		logger = log.New(logFile, "rex-lsp: ", log.LstdFlags|log.Lshortfile)
		logger.Println("LSP server starting")
	} else {
		logger = log.New(io.Discard, "", 0)
	}

	s := &Server{
		reader:    bufio.NewReader(os.Stdin),
		writer:    os.Stdout,
		logger:    logger,
		documents: make(map[string]string),
	}
	s.serve()
}

func (s *Server) serve() {
	for {
		msg, err := s.readMessage()
		if err != nil {
			if err == io.EOF {
				s.logger.Println("EOF on stdin, exiting")
				return
			}
			s.logger.Printf("read error: %v", err)
			return
		}
		s.handleMessage(msg)
	}
}

func (s *Server) readMessage() (*jsonRPCMessage, error) {
	var contentLength int
	for {
		line, err := s.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length: ") {
			n, err := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if err != nil {
				return nil, fmt.Errorf("bad Content-Length: %v", err)
			}
			contentLength = n
		}
	}
	if contentLength == 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	body := make([]byte, contentLength)
	_, err := io.ReadFull(s.reader, body)
	if err != nil {
		return nil, err
	}
	var msg jsonRPCMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("json decode: %v", err)
	}
	s.logger.Printf("<- %s (id=%v)", msg.Method, msg.ID)
	return &msg, nil
}

func (s *Server) sendResponse(id *json.RawMessage, result interface{}) {
	s.writeMessage(jsonRPCMessage{JSONRPC: "2.0", ID: id, Result: result})
}

func (s *Server) sendError(id *json.RawMessage, code int, message string) {
	s.writeMessage(jsonRPCMessage{JSONRPC: "2.0", ID: id, Error: &jsonRPCError{Code: code, Message: message}})
}

func (s *Server) sendNotification(method string, params interface{}) {
	s.writeMessage(jsonRPCMessage{JSONRPC: "2.0", Method: method, Params: mustMarshal(params)})
}

func (s *Server) writeMessage(msg jsonRPCMessage) {
	body, err := json.Marshal(msg)
	if err != nil {
		s.logger.Printf("marshal error: %v", err)
		return
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.writer.Write([]byte(header))
	s.writer.Write(body)
	s.logger.Printf("-> %s (id=%v)", msg.Method, msg.ID)
}

func mustMarshal(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

func (s *Server) handleMessage(msg *jsonRPCMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// No-op.
	case "shutdown":
		s.shutdown = true
		s.sendResponse(msg.ID, nil)
	case "exit":
		if s.shutdown {
			os.Exit(0)
		}
		os.Exit(1)
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didSave":
		s.handleDidSave(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	default:
		if msg.ID != nil {
			s.sendError(msg.ID, -32601, "method not found: "+msg.Method)
		}
	}
}

func (s *Server) handleInitialize(msg *jsonRPCMessage) {
	var params initializeParams
	json.Unmarshal(msg.Params, &params)
	if params.RootURI != "" {
		s.rootPath = uriToPath(params.RootURI)
	} else if params.RootPath != "" {
		s.rootPath = params.RootPath
	}
	s.logger.Printf("rootPath = %s", s.rootPath)
	s.setupProjectEnv()
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": map[string]interface{}{
				"openClose": true,
				"change":    1,
				"save":      map[string]interface{}{"includeText": false},
			},
			"hoverProvider":      true,
			"definitionProvider": true,
		},
		"serverInfo": map[string]interface{}{
			"name":    "rex-lsp",
			"version": "0.1.0",
		},
	}
	s.sendResponse(msg.ID, result)
}

func (s *Server) setupProjectEnv() {
	if s.rootPath == "" {
		return
	}
	typechecker.SetTarget("native")
	srcDir := filepath.Join(s.rootPath, "src")
	if info, err := os.Stat(srcDir); err == nil && info.IsDir() {
		typechecker.SetSrcRoot(srcDir)
	}
	projectRoot := manifest.FindProjectRoot(s.rootPath)
	if projectRoot != "" {
		_, deps, err := manifest.Load(projectRoot)
		if err == nil {
			roots, err := manifest.PackageRoots(projectRoot, deps)
			if err == nil {
				typechecker.SetPackageRoots(roots)
			}
		}
	}
}

func (s *Server) handleDidOpen(msg *jsonRPCMessage) {
	var params didOpenParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didOpen unmarshal error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	text := params.TextDocument.Text
	s.documentsMu.Lock()
	s.documents[uri] = text
	s.documentsMu.Unlock()
	s.publishDiagnostics(uri, text)
}

func (s *Server) handleDidChange(msg *jsonRPCMessage) {
	var params didChangeParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didChange unmarshal error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	if len(params.ContentChanges) > 0 {
		text := params.ContentChanges[len(params.ContentChanges)-1].Text
		s.documentsMu.Lock()
		s.documents[uri] = text
		s.documentsMu.Unlock()
		s.publishDiagnostics(uri, text)
	}
}

func (s *Server) handleDidSave(msg *jsonRPCMessage) {
	var params didSaveParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didSave unmarshal error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	s.documentsMu.Lock()
	text, ok := s.documents[uri]
	s.documentsMu.Unlock()
	if ok {
		s.publishDiagnostics(uri, text)
	}
}

func (s *Server) handleDidClose(msg *jsonRPCMessage) {
	var params didCloseParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logger.Printf("didClose unmarshal error: %v", err)
		return
	}
	uri := params.TextDocument.URI
	s.documentsMu.Lock()
	delete(s.documents, uri)
	s.documentsMu.Unlock()
	s.sendNotification("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         uri,
		Diagnostics: []diagnostic{},
	})
}

func (s *Server) handleHover(msg *jsonRPCMessage) {
	s.sendResponse(msg.ID, nil)
}

func (s *Server) handleDefinition(msg *jsonRPCMessage) {
	s.sendResponse(msg.ID, nil)
}

func (s *Server) publishDiagnostics(uri, source string) {
	diags := s.diagnose(source, uriToPath(uri))
	s.sendNotification("textDocument/publishDiagnostics", publishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	})
}

func (s *Server) diagnose(source, filePath string) []diagnostic {
	var diags []diagnostic
	exprs, err := parser.Parse(source)
	if err != nil {
		diags = append(diags, errorToDiagnostic(err, "parse"))
		return diags
	}
	if err := parser.ValidateToplevel(exprs); err != nil {
		diags = append(diags, errorToDiagnostic(err, "syntax"))
		return diags
	}
	if err := parser.ValidateIndentation(exprs); err != nil {
		diags = append(diags, errorToDiagnostic(err, "indentation"))
		return diags
	}
	exprs, err = typechecker.ReorderToplevel(exprs)
	if err != nil {
		diags = append(diags, errorToDiagnostic(err, "type"))
		return diags
	}
	_, warnings, err := typechecker.CheckProgram(exprs)
	if err != nil {
		diags = append(diags, errorToDiagnostic(err, "type"))
	}
	for _, w := range warnings {
		diags = append(diags, warningToDiagnostic(w))
	}
	return diags
}

func errorToDiagnostic(err error, source string) diagnostic {
	line := 0
	col := 0
	msg := err.Error()
	switch e := err.(type) {
	case *types.TypeError:
		if e.Line > 0 {
			line = e.Line - 1
		}
		msg = e.Msg
	case *parser.ParseError:
		if e.Line > 0 {
			line = e.Line - 1
		}
		if e.Col > 0 {
			col = e.Col - 1
		}
		msg = e.Msg
	case *lexer.LexError:
		if e.Line > 0 {
			line = e.Line - 1
		}
		if e.Col > 0 {
			col = e.Col - 1
		}
		msg = e.Msg
	}
	return diagnostic{
		Range: lspRange{
			Start: position{Line: line, Character: col},
			End:   position{Line: line, Character: col + 1},
		},
		Severity: 1,
		Source:   "rex/" + source,
		Message:  msg,
	}
}

func warningToDiagnostic(w typechecker.Warning) diagnostic {
	line := 0
	if w.Line > 0 {
		line = w.Line - 1
	}
	return diagnostic{
		Range: lspRange{
			Start: position{Line: line, Character: 0},
			End:   position{Line: line, Character: 1},
		},
		Severity: 2,
		Source:   "rex/warning",
		Message:  w.Msg,
	}
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}
