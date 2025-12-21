package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Server wraps the MCP SDK server with mm context.
type Server struct {
	server  *mcp.Server
	project core.Project
	dbConn  *sql.DB
	agentID string
}

// NewServer initializes the MCP server with tools and identity.
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

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	server := mcp.NewServer(&mcp.Implementation{Name: "mm", Title: "Mini Messenger", Version: version}, &mcp.ServerOptions{Logger: logger})

	toolCtx := &ToolContext{AgentID: agentID, DB: dbConn, Project: project}
	RegisterTools(server, toolCtx)

	return &Server{server: server, project: project, dbConn: dbConn, agentID: agentID}, nil
}

// Run starts the MCP server on stdio.
func (s *Server) Run(ctx context.Context) error {
	logf("Connected via stdio")
	return s.server.Run(ctx, &mcp.StdioTransport{})
}

// Close shuts down the server and cleans up.
func (s *Server) Close() error {
	if s.dbConn != nil {
		_ = s.dbConn.Close()
	}
	logf("Server closed")
	return nil
}

func logf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "[mm-mcp] %s\n", fmt.Sprintf(format, args...))
}
