package mcp

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
)

const defaultProtocolVersion = "2024-11-05"

// Server implements a minimal MCP JSON-RPC server.
type Server struct {
	name    string
	version string
	project core.Project
	dbConn  *sql.DB
	agentID string
	mu      sync.Mutex
}

// NewServer initializes the MCP server and loads context.
func NewServer(projectPath, version string) (*Server, error) {
	project, err := core.DiscoverProject(projectPath)
	if err != nil {
		return nil, err
	}
	logf("Discovered project: %s", project.Root)

	dbConn, err := db.OpenDatabase(project)
	if err != nil {
		return nil, err
	}
	if err := db.InitSchema(dbConn); err != nil {
		_ = dbConn.Close()
		return nil, err
	}
	logf("Database opened: %s", project.DBPath)

	agentID, err := initializeMcpAgent(dbConn, project)
	if err != nil {
		_ = dbConn.Close()
		return nil, err
	}
	logf("Agent initialized: %s", agentID)

	return &Server{
		name:    "mm",
		version: version,
		project: project,
		dbConn:  dbConn,
		agentID: agentID,
	}, nil
}

// Run starts the MCP server loop using stdio.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}

	decoder := json.NewDecoder(in)
	writer := bufio.NewWriter(out)
	encoder := json.NewEncoder(writer)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		var req rpcRequest
		if err := decoder.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			logf("Decode error: %v", err)
			continue
		}

		if req.Method == "" {
			continue
		}

		isNotification := len(req.ID) == 0 || string(req.ID) == "null"
		if isNotification {
			if req.Method == "shutdown" {
				_ = s.Close()
				return nil
			}
			continue
		}

		result, rpcErr := s.handleRequest(req)
		response := rpcResponse{JSONRPC: "2.0", ID: req.ID}
		if rpcErr != nil {
			response.Error = rpcErr
		} else {
			response.Result = result
		}

		s.mu.Lock()
		if err := encoder.Encode(response); err != nil {
			s.mu.Unlock()
			logf("Encode error: %v", err)
			continue
		}
		_ = writer.Flush()
		s.mu.Unlock()
	}
}

// Close shuts down the server.
func (s *Server) Close() error {
	if s.dbConn != nil {
		_ = s.dbConn.Close()
	}
	logf("Server closed")
	return nil
}

func (s *Server) handleRequest(req rpcRequest) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req.Params), nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		return s.handleToolCall(req.Params)
	case "ping":
		return map[string]any{}, nil
	case "shutdown":
		return map[string]any{}, nil
	default:
		return nil, &rpcError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)}
	}
}

func (s *Server) handleInitialize(raw json.RawMessage) any {
	protocolVersion := defaultProtocolVersion
	if len(raw) > 0 {
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if err := json.Unmarshal(raw, &params); err == nil && params.ProtocolVersion != "" {
			protocolVersion = params.ProtocolVersion
		}
	}

	return map[string]any{
		"protocolVersion": protocolVersion,
		"serverInfo": map[string]any{
			"name":    s.name,
			"version": s.version,
		},
		"capabilities": map[string]any{
			"tools": map[string]any{},
		},
	}
}

func (s *Server) handleToolCall(raw json.RawMessage) (any, *rpcError) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return nil, &rpcError{Code: -32602, Message: "Invalid params"}
	}
	if params.Name == "" {
		return nil, &rpcError{Code: -32602, Message: "Missing tool name"}
	}

	ctx := ToolContext{AgentID: s.agentID, DB: s.dbConn, Project: s.project}
	result, err := handleToolCall(ctx, params.Name, params.Arguments)
	if err != nil {
		return nil, &rpcError{Code: -32603, Message: err.Error()}
	}
	return result, nil
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func logf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "[mm-mcp] %s\n", fmt.Sprintf(format, args...))
}
