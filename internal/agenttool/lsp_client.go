// Package agenttool implements aikota's built-in tools.
// This file implements the LSP JSON-RPC client.
package agenttool

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// lspRequest is the JSON-RPC 2.0 request structure.
type lspRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	ID      int             `json:"id"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// lspResponse is the JSON-RPC 2.0 response structure.
type lspResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// LspClient is a client for communicating with an LSP server over stdio.
type LspClient struct {
	serverPath string
	args       []string
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     io.ReadCloser
	scanner    *bufio.Scanner
	mu         sync.Mutex
	nextID     int
	pending    map[int]chan *lspResponse
	timeout    time.Duration
}

// LspClientOptions configures an LspClient.
type LspClientOptions struct {
	// ServerPath is the path to the LSP server executable.
	// If empty, defaults to "gopls".
	ServerPath string
	// Args are additional arguments to pass to the server.
	Args []string
	// Timeout is the maximum time for a single request. Default: 10s.
	Timeout time.Duration
}

// NewLspClient creates a new LSP client.
func NewLspClient(opts LspClientOptions) (*LspClient, error) {
	path := opts.ServerPath
	if path == "" {
		var err error
		path, err = exec.LookPath("gopls")
		if err != nil {
			return nil, fmt.Errorf("lsp: gopls not found in PATH; set [lsp].server_path in config.toml: %w", err)
		}
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	cmd := exec.Command(path, opts.Args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp: cannot create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, fmt.Errorf("lsp: cannot create stdout pipe: %w", err)
	}

	c := &LspClient{
		serverPath: path,
		args:       opts.Args,
		stdin:      stdin,
		stdout:     stdout,
		scanner:    bufio.NewScanner(stdout),
		pending:    make(map[int]chan *lspResponse),
		timeout:    timeout,
	}

	if err := cmd.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, fmt.Errorf("lsp: cannot start %q: %w", path, err)
	}
	c.cmd = cmd

	// Start reading responses in background
	go c.readResponses()

	return c, nil
}

// Call sends an LSP request and waits for the response.
func (c *LspClient) Call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c.mu.Lock()
	id := c.nextID
	c.nextID++
	ch := make(chan *lspResponse, 1)
	c.pending[id] = ch
	c.mu.Unlock()

	req := lspRequest{
		JSONRPC: "2.0",
		Method:  method,
		ID:      id,
	}
	if params != nil {
		pm, err := json.Marshal(params)
		if err != nil {
			c.mu.Lock()
			delete(c.pending, id)
			c.mu.Unlock()
			return nil, fmt.Errorf("lsp: marshal params: %w", err)
		}
		req.Params = pm
	}

	data, err := json.Marshal(req)
	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("lsp: marshal request: %w", err)
	}
	data = append(data, '\n')

	// Write request with deadline
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := c.stdin.Write(data)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			c.mu.Lock()
			delete(c.pending, id)
			c.mu.Unlock()
			return nil, fmt.Errorf("lsp: write request: %w", err)
		}
	case <-writeCtx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("lsp: timeout writing request")
	}

	// Wait for response with timeout
	select {
	case resp := <-ch:
		if resp == nil {
			return nil, fmt.Errorf("lsp: no response received")
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("lsp: server error (code %d): %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("lsp: request %s cancelled: %w", method, ctx.Err())
	case <-time.After(c.timeout):
		return nil, fmt.Errorf("lsp: request %s timed out after %v", method, c.timeout)
	}
}

// readResponses reads responses from the server and dispatches them to pending requests.
func (c *LspClient) readResponses() {
	for c.scanner.Scan() {
		line := c.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp lspResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue // ignore malformed lines
		}
		if resp.ID == nil {
			continue // notifications have no id
		}

		c.mu.Lock()
		ch, ok := c.pending[*resp.ID]
		if ok {
			delete(c.pending, *resp.ID)
		}
		c.mu.Unlock()

		if ok {
			ch <- &resp
		}
	}
}

// Close terminates the LSP server process.
func (c *LspClient) Close() error {
	if c.cmd == nil {
		return nil
	}

	// Cancel all pending requests
	c.mu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()

	c.stdin.Close()
	c.stdout.Close()

	// Wait with timeout, then force kill
	done := make(chan error, 1)
	go func() {
		done <- c.cmd.Wait()
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(2 * time.Second):
		c.cmd.Process.Kill()
		return c.cmd.Wait()
	}
}

// uriFromPath converts a file path to a LSP URI (file://...).
func uriFromPath(path string) string {
	// Normalize to forward slashes for URI
	path = strings.ReplaceAll(path, "\\", "/")
	return "file://" + path
}
