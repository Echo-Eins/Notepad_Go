package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	lsp "github.com/sourcegraph/go-lsp"
	jsonrpc2 "github.com/sourcegraph/jsonrpc2"
)

// LSPClient represents a connection to a language server.
type LSPClient struct {
	lang          string
	cmd           *exec.Cmd
	conn          *jsonrpc2.Conn
	mu            sync.Mutex
	diagnostics   map[string][]lsp.Diagnostic
	onDiagnostics func(string, []lsp.Diagnostic)
}

// Handle implements jsonrpc2.Handler for server notifications.
func (c *LSPClient) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Notif {
		switch req.Method {
		case "textDocument/publishDiagnostics":
			var params lsp.PublishDiagnosticsParams
			if err := json.Unmarshal(*req.Params, &params); err == nil {
				c.mu.Lock()
				c.diagnostics[string(params.URI)] = params.Diagnostics
				c.mu.Unlock()
				if c.onDiagnostics != nil {
					c.onDiagnostics(string(params.URI), params.Diagnostics)
				}
			}
		}
		return
	}
	// For requests we don't handle, just reply nil and log any error.
	if err := conn.Reply(ctx, req.ID, nil, nil); err != nil {
		fmt.Fprintf(os.Stderr, "LSP reply error: %v\n", err)
	}
}

// Shutdown stops the language server process.
func (c *LSPClient) Shutdown() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
}

// LSPManager manages language servers for different languages.
type LSPManager struct {
	clients           map[string]*LSPClient
	mu                sync.Mutex
	diagnosticHandler func(string, []lsp.Diagnostic)
}

// NewLSPManager creates a new manager.
func NewLSPManager() *LSPManager {
	return &LSPManager{clients: make(map[string]*LSPClient)}
}

// SetDiagnosticsHandler sets a callback for diagnostics.
func (m *LSPManager) SetDiagnosticsHandler(handler func(string, []lsp.Diagnostic)) {
	m.mu.Lock()
	m.diagnosticHandler = handler
	for _, c := range m.clients {
		c.onDiagnostics = handler
	}
	m.mu.Unlock()
}

// getClient returns an existing client or starts a new one.
func (m *LSPManager) getClient(lang, root string) (*LSPClient, error) {
	m.mu.Lock()
	if c, ok := m.clients[lang]; ok {
		m.mu.Unlock()
		return c, nil
	}
	m.mu.Unlock()

	cmdArgs := serverCommandFor(lang)
	if len(cmdArgs) == 0 {
		return nil, fmt.Errorf("no LSP server for %s", lang)
	}
	cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	stream := jsonrpc2.NewBufferedStream(struct {
		io.Reader
		io.Writer
	}{stdout, stdin}, jsonrpc2.VSCodeObjectCodec{})
	client := &LSPClient{lang: lang, cmd: cmd, diagnostics: make(map[string][]lsp.Diagnostic)}
	client.conn = jsonrpc2.NewConn(context.Background(), stream, client)

	rootURI := lsp.DocumentURI("file://" + filepath.ToSlash(root))
	initParams := lsp.InitializeParams{
		ProcessID:    os.Getpid(),
		RootURI:      rootURI,
		Capabilities: lsp.ClientCapabilities{},
	}
	var initRes lsp.InitializeResult
	if err := client.conn.Call(context.Background(), "initialize", initParams, &initRes); err != nil {
		client.Shutdown()
		return nil, err
	}
	client.conn.Notify(context.Background(), "initialized", struct{}{})

	m.mu.Lock()
	client.onDiagnostics = m.diagnosticHandler
	m.clients[lang] = client
	m.mu.Unlock()
	return client, nil
}

// serverCommandFor returns the command to start the language server.
func serverCommandFor(lang string) []string {
	switch lang {
	case "go":
		return []string{"gopls"}
	case "rust":
		return []string{"rust-analyzer"}
	case "python":
		return []string{"pylsp"}
	case "java":
		return []string{"jdtls"}
	case "c", "cpp":
		return []string{"clangd"}
	default:
		return nil
	}
}

// DidOpen notifies server about an opened document.
func (m *LSPManager) DidOpen(lang, uri, text string) error {
	root := filepath.Dir(uri)
	client, err := m.getClient(lang, root)
	if err != nil {
		return err
	}
	params := lsp.DidOpenTextDocumentParams{
		TextDocument: lsp.TextDocumentItem{
			URI:        lsp.DocumentURI("file://" + filepath.ToSlash(uri)),
			LanguageID: lang,
			Version:    1,
			Text:       text,
		},
	}
	return client.conn.Notify(context.Background(), "textDocument/didOpen", params)
}

// DidChange sends document changes to server.
func (m *LSPManager) DidChange(lang, uri, text string) error {
	client, err := m.getClient(lang, filepath.Dir(uri))
	if err != nil {
		return err
	}
	params := lsp.DidChangeTextDocumentParams{
		TextDocument: lsp.VersionedTextDocumentIdentifier{
			TextDocumentIdentifier: lsp.TextDocumentIdentifier{
				URI: lsp.DocumentURI("file://" + filepath.ToSlash(uri)),
			},
			Version: 1,
		},
		ContentChanges: []lsp.TextDocumentContentChangeEvent{{Text: text}},
	}
	return client.conn.Notify(context.Background(), "textDocument/didChange", params)
}

// DidSave notifies server about file save.
func (m *LSPManager) DidSave(lang, uri, text string) error {
	client, err := m.getClient(lang, filepath.Dir(uri))
	if err != nil {
		return err
	}
	params := struct {
		TextDocument lsp.TextDocumentIdentifier `json:"textDocument"`
		Text         *string                    `json:"text,omitempty"`
	}{
		TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentURI("file://" + filepath.ToSlash(uri))},
		Text:         &text,
	}
	return client.conn.Notify(context.Background(), "textDocument/didSave", params)
}

// Completion requests completion items at position.
func (m *LSPManager) Completion(lang, uri string, line, ch int) ([]lsp.CompletionItem, error) {
	client, err := m.getClient(lang, filepath.Dir(uri))
	if err != nil {
		return nil, err
	}
	params := lsp.CompletionParams{
		TextDocumentPositionParams: lsp.TextDocumentPositionParams{
			TextDocument: lsp.TextDocumentIdentifier{URI: lsp.DocumentURI("file://" + filepath.ToSlash(uri))},
		},
	}
	params.Position.Line = line
	params.Position.Character = ch
	var res lsp.CompletionList
	if err := client.conn.Call(context.Background(), "textDocument/completion", params, &res); err != nil {
		return nil, err
	}
	return res.Items, nil
}

// Shutdown terminates all language servers.
func (m *LSPManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for lang, c := range m.clients {
		_ = c.conn.Notify(context.Background(), "shutdown", nil)
		c.Shutdown()
		delete(m.clients, lang)
	}
}

// Diagnostics returns diagnostics for a file.
func (m *LSPManager) Diagnostics(lang, uri string) []lsp.Diagnostic {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.clients[lang]; ok {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.diagnostics["file://"+filepath.ToSlash(uri)]
	}
	return nil
}
