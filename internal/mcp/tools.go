package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/adamavenir/fray/internal/core"
	"github.com/adamavenir/fray/internal/db"
	"github.com/adamavenir/fray/internal/types"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolContext struct {
	AgentID string
	DB      *sql.DB
	Project core.Project
}

type postArgs struct {
	Body string `json:"body" jsonschema:"Message body. Supports @mentions like @alice or @all for broadcast."`
}

type getArgs struct {
	Since string `json:"since,omitempty" jsonschema:"Get messages after this GUID (for polling new messages)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of messages to return (default: 10)"`
}

// RegisterTools registers MCP tools for fray.
func RegisterTools(server *mcp.Server, ctx *ToolContext) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "fray_post",
		Description: "Post a message to the fray room. Use @mentions to direct messages to specific agents.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args postArgs) (*mcp.CallToolResult, any, error) {
		return handlePost(*ctx, args.Body), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fray_get",
		Description: "Get recent room messages. Use for catching up on conversation.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args getArgs) (*mcp.CallToolResult, any, error) {
		return handleGet(*ctx, args.Since, args.Limit), nil, nil
	})
}

func handlePost(ctx ToolContext, body string) *mcp.CallToolResult {
	if strings.TrimSpace(body) == "" {
		return toolError("Error: Message body cannot be empty")
	}

	// Auto-join: create agent if not exists
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return toolError(err.Error())
	}
	if agent == nil {
		// Create the agent on first post
		now := time.Now().Unix()
		newAgent := types.Agent{
			AgentID:      ctx.AgentID,
			RegisteredAt: now,
			LastSeen:     now,
		}
		if err := db.CreateAgent(ctx.DB, newAgent); err != nil {
			return toolError(fmt.Sprintf("Failed to create agent: %v", err))
		}
		_ = db.AppendAgent(ctx.Project.DBPath, newAgent)
		agent = &newAgent
	}
	if agent.LeftAt != nil {
		// Reactivate if they had left
		now := time.Now().Unix()
		updates := db.AgentUpdates{
			LeftAt:   types.OptionalInt64{Set: true, Value: nil},
			LastSeen: types.OptionalInt64{Set: true, Value: &now},
		}
		if err := db.UpdateAgent(ctx.DB, ctx.AgentID, updates); err != nil {
			return toolError(err.Error())
		}
		if updated, err := db.GetAgent(ctx.DB, ctx.AgentID); err == nil && updated != nil {
			_ = db.AppendAgent(ctx.Project.DBPath, *updated)
		}
	}

	bases, err := db.GetAgentBases(ctx.DB)
	if err != nil {
		return toolError(err.Error())
	}
	mentions := core.ExtractMentions(body, bases)
	mentions = core.ExpandAllMention(mentions, bases)

	now := time.Now().Unix()
	created, err := db.CreateMessage(ctx.DB, types.Message{
		TS:        now,
		FromAgent: ctx.AgentID,
		Body:      strings.TrimSpace(body),
		Mentions:  mentions,
	})
	if err != nil {
		return toolError(err.Error())
	}
	_ = db.AppendMessage(ctx.Project.DBPath, created)

	updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
	_ = db.UpdateAgent(ctx.DB, ctx.AgentID, updates)

	mentionInfo := ""
	if len(mentions) > 0 {
		mentionInfo = fmt.Sprintf(" (mentioned: %s)", strings.Join(mentions, ", "))
	}
	return toolResult(fmt.Sprintf("Posted message #%s%s", created.ID, mentionInfo), false)
}

func handleGet(ctx ToolContext, since string, limit int) *mcp.CallToolResult {
	if limit <= 0 {
		limit = 10
	}
	options := &types.MessageQueryOptions{Limit: limit}
	if since != "" {
		options.SinceID = sanitizeMessageID(since)
	}

	messages, err := db.GetMessages(ctx.DB, options)
	if err != nil {
		return toolError(err.Error())
	}
	if len(messages) == 0 {
		qualifier := ""
		if since != "" {
			qualifier = fmt.Sprintf(" after message #%s", sanitizeMessageID(since))
		}
		return toolResult(fmt.Sprintf("No messages%s", qualifier), false)
	}

	formatted := formatMessages(messages)
	header := fmt.Sprintf("Recent messages (%d):", len(messages))
	if since != "" {
		header = fmt.Sprintf("Messages after #%s (%d):", sanitizeMessageID(since), len(messages))
	}
	return toolResult(fmt.Sprintf("%s\n\n%s", header, formatted), false)
}

func formatMessages(messages []types.Message) string {
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		mentions := ""
		if len(msg.Mentions) > 0 {
			mentions = fmt.Sprintf(" [mentions: %s]", strings.Join(msg.Mentions, ", "))
		}
		lines = append(lines, fmt.Sprintf("[#%s] @%s: %s%s", msg.ID, msg.FromAgent, msg.Body, mentions))
	}
	return strings.Join(lines, "\n")
}

func toolResult(text string, isError bool) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: isError,
	}
}

func toolError(text string) *mcp.CallToolResult {
	return toolResult(text, true)
}

func sanitizeMessageID(value string) string {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "@")
	trimmed = strings.TrimPrefix(trimmed, "#")
	return trimmed
}
