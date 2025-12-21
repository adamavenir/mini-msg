package mcp

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/adamavenir/mini-msg/internal/core"
	"github.com/adamavenir/mini-msg/internal/db"
	"github.com/adamavenir/mini-msg/internal/types"
	mcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

type ToolContext struct {
	AgentID string
	DB      *sql.DB
	Project core.Project
}

type postArgs struct {
	Body string `json:"body" jsonschema:"Message body. Supports @mentions like @alice.1 or @all for broadcast."`
}

type getArgs struct {
	Since string `json:"since,omitempty" jsonschema:"Get messages after this GUID (for polling new messages)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of messages to return (default: 10)"`
}

type mentionsArgs struct {
	Since string `json:"since,omitempty" jsonschema:"Get mentions after this GUID (for polling)"`
	Limit int    `json:"limit,omitempty" jsonschema:"Maximum number of mentions to return (default: 10)"`
}

type claimArgs struct {
	Files      []string `json:"files,omitempty" jsonschema:"File paths or glob patterns to claim"`
	BD         []string `json:"bd,omitempty" jsonschema:"Beads issue IDs to claim"`
	Issues     []string `json:"issues,omitempty" jsonschema:"GitHub issue numbers to claim"`
	Reason     string   `json:"reason,omitempty" jsonschema:"Reason for claim (optional)"`
	TTLMinutes int      `json:"ttl_minutes,omitempty" jsonschema:"Time to live in minutes (optional, default: no expiry)"`
}

type clearArgs struct {
	File  string `json:"file,omitempty" jsonschema:"Specific file claim to clear (optional)"`
	BD    string `json:"bd,omitempty" jsonschema:"Specific bd issue claim to clear (optional)"`
	Issue string `json:"issue,omitempty" jsonschema:"Specific GitHub issue claim to clear (optional)"`
}

type claimsArgs struct {
	Agent string `json:"agent,omitempty" jsonschema:"Filter by agent ID (optional, defaults to all agents)"`
}

type statusArgs struct {
	Message    string   `json:"message,omitempty" jsonschema:"Status message (your current task/focus)"`
	Files      []string `json:"files,omitempty" jsonschema:"File paths or glob patterns to claim"`
	BD         []string `json:"bd,omitempty" jsonschema:"Beads issue IDs to claim"`
	Issues     []string `json:"issues,omitempty" jsonschema:"GitHub issue numbers to claim"`
	TTLMinutes int      `json:"ttl_minutes,omitempty" jsonschema:"Time to live in minutes for claims"`
	Clear      bool     `json:"clear,omitempty" jsonschema:"Clear all claims and reset status"`
}

// RegisterTools registers all MCP tools for mm.
func RegisterTools(server *mcp.Server, ctx *ToolContext) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_post",
		Description: "Post a message to the mm room. Use @mentions to direct messages to specific agents.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args postArgs) (*mcp.CallToolResult, any, error) {
		return handlePost(*ctx, args.Body), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_get",
		Description: "Get recent room messages. Use for catching up on conversation.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args getArgs) (*mcp.CallToolResult, any, error) {
		return handleGet(*ctx, args.Since, args.Limit), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_mentions",
		Description: "Get messages that mention me. Use to check for messages directed at you.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args mentionsArgs) (*mcp.CallToolResult, any, error) {
		return handleMentions(*ctx, args.Since, args.Limit), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_here",
		Description: "List active agents in the room. See who is available to collaborate.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return handleHere(*ctx), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_whoami",
		Description: "Show my agent identity. Returns your agent ID and status.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
		return handleWhoami(*ctx), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_claim",
		Description: "Claim resources (files, bd issues, GitHub issues) to prevent collision with other agents.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args claimArgs) (*mcp.CallToolResult, any, error) {
		return handleClaim(*ctx, args.Files, args.BD, args.Issues, args.Reason, args.TTLMinutes), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_clear",
		Description: "Clear claims. Clear all your claims or specific ones.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args clearArgs) (*mcp.CallToolResult, any, error) {
		return handleClear(*ctx, args.File, args.BD, args.Issue), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_claims",
		Description: "List active claims. Shows who has claimed what resources.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args claimsArgs) (*mcp.CallToolResult, any, error) {
		return handleClaims(*ctx, args.Agent), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "mm_status",
		Description: "Update your status with optional resource claims. Sets your status and claims resources in one operation.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, args statusArgs) (*mcp.CallToolResult, any, error) {
		return handleStatus(*ctx, args.Message, args.Files, args.BD, args.Issues, args.TTLMinutes, args.Clear), nil, nil
	})
}

func handlePost(ctx ToolContext, body string) *mcp.CallToolResult {
	if strings.TrimSpace(body) == "" {
		return toolError("Error: Message body cannot be empty")
	}

	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return toolError(err.Error())
	}
	if agent == nil {
		return toolError(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID))
	}
	if agent.LeftAt != nil {
		return toolError(fmt.Sprintf("Error: Agent %s has left the room", ctx.AgentID))
	}

	bases, err := db.GetAgentBases(ctx.DB)
	if err != nil {
		return toolError(err.Error())
	}
	mentions := core.ExtractMentions(body, bases)

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

func handleMentions(ctx ToolContext, since string, limit int) *mcp.CallToolResult {
	if limit <= 0 {
		limit = 10
	}

	prefix := ctx.AgentID
	if parsed, err := core.ParseAgentID(ctx.AgentID); err == nil {
		prefix = parsed.Base
	}
	options := &types.MessageQueryOptions{Limit: limit}
	if since != "" {
		options.SinceID = sanitizeMessageID(since)
	}

	messages, err := db.GetMessagesWithMention(ctx.DB, prefix, options)
	if err != nil {
		return toolError(err.Error())
	}
	if len(messages) == 0 {
		qualifier := ""
		if since != "" {
			qualifier = fmt.Sprintf(" after message #%s", sanitizeMessageID(since))
		}
		return toolResult(fmt.Sprintf("No mentions%s", qualifier), false)
	}

	formatted := formatMessages(messages)
	header := fmt.Sprintf("Recent mentions (%d):", len(messages))
	if since != "" {
		header = fmt.Sprintf("Mentions after #%s (%d):", sanitizeMessageID(since), len(messages))
	}
	return toolResult(fmt.Sprintf("%s\n\n%s", header, formatted), false)
}

func handleHere(ctx ToolContext) *mcp.CallToolResult {
	staleHours := 4
	if raw, err := db.GetConfig(ctx.DB, "stale_hours"); err == nil {
		staleHours = parseStaleHours(raw, 4)
	}

	agents, err := db.GetActiveAgents(ctx.DB, staleHours)
	if err != nil {
		return toolError(err.Error())
	}
	if len(agents) == 0 {
		return toolResult("No active agents", false)
	}

	lines := make([]string, 0, len(agents))
	for _, agent := range agents {
		status := ""
		if agent.Status != nil && *agent.Status != "" {
			status = fmt.Sprintf(": \"%s\"", *agent.Status)
		}
		lines = append(lines, fmt.Sprintf("  %s%s", agent.AgentID, status))
	}
	return toolResult(fmt.Sprintf("Active agents (%d):\n%s", len(agents), strings.Join(lines, "\n")), false)
}

func handleWhoami(ctx ToolContext) *mcp.CallToolResult {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return toolError(err.Error())
	}
	if agent == nil {
		return toolResult(fmt.Sprintf("Agent ID: %s (not found in database)", ctx.AgentID), false)
	}

	activeStatus := "active"
	if agent.LeftAt != nil {
		activeStatus = "left"
	}
	statusLine := ""
	if agent.Status != nil && *agent.Status != "" {
		statusLine = fmt.Sprintf("\nStatus: %s", *agent.Status)
	}
	purposeLine := ""
	if agent.Purpose != nil && *agent.Purpose != "" {
		purposeLine = fmt.Sprintf("\nPurpose: %s", *agent.Purpose)
	}
	return toolResult(fmt.Sprintf("Agent ID: %s\nActive: %s%s%s", ctx.AgentID, activeStatus, statusLine, purposeLine), false)
}

func handleClaim(ctx ToolContext, files, bdClaims, issues []string, reason string, ttlMinutes int) *mcp.CallToolResult {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return toolError(err.Error())
	}
	if agent == nil {
		return toolError(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID))
	}

	_, _ = db.PruneExpiredClaims(ctx.DB)

	var expiresAt *int64
	if ttlMinutes > 0 {
		value := time.Now().Unix() + int64(ttlMinutes*60)
		expiresAt = &value
	}

	claims := make([]types.ClaimInput, 0, len(files)+len(bdClaims)+len(issues))
	for _, pattern := range files {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeFile, Pattern: pattern})
	}
	for _, entry := range bdClaims {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeBD, Pattern: stripHash(entry)})
	}
	for _, entry := range issues {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeIssue, Pattern: stripHash(entry)})
	}

	if len(claims) == 0 {
		return toolError("Error: No claims specified. Provide files, bd, or issues.")
	}

	created := make([]types.Claim, 0, len(claims))
	var errors []string
	for _, claim := range claims {
		createdClaim, err := db.CreateClaim(ctx.DB, types.ClaimInput{
			AgentID:   ctx.AgentID,
			ClaimType: claim.ClaimType,
			Pattern:   claim.Pattern,
			Reason:    optionalString(reason),
			ExpiresAt: expiresAt,
		})
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		created = append(created, *createdClaim)
	}

	if len(created) > 0 {
		body := fmt.Sprintf("claimed: %s", buildClaimList(created))
		msg, err := db.CreateMessage(ctx.DB, types.Message{
			TS:        time.Now().Unix(),
			FromAgent: ctx.AgentID,
			Body:      body,
			Mentions:  []string{},
		})
		if err == nil {
			_ = db.AppendMessage(ctx.Project.DBPath, msg)
		}
	}

	result := fmt.Sprintf("Claimed %d resource", len(created))
	if len(created) != 1 {
		result += "s"
	}
	if len(created) > 0 {
		list := make([]string, 0, len(created))
		for _, claim := range created {
			if claim.ClaimType == types.ClaimTypeFile {
				list = append(list, claim.Pattern)
			} else {
				list = append(list, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
			}
		}
		result += ":\n  " + strings.Join(list, "\n  ")
	}
	if len(errors) > 0 {
		result += "\n\nErrors:\n  " + strings.Join(errors, "\n  ")
	}
	if expiresAt != nil {
		result += fmt.Sprintf("\n\nExpires in %d minutes", ttlMinutes)
	}

	return toolResult(result, len(errors) > 0)
}

func handleClear(ctx ToolContext, file, bdEntry, issue string) *mcp.CallToolResult {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return toolError(err.Error())
	}
	if agent == nil {
		return toolError(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID))
	}

	cleared := 0
	var clearedItems []string

	if file != "" {
		if ok, _ := db.DeleteClaim(ctx.DB, types.ClaimTypeFile, file); ok {
			cleared++
			clearedItems = append(clearedItems, file)
		}
	}
	if bdEntry != "" {
		pattern := stripHash(bdEntry)
		if ok, _ := db.DeleteClaim(ctx.DB, types.ClaimTypeBD, pattern); ok {
			cleared++
			clearedItems = append(clearedItems, fmt.Sprintf("bd:%s", pattern))
		}
	}
	if issue != "" {
		pattern := stripHash(issue)
		if ok, _ := db.DeleteClaim(ctx.DB, types.ClaimTypeIssue, pattern); ok {
			cleared++
			clearedItems = append(clearedItems, fmt.Sprintf("issue:%s", pattern))
		}
	}

	if file == "" && bdEntry == "" && issue == "" {
		existing, err := db.GetClaimsByAgent(ctx.DB, ctx.AgentID)
		if err == nil {
			for _, claim := range existing {
				if claim.ClaimType == types.ClaimTypeFile {
					clearedItems = append(clearedItems, claim.Pattern)
				} else {
					clearedItems = append(clearedItems, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
				}
			}
		}
		count, err := db.DeleteClaimsByAgent(ctx.DB, ctx.AgentID)
		if err == nil {
			cleared = int(count)
		}
	}

	if cleared > 0 {
		body := fmt.Sprintf("cleared claims: %s", strings.Join(clearedItems, ", "))
		msg, err := db.CreateMessage(ctx.DB, types.Message{
			TS:        time.Now().Unix(),
			FromAgent: ctx.AgentID,
			Body:      body,
			Mentions:  []string{},
		})
		if err == nil {
			_ = db.AppendMessage(ctx.Project.DBPath, msg)
		}
		return toolResult(fmt.Sprintf("Cleared %d claim", cleared)+pluralSuffix(cleared)+":\n  "+strings.Join(clearedItems, "\n  "), false)
	}

	return toolResult("No claims to clear", false)
}

func handleClaims(ctx ToolContext, agent string) *mcp.CallToolResult {
	var claims []types.Claim
	var err error
	if agent != "" {
		claims, err = db.GetClaimsByAgent(ctx.DB, agent)
	} else {
		claims, err = db.GetAllClaims(ctx.DB)
	}
	if err != nil {
		return toolError(err.Error())
	}
	if len(claims) == 0 {
		qualifier := ""
		if agent != "" {
			qualifier = fmt.Sprintf(" for @%s", agent)
		}
		return toolResult(fmt.Sprintf("No active claims%s", qualifier), false)
	}

	byAgent := make(map[string][]types.Claim)
	for _, claim := range claims {
		byAgent[claim.AgentID] = append(byAgent[claim.AgentID], claim)
	}

	lines := []string{fmt.Sprintf("Active claims (%d):", len(claims))}
	for agentID, entries := range byAgent {
		lines = append(lines, fmt.Sprintf("\n@%s:", agentID))
		for _, claim := range entries {
			typePrefix := ""
			if claim.ClaimType != types.ClaimTypeFile {
				typePrefix = fmt.Sprintf("%s:", claim.ClaimType)
			}
			reason := ""
			if claim.Reason != nil && *claim.Reason != "" {
				reason = fmt.Sprintf(" - %s", *claim.Reason)
			}
			lines = append(lines, fmt.Sprintf("  %s%s%s", typePrefix, claim.Pattern, reason))
		}
	}

	return toolResult(strings.Join(lines, "\n"), false)
}

func handleStatus(ctx ToolContext, message string, files, bdClaims, issues []string, ttlMinutes int, clear bool) *mcp.CallToolResult {
	agent, err := db.GetAgent(ctx.DB, ctx.AgentID)
	if err != nil {
		return toolError(err.Error())
	}
	if agent == nil {
		return toolError(fmt.Sprintf("Error: Agent %s not found", ctx.AgentID))
	}

	if clear {
		count, err := db.DeleteClaimsByAgent(ctx.DB, ctx.AgentID)
		if err != nil {
			return toolError(err.Error())
		}

		now := time.Now().Unix()
		updates := db.AgentUpdates{
			Status:   types.OptionalString{Set: true, Value: nil},
			LastSeen: types.OptionalInt64{Set: true, Value: &now},
		}
		_ = db.UpdateAgent(ctx.DB, ctx.AgentID, updates)
		if updated, err := db.GetAgent(ctx.DB, ctx.AgentID); err == nil && updated != nil {
			_ = db.AppendAgent(ctx.Project.DBPath, *updated)
		}

		body := "status cleared"
		if count > 0 {
			body = fmt.Sprintf("status cleared (released %d claim", count) + pluralSuffix(int(count)) + ")"
		}
		msg, err := db.CreateMessage(ctx.DB, types.Message{
			TS:        now,
			FromAgent: ctx.AgentID,
			Body:      body,
			Mentions:  []string{},
		})
		if err == nil {
			_ = db.AppendMessage(ctx.Project.DBPath, msg)
		}

		result := "Status cleared"
		if count > 0 {
			result += fmt.Sprintf(", released %d claim", count) + pluralSuffix(int(count))
		}
		return toolResult(result, false)
	}

	_, _ = db.PruneExpiredClaims(ctx.DB)

	var expiresAt *int64
	if ttlMinutes > 0 {
		value := time.Now().Unix() + int64(ttlMinutes*60)
		expiresAt = &value
	}

	claims := make([]types.ClaimInput, 0, len(files)+len(bdClaims)+len(issues))
	for _, pattern := range files {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeFile, Pattern: pattern})
	}
	for _, entry := range bdClaims {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeBD, Pattern: stripHash(entry)})
	}
	for _, entry := range issues {
		claims = append(claims, types.ClaimInput{ClaimType: types.ClaimTypeIssue, Pattern: stripHash(entry)})
	}

	created := make([]types.Claim, 0, len(claims))
	var errors []string
	for _, claim := range claims {
		createdClaim, err := db.CreateClaim(ctx.DB, types.ClaimInput{
			AgentID:   ctx.AgentID,
			ClaimType: claim.ClaimType,
			Pattern:   claim.Pattern,
			Reason:    optionalString(message),
			ExpiresAt: expiresAt,
		})
		if err != nil {
			errors = append(errors, err.Error())
			continue
		}
		created = append(created, *createdClaim)
	}

	now := time.Now().Unix()
	updates := db.AgentUpdates{LastSeen: types.OptionalInt64{Set: true, Value: &now}}
	if message != "" {
		updates.Status = types.OptionalString{Set: true, Value: &message}
	}
	_ = db.UpdateAgent(ctx.DB, ctx.AgentID, updates)
	if updated, err := db.GetAgent(ctx.DB, ctx.AgentID); err == nil && updated != nil {
		_ = db.AppendAgent(ctx.Project.DBPath, *updated)
	}

	body := message
	if body == "" {
		body = "status update"
	}
	if len(created) > 0 {
		claimList := buildClaimList(created)
		if message != "" {
			body = fmt.Sprintf("%s [claimed: %s]", message, claimList)
		} else {
			body = fmt.Sprintf("claimed: %s", claimList)
		}
	}

	msg, err := db.CreateMessage(ctx.DB, types.Message{
		TS:        now,
		FromAgent: ctx.AgentID,
		Body:      body,
		Mentions:  []string{},
	})
	if err == nil {
		_ = db.AppendMessage(ctx.Project.DBPath, msg)
	}

	result := "Status updated"
	if message != "" {
		result = fmt.Sprintf("Status: %s", message)
	}
	if len(created) > 0 {
		list := make([]string, 0, len(created))
		for _, claim := range created {
			if claim.ClaimType == types.ClaimTypeFile {
				list = append(list, claim.Pattern)
			} else {
				list = append(list, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
			}
		}
		result += "\n\nClaimed:\n  " + strings.Join(list, "\n  ")
	}
	if len(errors) > 0 {
		result += "\n\nErrors:\n  " + strings.Join(errors, "\n  ")
	}

	return toolResult(result, len(errors) > 0)
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

func stripHash(value string) string {
	return strings.TrimPrefix(value, "#")
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func buildClaimList(claims []types.Claim) string {
	parts := make([]string, 0, len(claims))
	for _, claim := range claims {
		if claim.ClaimType == types.ClaimTypeFile {
			parts = append(parts, claim.Pattern)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%s", claim.ClaimType, claim.Pattern))
	}
	return strings.Join(parts, ", ")
}

func parseStaleHours(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
