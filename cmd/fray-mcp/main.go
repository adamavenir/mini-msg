package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/adamavenir/fray/internal/mcp"
)

// Version is overwritten at build time using -ldflags.
var Version = "dev"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	projectPath := os.Args[1]
	agentName := "desktop"
	if len(os.Args) >= 3 {
		agentName = os.Args[2]
	}

	server, err := mcp.NewServer(projectPath, agentName, Version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start MCP server: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signals
		_ = server.Close()
		os.Exit(0)
	}()

	if err := server.Run(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "MCP server error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: fray-mcp <project-path> [agent-name]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments:")
	fmt.Fprintln(os.Stderr, "  project-path  Path to project with .fray/ directory")
	fmt.Fprintln(os.Stderr, "  agent-name    Agent identity (default: desktop)")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Example:")
	fmt.Fprintln(os.Stderr, "  fray-mcp /Users/adam/dev/myproject claude-desktop")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Configure in Claude Desktop (~/Library/Application Support/Claude/claude_desktop_config.json):")
	fmt.Fprintln(os.Stderr, "  {")
	fmt.Fprintln(os.Stderr, "    \"mcpServers\": {")
	fmt.Fprintln(os.Stderr, "      \"fray-myproject\": {")
	fmt.Fprintln(os.Stderr, "        \"command\": \"/path/to/fray-mcp\",")
	fmt.Fprintln(os.Stderr, "        \"args\": [\"/Users/adam/dev/myproject\", \"claude-desktop\"]")
	fmt.Fprintln(os.Stderr, "      }")
	fmt.Fprintln(os.Stderr, "    }")
	fmt.Fprintln(os.Stderr, "  }")
}
