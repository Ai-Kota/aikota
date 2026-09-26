// This file implements the Model Context Protocol (MCP) adapter for aikota.
//
// MCP is a standard protocol for connecting AI models to external tools.
// This implementation provides:
// - stdio transport for IPC with MCP servers
// - Tool list and schema exchange
// - Tool execution proxying
//
// Reference: https://modelcontextprotocol.io/specification
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Message represents an MCP JSON-RPC message.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.Number     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// ToolInfo represents a tool advertised by an MCP server.
type ToolInfo struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Server represents an MCP server process we connect to.
type Server struct {
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	nextID    atomic.Int64
}

// Connect starts an MCP server process and connects to it via stdio.
func Connect(ctx context.Context, serverCmd string, args []string) (*Server, error) {
	cmd := exec.CommandContext(ctx, serverCmd, args...)
	
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdin pipe: %w", err)
	}
	
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stdout pipe: %w", err)
	}
	
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp: stderr pipe: %w", err)
	}
	
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("mcp: start server: %w", err)
	}
	
	s := &Server{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
	}
	
	// Start reading responses in background
	go s.readLoop(ctx)
	
	// Send initialize request
	if err := s.initialize(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("mcp: initialize: %w", err)
	}
	
	return s, nil
}

// Close shuts down the MCP server.
func (s *Server) Close() error {
	s.stdin.Close()
	s.stdout.Close()
	s.stderr.Close()
	return s.cmd.Wait()
}

// ListTools returns the tool list advertised by the MCP server.
func (s *Server) ListTools(ctx context.Context) ([]ToolInfo, error) {
	resp, err := s.request(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	
	var result struct {
		Tools []ToolInfo `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp: parse tools/list: %w", err)
	}
	
	return result.Tools, nil
}

// CallTool invokes a tool on the MCP server.
func (s *Server) CallTool(ctx context.Context, name string, args map[string]interface{}) (string, error) {
	params, _ := json.Marshal(args)
	resp, err := s.request(ctx, "tools/call", map[string]json.RawMessage{
		"name":      json.RawMessage(fmt.Sprintf(`"%s"`, name)),
		"arguments": params,
	})
	if err != nil {
		return "", err
	}
	
	var result struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text,omitempty"`
			Image    string `json:"image,omitempty"`
			MimeType string `json:"mimeType,omitempty"`
		} `json:"content"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return "", fmt.Errorf("mcp: parse tool result: %w", err)
	}
	
	var sb strings.Builder
	for _, c := range result.Content {
		switch c.Type {
		case "text":
			sb.WriteString(c.Text)
		case "image":
			sb.WriteString(fmt.Sprintf("[image: %s (%s)]", c.MimeType, c.Image[:min(50, len(c.Image))]))
		default:
			sb.WriteString(fmt.Sprintf("[%s]", c.Type))
		}
	}
	
	return sb.String(), nil
}

func (s *Server) request(ctx context.Context, method string, params interface{}) (*Message, error) {
	id := fmt.Sprintf("req-%d", s.nextID.Add(1))
	msg := Message{
		JSONRPC: "2.0",
		ID:      json.Number(id),
		Method:  method,
	}
	if params != nil {
		msg.Params, _ = json.Marshal(params)
	}
	
	// Send request
	payload, _ := json.Marshal(msg)
	payload = append(payload, '\n')
	if _, err := s.stdin.Write(payload); err != nil {
		return nil, fmt.Errorf("mcp: send request: %w", err)
	}
	
	// Wait for response with timeout
	select {
	case resp := <-getResponse(id):
		if resp.Error != nil {
			return nil, fmt.Errorf("mcp: %s error: %s", method, resp.Error.Message)
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, fmt.Errorf("mcp: request timeout")
	}
}

func (s *Server) initialize(ctx context.Context) error {
	params, _ := json.Marshal(map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo":      map[string]string{"name": "aikota", "version": "0.1.0"},
	})
	
	msg := Message{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "initialize",
		Params:  params,
	}
	payload, _ := json.Marshal(msg)
	payload = append(payload, '\n')
	if _, err := s.stdin.Write(payload); err != nil {
		return err
	}
	
	// Read response (simplified - just drain one message)
	select {
	case resp := <-getResponse("1"):
		if resp.Error != nil {
			return fmt.Errorf("mcp: initialize error: %s", resp.Error.Message)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) readLoop(ctx context.Context) {
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)
	
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		
		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		
		if msg.ID != "" {
			getResponse(msg.ID.String()) <- &msg
		}
	}
}

// responseChan stores pending responses keyed by ID.
var responseChans = make(map[string]chan *Message)
var responseMu sync.Mutex

func getResponse(id string) chan *Message {
	responseMu.Lock()
	defer responseMu.Unlock()
	
	ch, ok := responseChans[id]
	if !ok {
		ch = make(chan *Message, 1)
		responseChans[id] = ch
	}
	return ch
}
